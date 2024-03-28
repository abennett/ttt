package main

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"slices"
	"strconv"
	"time"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/gorilla/websocket"

	"github.com/abennett/ttt/pkg"
)

var ErrTooManyRedirects = errors.New("too many redirects")

var baseStyle = lipgloss.NewStyle().
	BorderStyle(lipgloss.NormalBorder()).
	Align(lipgloss.Center)

var columns = []table.Column{
	{Title: "User", Width: 10},
	{Title: "Result", Width: 6},
}

type client struct {
	cmd      tea.Cmd
	endpoint string
	table    table.Model
	updates  chan []pkg.RollResult
}

func connectLoop(wsUrl string) (*websocket.Conn, error) {
	for x := 0; x < 3; x++ {
		slog.Debug("attempting connection", "url", wsUrl)
		conn, resp, err := websocket.DefaultDialer.Dial(wsUrl, nil)
		slog.Debug("connection attempted",
			"resp", resp,
			"error", err)
		if err != nil {
			if resp != nil {
				_, _ = io.Copy(os.Stderr, resp.Body)
			}
			return nil, err
		}
		if resp != nil && resp.StatusCode >= 300 && resp.StatusCode < 400 {
			wsUrl = resp.Header.Get("Location")
			slog.Debug("redirecting", "location", wsUrl)
			continue
		}
		return conn, nil
	}

	return nil, ErrTooManyRedirects
}

func hostUrl(endpoint, room string) (string, error) {
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return "", err
	}
	var scheme string
	switch parsed.Scheme {
	case "https", "wss":
		scheme = "wss"
	case "http", "ws":
		scheme = "ws"
	default:
		return "", fmt.Errorf("%s is not a valid protocol", parsed.Scheme)
	}
	hostUrl := fmt.Sprintf("%s://%s:%d/%s", scheme, parsed.Host, *port, room)
	return hostUrl, nil
}

func newClient(host, room, user string) (client, error) {
	var c client
	logWriter := io.Discard
	if logFile != nil && *logFile != "" {
		f, err := os.OpenFile(*logFile, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0644)
		if err != nil {
			return c, err
		}
		logWriter = f
	}
	h := slog.NewTextHandler(logWriter, &slog.HandlerOptions{Level: slog.LevelDebug})
	slog.SetDefault(slog.New(h))
	req := pkg.RollRequest{
		User: user,
	}
	endpoint, err := hostUrl(host, room)
	if err != nil {
		return c, err
	}
	slog.Debug("using endpoint", "endpoint", endpoint)
	conn, err := connectLoop(endpoint)
	if err != nil {
		return c, err
	}
	err = conn.WriteJSON(req)
	if err != nil {
		return c, fmt.Errorf("unable to write json: %w", err)
	}
	updates := make(chan []pkg.RollResult, 1)
	cmd := updateLoop(conn, updates)
	t := table.New(
		table.WithColumns(columns),
		table.WithHeight(0),
		table.WithFocused(false),
	)
	s := table.DefaultStyles()
	s.Header = s.Header.Foreground(lipgloss.Color("#01c5d1"))
	s.Selected = s.Selected.Foreground(lipgloss.NoColor{}).Bold(false)
	t.SetStyles(s)
	//table.WithStyles(
	//	table.Styles{
	//		Header:   lipgloss.NewStyle().Bold(true).Padding(0, 1),
	//		Cell:     lipgloss.NewStyle().Padding(0, 1),
	//		Selected: lipgloss.NewStyle().Padding(0, 1),
	//	}),
	return client{
		cmd:      cmd,
		table:    t,
		endpoint: host,
		updates:  updates,
	}, nil
}

func (c client) Init() tea.Cmd {
	return tea.Batch(c.cmd, c.tick())
}

func resultsToRows(rrs []pkg.RollResult) []table.Row {
	rows := make([]table.Row, len(rrs))
	for idx, rr := range rrs {
		rows[idx] = table.Row{rr.User, strconv.Itoa(rr.Result)}
	}
	return rows
}

func (c client) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	slog.Debug("updating model", "msg", msg)
	switch msg := msg.(type) {
	case []pkg.RollResult:
		slog.Debug("roll result")
		c.table.SetHeight(len(msg))
		c.table.SetRows(resultsToRows(msg))
		return c, c.tick()
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return c, tea.Quit
		}
	}
	c.table, _ = c.table.Update(msg)
	return c, nil
}

func (c client) View() string {
	slog.Debug("rerendering view")
	return baseStyle.Render(c.table.View()) + "\n"
}

func (c client) tick() tea.Cmd {
	return tea.Every(time.Second, func(time.Time) tea.Msg {
		slog.Debug("ticking")
		return <-c.updates
	})
}

func updateLoop(conn *websocket.Conn, updates chan<- []pkg.RollResult) tea.Cmd {
	return func() tea.Msg {
		go func() {
			slog.Debug("running update loop")
			var currentVersion int
			for {
				var room pkg.Room
				err := conn.ReadJSON(&room)
				if err != nil {
					slog.Error(err.Error())
					return
				}
				slog.Debug("message recieved")
				if currentVersion == room.Version {
					continue
				}
				currentVersion = room.Version
				rolls := make([]pkg.RollResult, len(room.Rolls))
				var idx int
				for _, rr := range room.Rolls {
					rolls[idx] = rr
					idx++
				}
				slices.SortFunc(rolls, func(a, b pkg.RollResult) int {
					return cmp.Compare(b.Result, a.Result)
				})
				updates <- rolls
			}
		}()
		return nil
	}
}

func rollRemote(ctx context.Context, args []string) error {
	c, err := newClient(args[0], args[1], args[2])
	if err != nil {
		return err
	}
	_, err = tea.NewProgram(c).Run()
	return err
}
