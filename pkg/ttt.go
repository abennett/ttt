package pkg

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"math/rand"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
)

var (
	ErrRoomExists    = errors.New("room exists")
	ErrRoomNotExists = errors.New("room does not exist")
)

type Server struct {
	rw       sync.RWMutex
	upgrader websocket.Upgrader

	Rooms map[string]*Room
}

func NewServer() *Server {
	return &Server{
		rw:    sync.RWMutex{},
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
	_, ok := s.Rooms[roomName]
	if !ok {
		s.NewRoom(roomName)
	}
	defer func() {
		err := s.Disconnect(roomName)
		if err != nil {
			slog.Error("failed to run disconnect on room", "error", err)
		}
	}()
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
	err = s.Roll(roomName, rollRequest.User)
	if err != nil {
		slog.Error(err.Error())
		return
	}
	for {
		// update room status
		room, err := s.GetRoom(roomName)
		if err != nil {
			slog.Error(err.Error())
			return
		}
		b, _ := json.Marshal(room)
		err = conn.WriteMessage(websocket.TextMessage, b)
		if err != nil {
			slog.Error(err.Error())
			return
		}
		time.Sleep(time.Second)
	}
}

type Room struct {
	// users must be mutated under a lock
	users int

	Version int                   `json:"version"`
	Name    string                `json:"name"`
	Dice    string                `json:"required_roll"`
	Rolls   map[string]RollResult `json:"rolls"`
}

type RollRequest struct {
	User string `json:"user"`
	Roll string `json:"roll"`
}

type RollResult struct {
	User   string `json:"user"`
	Result int    `json:"result"`
}

type DiceRoll struct {
	Count     int
	DiceSides int
	Modifier  int
}

func ParseDiceRoll(diceRoll string) (DiceRoll, error) {
	// <int>d<int>[+|-<int>]
	// (\d+)d(\d+)???
	var d DiceRoll
	r := regexp.MustCompile(`(\d+)d(\d+)(\+\d+|\-\d+)?`)
	matches := r.FindStringSubmatch(diceRoll)
	if len(matches) < 3 {
		return d, errors.New("string does not match expression")
	}
	parsed := make([]int, 3)
	for idx, s := range matches[1:] {
		if s == "" {
			parsed[idx] = 0
			continue
		}
		v, err := strconv.Atoi(s)
		if err != nil {
			return d, err
		}
		parsed[idx] = v
	}
	return DiceRoll{
		Count:     parsed[0],
		DiceSides: parsed[1],
		Modifier:  parsed[2],
	}, nil
}

func (dr DiceRoll) String() string {
	var builder strings.Builder
	base := fmt.Sprintf("%dd%d", dr.Count, dr.DiceSides)
	builder.WriteString(base)
	if dr.Modifier > 0 {
		builder.WriteString("+" + strconv.Itoa(dr.Modifier))
	}
	if dr.Modifier < 0 {
		absolute := int(math.Abs(float64(dr.Modifier)))
		builder.WriteString("-" + strconv.Itoa(absolute))
	}
	return builder.String()
}

func (dr DiceRoll) Roll() int {
	var result int
	for x := 0; x < dr.Count; x++ {
		result += rand.Intn(dr.DiceSides) + 1
	}
	return result + dr.Modifier
}

func (s *Server) NewRoom(name string) error {
	s.rw.Lock()
	defer s.rw.Unlock()
	_, ok := s.Rooms[name]
	if ok {
		return ErrRoomExists
	}
	s.Rooms[name] = &Room{
		Version: 0,
		Dice:    "1d20",
		Name:    name,
		Rolls:   map[string]RollResult{},
	}
	return nil
}

func (s *Server) GetRoom(roomName string) (*Room, error) {
	room := new(Room)
	s.rw.RLock()
	defer s.rw.RUnlock()
	room, ok := s.Rooms[roomName]
	if !ok {
		return room, ErrRoomNotExists
	}
	return room, nil
}

func (s *Server) Disconnect(roomName string) error {
	s.rw.Lock()
	defer s.rw.Unlock()

	room, ok := s.Rooms[roomName]
	if !ok {
		return ErrRoomNotExists
	}

	room.users--
	if room.users <= 0 {
		delete(s.Rooms, roomName)
		slog.Info("deleted room", "room_name", roomName)
	}

	return nil
}

func (s *Server) Roll(roomName, user string) error {
	s.rw.Lock()
	defer s.rw.Unlock()
	room, ok := s.Rooms[roomName]
	if !ok {
		return ErrRoomNotExists
	}
	room.users++
	dice, err := ParseDiceRoll(room.Dice)
	if err != nil {
		return err
	}
	rollResult := RollResult{
		User:   user,
		Result: dice.Roll(),
	}
	room.Rolls[user] = rollResult
	room.Version++
	return nil
}
