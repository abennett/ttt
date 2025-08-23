package client

import (
	"net/http/httptest"
	"testing"

	"github.com/shoenig/test/must"
	"github.com/shoenig/test/wait"

	"github.com/abennett/ttt/pkg/server"
)

func TestSingleClient(t *testing.T) {
	t.Parallel()
	srv := server.NewServer()
	mux := server.NewMux(srv)
	testSrv := httptest.NewServer(mux)

	client, err := New(testSrv.URL, "test1", "tester", nil)
	must.NoError(t, err)

	err = client.Init()
	must.NoError(t, err)

	must.MapContainsKey(t, srv.Rooms, "test1")
	must.Wait(t, wait.InitialSuccess(wait.BoolFunc(func() bool {
		return len(client.Room.Rolls) > 0
	})))

	roomState := srv.Rooms["test1"].ToState()
	must.Eq(t, roomState, client.Room)
	t.Log(roomState)
}

func TestMultipleClients(t *testing.T) {
	t.Parallel()
	srv := server.NewServer()
	mux := server.NewMux(srv)
	testSrv := httptest.NewServer(mux)

	client1, err := New(testSrv.URL, "test1", "tester1", nil)
	must.NoError(t, err)

	client2, err := New(testSrv.URL, "test1", "tester2", nil)
	must.NoError(t, err)

	err = client1.Init()
	must.NoError(t, err)

	err = client2.Init()
	must.NoError(t, err)

	must.MapContainsKey(t, srv.Rooms, "test1")
	must.Wait(t, wait.InitialSuccess(wait.BoolFunc(func() bool {
		return client1.Room.Version == 2
	})))
	must.Wait(t, wait.InitialSuccess(wait.BoolFunc(func() bool {
		return client2.Room.Version == 2
	})))

	roomState := srv.Rooms["test1"].ToState()
	must.Eq(t, roomState, client1.Room)
	must.Eq(t, roomState, client2.Room)
	t.Log(roomState)
}
