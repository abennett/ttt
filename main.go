package main

import (
	"context"
	"flag"
	"fmt"
	"io"
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
	fs   = flag.NewFlagSet("ttt remote_roll", flag.ExitOnError)
	port = fs.Int("port", 8080, "port number of server")

	rootFS  = flag.NewFlagSet("ttt", flag.ExitOnError)
	logFile = rootFS.String("logfile", "", "log to a file")
)

var serveCmd = &ffcli.Command{
	Name:    "serve",
	FlagSet: fs,
	Exec:    serve,
}

var rollCmd = &ffcli.Command{
	Name:       "roll_remote",
	FlagSet:    fs,
	ShortUsage: "roll_remote <host> <room> <username>",
	Exec:       rollRemote,
}

func serve(ctx context.Context, args []string) error {
	server := pkg.NewServer()
	r := chi.NewRouter()
	r.Use(middleware.DefaultLogger)
	r.Get("/{roomID}", server.ServeHTTP)
	slog.Info("serving", "port", ":"+strconv.Itoa(*port))
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
	root := &ffcli.Command{
		ShortUsage: "ttt <subcommand>",
		FlagSet:    rootFS,
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

	logOutput := io.Discard
	if logFile != nil && *logFile != "" {
		lf, err := os.OpenFile(*logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		logOutput = lf
	}
	h := slog.NewTextHandler(logOutput, &slog.HandlerOptions{Level: slog.LevelDebug})
	slog.SetDefault(slog.New(h))

	err = root.Run(context.Background())
	if err != nil && err != flag.ErrHelp {
		fmt.Println(err)
	}
}
