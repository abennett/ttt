package server

import (
	"errors"
	"log/slog"
	"net/http"
	"sync"

	"github.com/abennett/ttt/pkg"
	"github.com/abennett/ttt/pkg/messages"
	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
)

var (
	ErrRoomExists    = errors.New("room exists")
	ErrRoomNotExists = errors.New("room does not exist")
)

type Server struct {
	rw       *sync.RWMutex
	upgrader websocket.Upgrader

	rooms map[string]*Room
}

func NewServer() *Server {
	return &Server{
		rw:    &sync.RWMutex{},
		rooms: map[string]*Room{},
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
	room, ok := s.rooms[roomName]
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
	defer conn.Close() //nolint: errcheck

	// Keep connection alive
	room.RunSession(r.Context(), conn)

	room.mu.Lock()
	if len(room.userSessions) == 0 {
		s.deleteRoom(roomName)
		slog.Info("closed room", "room", roomName)
	}
	room.mu.Unlock()
}

func (s *Server) NewRoom(name string) (*Room, error) {
	s.rw.Lock()
	defer s.rw.Unlock()
	_, ok := s.rooms[name]
	if ok {
		return nil, ErrRoomExists
	}
	s.rooms[name] = &Room{
		mu:           new(sync.Mutex),
		logger:       slog.With("room", name),
		userSessions: make(map[string]userSession),
		Version:      0,
		Dice: pkg.DiceRoll{
			Count:     1,
			DiceSides: 20,
		},
		Name:  name,
		Rolls: map[string]*messages.RollResult{},
	}
	return s.rooms[name], nil
}

func (s *Server) GetRooms() map[string]Room {
	s.rw.RLock()
	defer s.rw.RUnlock()

	rooms := make(map[string]Room, len(s.rooms))
	for k, v := range s.rooms {
		rooms[k] = *v
	}
	return rooms
}

func (s *Server) GetRoom(roomName string) (*Room, error) {
	s.rw.RLock()
	defer s.rw.RUnlock()
	room, ok := s.rooms[roomName]
	if !ok {
		return room, ErrRoomNotExists
	}
	return room, nil
}

func (s *Server) deleteRoom(roomName string) {
	s.rw.Lock()
	delete(s.rooms, roomName)
	s.rw.Unlock()
}
