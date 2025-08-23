package server

import (
	"cmp"
	"context"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/vmihailenco/msgpack/v5"

	"github.com/abennett/ttt/pkg"
	"github.com/abennett/ttt/pkg/messages"
)

const (
	PingInterval = 5 * time.Second
)

type userSession struct {
	wg      *sync.WaitGroup
	logger  *slog.Logger
	name    string
	writeCh chan []byte
}

type Room struct {
	mu           *sync.Mutex
	logger       *slog.Logger
	userSessions map[string]userSession

	Version int
	Name    string
	Dice    pkg.DiceRoll
	Rolls   map[string]messages.RollResult
}

func (r *Room) RunSession(ctx context.Context, conn *websocket.Conn) {
	_, b, err := conn.ReadMessage()
	if err != nil {
		r.logger.Error("failed to read initial message", "error", err)
		return
	}

	var msg messages.Message
	if err = msgpack.Unmarshal(b, &msg); err != nil {
		r.logger.Error("failed to parse initial message", "error", err, "payload", string(b))
		return
	}

	req, ok := msg.Payload.(messages.RollRequest)
	if !ok {
		r.logger.Error("initial message was incorrect", "error", err, "payload", string(b))
		return
	}

	name := req.User
	r.logger.Debug("starting a session", "user", name)
	writeCh := make(chan []byte, 1)
	session := userSession{
		wg:      new(sync.WaitGroup),
		logger:  slog.With("user", req.User),
		name:    req.User,
		writeCh: writeCh,
	}

	r.startUserSession(ctx, session, conn)

	roll := messages.RollResult{
		User:   name,
		Result: r.Dice.Roll(),
	}

	err = r.Update(roll)
	if err != nil {
		r.logger.Error(err.Error())
		return
	}

	session.wg.Wait()
	r.stopUserSession(session)
	r.logger.Info("closing session", "active_sessions", len(r.userSessions), "user", name)
}

func (r *Room) startUserSession(ctx context.Context, session userSession, conn *websocket.Conn) {
	r.mu.Lock()
	r.userSessions[session.name] = session
	r.mu.Unlock()

	// Add to the waitGroup outside of goroutines here to avoid race condition on Add
	ctx, cancel := context.WithCancel(ctx)
	session.wg.Add(2)
	go r.userReadLoop(cancel, session, conn)
	go r.userWriteLoop(ctx, session, conn)
}

func (r *Room) stopUserSession(session userSession) {
	r.mu.Lock()
	delete(r.userSessions, session.name)
	r.mu.Unlock()
}

func (r *Room) userReadLoop(cancel func(), session userSession, conn *websocket.Conn) {
	defer cancel()
	defer session.wg.Done()
	defer session.logger.Debug("closing read loop")

	for {
		t, _, err := conn.ReadMessage()
		if closeErr, ok := err.(*websocket.CloseError); ok {
			if closeErr.Code == websocket.CloseNormalClosure {
				session.logger.Info("close message received")
				return
			}
		}
		if err != nil {
			r.logger.Error("failure in user read loop", "error", err)
			return
		}

		switch t {
		case websocket.CloseMessage:
			session.logger.Info("close message received")
			return
		case websocket.BinaryMessage:
			session.logger.Info("binary message received")
			// handle
		}
	}
}

func (r *Room) HandleBinaryMessage(b []byte) error {
	var msg messages.Message
	err := msgpack.Unmarshal(b, &msg)
	if err != nil {
		return messages.ErrMessageInvalid
	}

	switch msg.Payload {

	}
	return nil
}

func (r *Room) userWriteLoop(ctx context.Context, session userSession, conn *websocket.Conn) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	defer session.wg.Done()
	defer session.logger.Debug("closing write loop")
	ticker := time.NewTicker(PingInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			session.logger.Debug("write loop is done")
			return
		case b := <-session.writeCh:
			session.logger.Debug("writing message")
			err := conn.WriteMessage(websocket.BinaryMessage, b)
			if err != nil {
				r.logger.Error(err.Error())
				return
			}
		case <-ticker.C:
			session.logger.Debug("writing ping message")
			err := conn.WriteMessage(websocket.PingMessage, []byte{})
			if err == websocket.ErrCloseSent {
				session.logger.Debug("error close was sent")
				return
			}
			if err != nil {
				session.logger.Error("ping failed", "error", err)
				return
			}
		}
	}
}

func (r *Room) Update(update any) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	switch u := update.(type) {
	case messages.RollResult:
		r.Rolls[u.User] = u
		r.logger.Debug("added roll", "active_sessions", len(r.userSessions), "user", u.User)
	default:
		err := fmt.Errorf("unknown update type: %T", update)
		r.logger.Error(err.Error())
		return err
	}

	r.Version++

	msg := messages.Message{
		Type:    messages.StateMsgType,
		Version: "1",
		Payload: r.ToState(),
	}
	b, err := msgpack.Marshal(msg)
	if err != nil {
		r.logger.Error("failed marshalling room", "error", err)
		return err
	}

	for _, us := range r.userSessions {
		r.logger.Debug("pushing update", "user", us.name, "version", r.Version)
		us.writeCh <- b
	}
	return nil
}

func (r *Room) ToState() messages.RoomState {
	rolls := make([]messages.RollResult, len(r.Rolls))
	var i int
	for _, roll := range r.Rolls {
		rolls[i] = roll
		i++
	}
	slices.SortFunc(rolls, func(a, b messages.RollResult) int {
		return cmp.Compare(b.Result, a.Result)
	})
	return messages.RoomState{
		Version: r.Version,
		Name:    r.Name,
		Dice:    r.Dice.String(),
		Rolls:   rolls,
	}
}
