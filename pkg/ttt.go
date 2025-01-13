package pkg

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
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
	var rollRequest RollRequest
	err = conn.ReadJSON(&rollRequest)
	if err != nil {
		slog.Error(err.Error())
		return
	}
	slog.Info("message received", "message", rollRequest)
	<-room.NewSession(r.Context(), rollRequest.User, conn)
	slog.Info("session ended", "user", rollRequest.User)

	room.mu.Lock()
	if len(room.userSessions) == 0 {
		s.rw.Lock()
		delete(s.Rooms, roomName)
		s.rw.Unlock()
		slog.Info("closed room", "room", roomName)
	}
	room.mu.Unlock()
}

type Room struct {
	mu           sync.Mutex
	userSessions map[string]userSession

	Version int                   `json:"version"`
	Name    string                `json:"name"`
	Dice    string                `json:"required_roll"`
	Rolls   map[string]RollResult `json:"rolls"`
}

func (r *Room) NewSession(ctx context.Context, name string, conn *websocket.Conn) <-chan struct{} {
	doneCh := make(chan struct{}, 1)
	writeCh := make(chan []byte, 1)
	session := userSession{
		name:    name,
		writeCh: writeCh,
	}
	r.mu.Lock()
	r.userSessions[name] = session
	r.mu.Unlock()
	err := r.Roll(name)
	if err != nil {
		slog.Error(err.Error())
		return nil
	}

	go func() {
		ticker := time.NewTicker(PingInterval)
		defer func() {
			doneCh <- struct{}{}

			r.mu.Lock()
			delete(r.userSessions, name)
			r.mu.Unlock()

			ticker.Stop()
			<-ticker.C
		}()
	EXIT:
		for {
			select {
			case <-ctx.Done():
				break EXIT
			case b := <-writeCh:
				err := conn.WriteMessage(websocket.TextMessage, b)
				if err != nil {
					slog.Error(err.Error())
					return
				}
			case <-ticker.C:
				err := conn.WriteMessage(websocket.PingMessage, []byte{})
				if err != nil {
					slog.Error("ping failed", "error", err)
					return
				}
			}
		}
	}()
	return doneCh
}

func (r *Room) RemoveSession(name string) {
	r.mu.Lock()
	delete(r.userSessions, name)
	r.mu.Unlock()
}

func (r *Room) Roll(user string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	dice, err := ParseDiceRoll(r.Dice)
	if err != nil {
		return err
	}
	rollResult := RollResult{
		User:   user,
		Result: dice.Roll(),
	}
	r.Rolls[user] = rollResult
	r.Version++
	b, err := json.Marshal(r)
	if err != nil {
		slog.Error("failed marshalling room", "error", err)
		return err
	}
	for _, us := range r.userSessions {
		us.writeCh <- b
	}
	return nil
}

type userSession struct {
	name    string
	writeCh chan<- []byte
}

type RollRequest struct {
	User string `json:"user"`
	Roll string `json:"roll"`
}

type RollResult struct {
	User   string `json:"user"`
	Result int    `json:"result"`
}

func (s *Server) NewRoom(name string) (*Room, error) {
	s.rw.Lock()
	defer s.rw.Unlock()
	_, ok := s.Rooms[name]
	if ok {
		return nil, ErrRoomExists
	}
	s.Rooms[name] = &Room{
		userSessions: make(map[string]userSession),
		Version:      0,
		Dice:         "1d20",
		Name:         name,
		Rolls:        map[string]RollResult{},
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
