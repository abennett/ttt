package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strconv"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

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
	{Title: "Done", Width: 6},
}

type ttt struct {
	client *client.Client
	table  table.Model
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
		if rr.IsDone {
			rows[idx] = table.Row{rr.User, strconv.Itoa(rr.Result), "✅"}
		} else {
			rows[idx] = table.Row{rr.User, strconv.Itoa(rr.Result), ""}
		}
	}
	return rows
}

func (t *ttt) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case []messages.RollResult:
		slog.Debug("roll result")
		t.table.SetHeight(len(msg) + 1)
		t.table.SetRows(resultsToRows(msg))
		for _, rr := range msg {
			if !rr.IsDone {
				return t, func() tea.Msg {
					return t.client.ReadUpdate()
				}
			}
		}
		return t, tea.Quit
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			err := t.client.Close()
			if err != nil {
				slog.Error("failed to close client", "error", err)
			}
			return t, tea.Quit

		// Attempt to update done index
		case " ":
			err := t.client.SubmitDone()
			if err != nil {
				panic(err)
			}
		}
	case error:
		slog.Error("exiting for error", "error", msg)
		return t, tea.Quit
	default:
		slog.Debug("unsupported message", "msg", msg)
	}
	slog.Debug("no update")
	return t, nil
}

func (t *ttt) View() string {
	slog.Debug("rerendering view")
	return baseStyle.Render(t.table.View()) + "\n"
}

func rollRemote(_ context.Context, args []string) error {
	c, err := client.New(args[0], args[1], args[2], io.Discard)
	if err != nil {
		return err
	}
	ttt, err := newTTT(c)
	if err != nil {
		return err
	}

	_, err = tea.NewProgram(ttt).Run()
	return err
}
