package messages

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/vmihailenco/msgpack/v5"
)

var (
	ErrMessageInvalid     = errors.New("message was invalid")
	ErrUnknownMessageType = errors.New("unknown message type")
)

type Type int

const (
	StateMsgType Type = iota
	DoneRequestType
	RollRequestType
)

type Message struct {
	_msgpack struct{} `msgpack:",as_array"` //nolint:unused
	Type     Type     `msgpack:"type"`
	Version  string   `msgpack:"version"`
	Payload  any
}

func (m *Message) UnmarshalMsgpack(b []byte) error {
	decoder := msgpack.NewDecoder(bytes.NewReader(b))
	l, err := decoder.DecodeArrayLen()
	if err != nil {
		return err
	}
	if l != 3 {
		panic("nope")
	}
	t, err := decoder.DecodeInt()
	if err != nil {
		return err
	}
	m.Type = Type(t)

	if err = decoder.Skip(); err != nil {
		return err
	}

	switch m.Type {
	case DoneRequestType:
		var done DoneRequest
		if err = decoder.Decode(&done); err != nil {
			return err
		}
		m.Payload = done
	case StateMsgType:
		var room RoomState
		if err = decoder.Decode(&room); err != nil {
			return err
		}
		m.Payload = room
	case RollRequestType:
		var roll RollRequest
		if err = decoder.Decode(&roll); err != nil {
			return err
		}
		m.Payload = roll
	default:
		panic(fmt.Sprintf("unexpected messages.Type: %#v", m.Type))
	}
	return nil
}

type RoomState struct {
	Version int          `msgpack:"version"`
	Name    string       `msgpack:"name"`
	Dice    string       `msgpack:"required_roll"`
	Rolls   []RollResult `msgpack:"rolls"`
}

type RollRequest struct {
	User string `msgpack:"user"`
	Roll string `msgpack:"roll"`
}

type RollResult struct {
	User   string `msgpack:"user"`
	Result int    `msgpack:"result"`
	IsDone bool   `msgpack:"is_done"`
}

type DoneRequest struct {
	User string `msgpack:"user"`
}
