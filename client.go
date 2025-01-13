package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"strconv"
	"time"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/gorilla/websocket"
	"github.com/vmihailenco/msgpack/v5"

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
	user     string
	endpoint string
	table    table.Model
	updates  chan []pkg.RollResult
	done     chan struct{}
	err      error
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
		defer resp.Body.Close()
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
	parsed.Scheme = scheme
	parsed.Path = room
	return parsed.String(), nil
}

func setupLogger(user string, logFile *string) error {
	logWriter := io.Discard
	if logFile != nil && *logFile != "" {
		f, err := os.OpenFile(*logFile, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0644)
		if err != nil {
			return err
		}
		logWriter = f
	}
	h := slog.NewTextHandler(logWriter, &slog.HandlerOptions{Level: slog.LevelDebug})
	logger := slog.New(h)
	logger = logger.With("user", user)
	slog.SetDefault(logger)
	return nil
}

func newClient(host, room, user string) (*client, error) {
	err := setupLogger(user, logFile)
	if err != nil {
		return nil, fmt.Errorf("unable to setup log file: %w", err)
	}

	endpoint, err := hostUrl(host, room)
	if err != nil {
		return nil, err
	}
	slog.Debug("using endpoint", "endpoint", endpoint)

	t := table.New(
		table.WithColumns(columns),
		table.WithHeight(0),
		table.WithFocused(false),
	)
	s := table.DefaultStyles()
	s.Header = s.Header.Foreground(lipgloss.Color("#01c5d1"))
	s.Selected = s.Selected.Foreground(lipgloss.NoColor{}).Bold(false)
	t.SetStyles(s)
	return &client{
		user:     user,
		table:    t,
		endpoint: endpoint,
		updates:  make(chan []pkg.RollResult),
		done:     make(chan struct{}),
	}, nil
}

func errorCmd(err error) tea.Cmd {
	return func() tea.Msg {
		return err
	}
}

func (c *client) Init() tea.Cmd {
	slog.Debug("running Init")
	conn, err := connectLoop(c.endpoint)
	if err != nil {
		return errorCmd(err)
	}

	req := pkg.RollRequest{
		User: c.user,
	}
	b, err := msgpack.Marshal(req)
	if err != nil {
		return errorCmd(fmt.Errorf("failed to marshal: %w", err))
	}
	err = conn.WriteMessage(websocket.BinaryMessage, b)
	if err != nil {
		return errorCmd(fmt.Errorf("unable to write server: %w", err))
	}

	go waitClose(conn, c.done)
	go updateLoop(conn, c.updates)

	return c.readUpdate()
}

func resultsToRows(rrs []pkg.RollResult) []table.Row {
	rows := make([]table.Row, len(rrs))
	for idx, rr := range rrs {
		rows[idx] = table.Row{rr.User, strconv.Itoa(rr.Result)}
	}
	return rows
}

func (c *client) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case []pkg.RollResult:
		slog.Debug("roll result")
		c.table.SetHeight(len(msg) + 1)
		c.table.SetRows(resultsToRows(msg))
		return c, c.readUpdate()
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			c.done <- struct{}{}
			return c, tea.Quit
		}
	case error:
		slog.Error("exiting for error", "error", msg)
		c.err = msg
		return c, tea.Quit
	default:
		slog.Debug("unsupported message", "msg", msg)
	}
	slog.Debug("no update")
	return c, nil
}

func (c *client) View() string {
	slog.Debug("rerendering view")
	if c.err != nil {
		return fmt.Sprintln(c.err)
	}
	return baseStyle.Render(c.table.View()) + "\n"
}

func (c *client) readUpdate() tea.Cmd {
	slog.Debug("reading update")
	return func() tea.Msg {
		slog.Debug("reading from channel")
		update := <-c.updates
		slog.Debug("read from channel")
		return update
	}
}

func waitClose(conn *websocket.Conn, done <-chan struct{}) {
	<-done
	slog.Debug("closing connection")
	err := conn.WriteControl(
		websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
		time.Now().Add(time.Second),
	)
	if err != nil {
		slog.Error("close control message failed", "error", err)
	}
}

func updateLoop(conn *websocket.Conn, updates chan<- []pkg.RollResult) {
	slog.Debug("running update loop")
	var currentVersion int
	for {
		_, b, err := conn.ReadMessage()
		if err != nil {
			slog.Error(err.Error())
			return
		}
		var room pkg.Room
		err = msgpack.Unmarshal(b, &room)
		if err != nil {
			slog.Error("failed parsing room", "error", err)
			return
		}
		slog.Debug("message recieved", "room", room)
		if currentVersion == room.Version {
			slog.Debug("version hasn't changed, continuing")
			continue
		}

		slog.Debug("new version")
		rolls := make([]pkg.RollResult, len(room.Rolls))
		var idx int
		for _, rr := range room.Rolls {
			rolls[idx] = rr
			idx++
		}
		slog.Debug("pushing rolls on channel")
		updates <- rolls
		currentVersion = room.Version
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
