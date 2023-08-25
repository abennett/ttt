package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
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
	Exec:       rollRemote,
}

func serve(ctx context.Context, args []string) error {
	server := pkg.NewServer()
	r := chi.NewRouter()
	r.Use(middleware.DefaultLogger)
	r.Get("/{roomID}", server.ServeHTTP)
	slog.Info("serving", "port", ":8080")
	return http.ListenAndServe(":8080", r)
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
	h := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug})
	slog.SetDefault(slog.New(h))
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
