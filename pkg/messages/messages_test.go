package messages

import (
	"testing"

	"github.com/shoenig/test/must"
	"github.com/vmihailenco/msgpack/v5"
)

func TestCustomUnmarshal_Done(t *testing.T) {
	base := Message{
		Type:    DoneRequestType,
		Version: "1",
		Payload: DoneRequest{
			DoneIndex: 3,
		},
	}
	b, err := msgpack.Marshal(base)
	must.NoError(t, err)
	var un Message
	err = msgpack.Unmarshal(b, &un)
	must.NoError(t, err)
	done, ok := un.Payload.(DoneRequest)
	must.True(t, ok)
	must.EqOp(t, 3, done.DoneIndex)
}

func TestCustomUnmarshal_RoomState(t *testing.T) {
	base := Message{
		Type:    StateMsgType,
		Version: "1",
		Payload: RoomState{
			Version:   1,
			Name:      "test",
			Dice:      "1d20",
			DoneIndex: 0,
			Rolls:     []RollResult{},
		},
	}
	b, err := msgpack.Marshal(base)
	must.NoError(t, err)
	var un Message
	err = msgpack.Unmarshal(b, &un)
	must.NoError(t, err)
	room, ok := un.Payload.(RoomState)
	must.True(t, ok)
	must.EqOp(t, room.Version, 1)
}
