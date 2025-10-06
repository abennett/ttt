package main

import (
	"io"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/shoenig/test/must"
	"github.com/shoenig/test/wait"

	"github.com/abennett/ttt/pkg/client"
	"github.com/abennett/ttt/pkg/server"
)

func TestSingleClient(t *testing.T) {
	t.Parallel()
	srv := server.NewServer()
	mux := server.NewMux(srv)
	testSrv := httptest.NewServer(mux)

	client, err := client.New(testSrv.URL, "test1", "tester", io.Discard)
	must.NoError(t, err)

	err = client.Init()
	must.NoError(t, err)

	must.MapContainsKey(t, srv.GetRooms(), "test1")
	must.Wait(t, wait.InitialSuccess(wait.BoolFunc(func() bool {
		return len(client.Room.Rolls) > 0
	})))

	rooms := srv.GetRooms()
	roomState := rooms["test1"]
	must.Eq(t, roomState.Version, client.Room.Version)
	t.Log(roomState)

	must.MapContainsKey(t, roomState.Rolls, "tester")
	tester := roomState.Rolls["tester"]
	must.False(t, tester.IsDone)
	must.NoError(t, client.ToggleDone())
	time.Sleep(time.Second)
	rooms = srv.GetRooms()
	roomState = rooms["test1"]
	must.MapContainsKey(t, roomState.Rolls, "tester")
	tester = roomState.Rolls["tester"]
	must.True(t, tester.IsDone)
	must.EqOp(t, 0, tester.ID)

	err = client.Close()
	must.NoError(t, err)
	time.Sleep(time.Second)
	must.MapEmpty(t, srv.GetRooms())
}

func TestMultipleClients(t *testing.T) {
	t.Parallel()
	srv := server.NewServer()
	mux := server.NewMux(srv)
	testSrv := httptest.NewServer(mux)

	client1, err := client.New(testSrv.URL, "test1", "tester1", io.Discard)
	must.NoError(t, err)

	client2, err := client.New(testSrv.URL, "test1", "tester2", io.Discard)
	must.NoError(t, err)

	err = client1.Init()
	must.NoError(t, err)

	err = client2.Init()
	must.NoError(t, err)

	must.MapContainsKey(t, srv.GetRooms(), "test1")
	must.Wait(t, wait.InitialSuccess(wait.BoolFunc(func() bool {
		return client1.Room.Version == 2
	})))
	must.Wait(t, wait.InitialSuccess(wait.BoolFunc(func() bool {
		return client2.Room.Version == 2
	})))

	rooms := srv.GetRooms()
	roomState := rooms["test1"]
	tester1 := roomState.Rolls["tester1"]
	must.Eq(t, 0, tester1.ID)
	tester2 := roomState.Rolls["tester2"]
	must.Eq(t, 1, tester2.ID)
}
