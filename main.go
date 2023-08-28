package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/peterbourgon/ff/v3/ffcli"

	"github.com/abennett/ttt/pkg"
)

var (
	fs   = flag.NewFlagSet("ttt", flag.ExitOnError)
	port = fs.Int("port", 8080, "port number of server")

	clientFS = flag.NewFlagSet("ttt roll", flag.ExitOnError)
	logFile  = clientFS.String("logfile", "", "log to a file")
)

var serveCmd = &ffcli.Command{
	Name:    "serve",
	FlagSet: fs,
	Exec:    serve,
}

var rollCmd = &ffcli.Command{
	Name:       "roll",
	FlagSet:    fs,
	ShortUsage: "roll <host> <room> <username>",
	Exec:       rollRemote,
}

func serve(ctx context.Context, args []string) error {
	h := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug})
	slog.SetDefault(slog.New(h))

	server := pkg.NewServer()
	r := chi.NewRouter()
	r.Use(middleware.DefaultLogger)
	r.Get("/{roomName}", server.ServeHTTP)
	port := ":" + strconv.Itoa(*port)
	slog.Info("serving", "port", port)
	return http.ListenAndServe(port, r)
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

	err := root.Parse(os.Args[1:])
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	err = root.Run(context.Background())
	if err != nil && err != flag.ErrHelp {
		fmt.Println(err)
	}
}
