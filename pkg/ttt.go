package pkg

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
	"github.com/vmihailenco/msgpack/v5"
)

const (
	PingInterval = time.Second
)

var (
	ErrRoomExists    = errors.New("room exists")
	ErrRoomNotExists = errors.New("room does not exist")
)

type Server struct {
	rw       *sync.RWMutex
	upgrader websocket.Upgrader

	Rooms map[string]*Room
}

func NewServer() *Server {
	return &Server{
		rw:    &sync.RWMutex{},
		Rooms: map[string]*Room{},
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	roomName := chi.URLParam(r, "roomName")
	if roomName == "" {
		http.Error(w, "room name is required", http.StatusBadRequest)
		return
	}
	slog.Info("serving request", "roomName", roomName)
	var err error
	room, ok := s.Rooms[roomName]
	if !ok {
		room, err = s.NewRoom(roomName)
		if err != nil {
			slog.Error("unable to create new room", "room_name", roomName, "error", err)
			http.Error(w, "unable to create new room", http.StatusInternalServerError)
			return
		}
	}
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error(err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer conn.Close()

	// Keep connection alive
	room.RunSession(r.Context(), conn)

	room.mu.Lock()
	if len(room.userSessions) == 0 {
		s.rw.Lock()
		delete(s.Rooms, roomName)
		s.rw.Unlock()
		slog.Info("closed room", "room", roomName)
	}
	room.mu.Unlock()
}

type RollRequest struct {
	User string `msgpack:"user"`
	Roll string `msgpack:"roll"`
}

type RollResult struct {
	User   string `msgpack:"user"`
	Result int    `msgpack:"result"`
}

type Room struct {
	mu           *sync.Mutex
	userSessions map[string]userSession

	Version int
	Name    string
	Dice    DiceRoll
	Rolls   map[string]RollResult
}

type RoomState struct {
	Version int          `msgpack:"version"`
	Name    string       `msgpack:"name"`
	Dice    string       `msgpack:"required_roll"`
	Rolls   []RollResult `msgpack:"rolls"`
}

type userSession struct {
	wg      *sync.WaitGroup
	name    string
	writeCh chan []byte
}

func (r *Room) startUserSession(ctx context.Context, session userSession, conn *websocket.Conn) {
	// Add to the waitGroup outside of goroutines here to avoid race condition on Add
	session.wg.Add(2)
	go r.userReadLoop(ctx, session, conn)
	go r.userWriteLoop(ctx, session, conn)
}

func (r *Room) userReadLoop(ctx context.Context, session userSession, conn *websocket.Conn) {
	defer session.wg.Done()
	for {
		t, _, err := conn.ReadMessage()
		if closeErr, ok := err.(*websocket.CloseError); ok {
			if closeErr.Code == websocket.CloseNormalClosure {
				return
			}
		}
		if err != nil {
			slog.Error("failure in user read loop", "error", err)
			return
		}

		switch t {
		case websocket.CloseMessage:
			slog.Info("close message received")
			return
		case websocket.BinaryMessage:
			slog.Info("binary message received")
			// handle
		}
	}
}

func (r *Room) userWriteLoop(ctx context.Context, session userSession, conn *websocket.Conn) {
	ticker := time.NewTicker(PingInterval)
	defer func() {
		r.mu.Lock()
		delete(r.userSessions, session.name)
		r.mu.Unlock()

		ticker.Stop()
		session.wg.Done()
	}()
EXIT:
	for {
		select {
		case <-ctx.Done():
			break EXIT
		case b := <-session.writeCh:
			slog.Debug("writing message", "user", session.name)
			err := conn.WriteMessage(websocket.BinaryMessage, b)
			if err != nil {
				slog.Error(err.Error())
				return
			}
		case <-ticker.C:
			err := conn.WriteMessage(websocket.PingMessage, []byte{})
			if err == websocket.ErrCloseSent {
				return
			}
			if err != nil {
				slog.Error("ping failed", "error", err)
				return
			}
		}
	}
}

func (r *Room) RunSession(ctx context.Context, conn *websocket.Conn) {
	_, b, err := conn.ReadMessage()
	if err != nil {
		slog.Error("failed to read initial message", "error", err)
		return
	}
	var req RollRequest
	if err = msgpack.Unmarshal(b, &req); err != nil {
		slog.Error("failed to parse initial message", "error", err, "payload", string(b))
		return
	}
	name := req.User
	writeCh := make(chan []byte, 1)
	session := userSession{
		wg:      new(sync.WaitGroup),
		name:    req.User,
		writeCh: writeCh,
	}
	r.mu.Lock()
	r.userSessions[name] = session
	r.mu.Unlock()

	r.startUserSession(ctx, session, conn)

	roll := RollResult{
		User:   name,
		Result: r.Dice.Roll(),
	}
	err = r.Update(roll)
	if err != nil {
		slog.Error(err.Error())
		return
	}

	session.wg.Wait()
	slog.Info("closing session", "user", name)
}

func (r *Room) Update(update any) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	switch u := update.(type) {
	case RollResult:
		r.Rolls[u.User] = u
	default:
		err := fmt.Errorf("unknown update type: %T", update)
		slog.Error(err.Error())
		return err
	}

	r.Version++

	b, err := msgpack.Marshal(r.toState())
	if err != nil {
		slog.Error("failed marshalling room", "error", err)
		return err
	}

	for _, us := range r.userSessions {
		slog.Debug("pushing update", "user", us.name, "version", r.Version)
		us.writeCh <- b
	}
	return nil
}

func (r *Room) toState() RoomState {
	rolls := make([]RollResult, len(r.Rolls))
	var i int
	for _, roll := range r.Rolls {
		rolls[i] = roll
		i++
	}
	slices.SortFunc(rolls, func(a, b RollResult) int {
		return cmp.Compare(b.Result, a.Result)
	})
	return RoomState{
		Version: r.Version,
		Name:    r.Name,
		Dice:    r.Dice.String(),
		Rolls:   rolls,
	}
}

func (s *Server) NewRoom(name string) (*Room, error) {
	s.rw.Lock()
	defer s.rw.Unlock()
	_, ok := s.Rooms[name]
	if ok {
		return nil, ErrRoomExists
	}
	s.Rooms[name] = &Room{
		mu:           new(sync.Mutex),
		userSessions: make(map[string]userSession),
		Version:      0,
		Dice: DiceRoll{
			Count:     1,
			DiceSides: 20,
		},
		Name:  name,
		Rolls: map[string]RollResult{},
	}
	return s.Rooms[name], nil
}

func (s *Server) GetRoom(roomName string) (*Room, error) {
	s.rw.RLock()
	defer s.rw.RUnlock()
	room, ok := s.Rooms[roomName]
	if !ok {
		return room, ErrRoomNotExists
	}
	return room, nil
}
