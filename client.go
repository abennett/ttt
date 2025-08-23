package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/gorilla/websocket"
	"github.com/vmihailenco/msgpack/v5"

	"github.com/abennett/ttt/pkg/client"
	"github.com/abennett/ttt/pkg/messages"
)

var ErrTooManyRedirects = errors.New("too many redirects")

var baseStyle = lipgloss.NewStyle().
	BorderStyle(lipgloss.NormalBorder()).
	Align(lipgloss.Center)

var columns = []table.Column{
	{Title: "User", Width: 10},
	{Title: "Result", Width: 6},
}

type ttt struct {
	client *client.Client
	table  table.Model
	done   chan struct{}
}

func newTTT(c *client.Client) (*ttt, error) {
	t := table.New(
		table.WithColumns(columns),
		table.WithHeight(0),
		table.WithFocused(false),
	)
	s := table.DefaultStyles()
	s.Header = s.Header.Foreground(lipgloss.Color("#01c5d1"))
	s.Selected = s.Selected.Foreground(lipgloss.NoColor{}).Bold(false)
	t.SetStyles(s)
	return &ttt{
		client: c,
		table:  t,
		done:   make(chan struct{}),
	}, nil
}

func errorCmd(err error) tea.Cmd {
	return func() tea.Msg {
		return err
	}
}

func (t *ttt) Init() tea.Cmd {
	err := t.client.Init()
	if err != nil {
		panic(err)
	}
	return func() tea.Msg {
		return t.client.ReadUpdate()
	}
}

func resultsToRows(rrs []messages.RollResult) []table.Row {
	rows := make([]table.Row, len(rrs))
	for idx, rr := range rrs {
		rows[idx] = table.Row{rr.User, strconv.Itoa(rr.Result)}
	}
	return rows
}

func (t *ttt) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case []messages.RollResult:
		slog.Debug("roll result")
		t.table.SetHeight(len(msg) + 1)
		t.table.SetRows(resultsToRows(msg))
		return t, t.readUpdate()
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			c.done <- struct{}{}
			return c, tea.Quit

		// Attempt to update done index
		case " ":
			doneReq := messages.DoneRequest{DoneIndex: 0}
			m := messages.Message{
				Type:    messages.DoneRequestType,
				Version: "1",
				Payload: doneReq,
			}
			b, err := msgpack.Marshal(m)
			if err != nil {
				panic(err)
			}
			err = c.conn.WriteMessage(websocket.BinaryMessage, b)
			if err != nil {
				panic(err)
			}
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
		msg := <-c.messages
		slog.Debug("read from channel")
		switch payload := msg.Payload.(type) {
		case messages.RoomState:
			if payload.Version == c.roomVersion {
				slog.Debug("version hasn't changed, continuing")
			}

			slog.Debug("new version")
			rolls := make([]messages.RollResult, len(payload.Rolls))
			for idx, rr := range payload.Rolls {
				rolls[idx] = rr
			}
			slog.Debug("pushing rolls on channel")
			c.roomVersion = payload.Version
			return payload.Rolls
		case messages.DoneRequest:
			c.doneIndex = payload.DoneIndex
		default:
			panic(fmt.Sprintf("unexpected messages.Type: %#v", msg.Type))
		}
		return msg
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
