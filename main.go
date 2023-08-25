package main

import (
	"cmp"
	"context"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"slices"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/gorilla/websocket"
	"github.com/peterbourgon/ff/v3/ffcli"

	"github.com/abennett/ttt/pkg"
)

var serveCmd = &ffcli.Command{
	Name: "serve",
	Exec: serve,
}

var rollCmd = &ffcli.Command{
	Name:       "roll_remote",
	ShortUsage: "roll_remote <ws://host:port> <username>",
	Exec:       roll,
}

func serve(ctx context.Context, args []string) error {
	server := pkg.NewServer()
	r := chi.NewRouter()
	r.Use(middleware.DefaultLogger)
	r.Get("/{roomID}", server.ServeHTTP)
	slog.Info("serving", "port", ":8080")
	return http.ListenAndServe(":8080", r)
}

func roll(ctx context.Context, args []string) error {
	req := pkg.RollRequest{
		User: args[1],
	}
	conn, _, err := websocket.DefaultDialer.Dial(args[0], nil)
	if err != nil {
		return err
	}
	defer conn.Close()
	err = conn.WriteJSON(req)
	if err != nil {
		return err
	}
	var currentVersion int
	for {
		var room pkg.Room
		err := conn.ReadJSON(&room)
		if err != nil {
			return err
		}
		if currentVersion == room.Version {
			continue
		}
		currentVersion = room.Version
		fmt.Printf("\n===== New Room Version: %d =====\n", currentVersion)
		rolls := make([]pkg.RollResult, len(room.Rolls))
		var idx int
		for _, rr := range room.Rolls {
			rolls[idx] = rr
			idx++
		}
		slices.SortFunc(rolls, func(a, b pkg.RollResult) int {
			return cmp.Compare(b.Result, a.Result)
		})
		for _, roll := range rolls {
			fmt.Printf("%s: %d\n", roll.User, roll.Result)
		}
	}
}

var diceRollCmd = &ffcli.Command{
	Name: "roll_local",
	Exec: func(ctx context.Context, args []string) error {
		if len(args) == 0 {
			fmt.Println("a roll argument is required")
			return nil
		}
		dr, err := pkg.ParseDiceRoll(args[0])
		if err != nil {
			return err
		}
		fmt.Printf("%s => %d\n", args[0], dr.Roll())
		return nil
	},
}

func main() {
	root := &ffcli.Command{
		ShortUsage: "ttt <subcommand>",
		Subcommands: []*ffcli.Command{
			diceRollCmd,
			serveCmd,
			rollCmd,
		},
		Exec: func(context.Context, []string) error {
			return flag.ErrHelp
		},
	}

	err := root.ParseAndRun(context.Background(), os.Args[1:])
	if err != nil {
		log.Fatal(err)
	}
}
