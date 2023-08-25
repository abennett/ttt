package main

import (
	"cmp"
	"context"
	"log/slog"
	"slices"
	"strconv"
	"time"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/gorilla/websocket"

	"github.com/abennett/ttt/pkg"
)

var baseStyle = lipgloss.NewStyle().
	BorderStyle(lipgloss.NormalBorder()).
	BorderForeground(lipgloss.Color("240"))

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

func newClient(endpoint, user string) (client, error) {
	var c client
	req := pkg.RollRequest{
		User: user,
	}
	conn, _, err := websocket.DefaultDialer.Dial(endpoint, nil)
	if err != nil {
		return c, err
	}
	err = conn.WriteJSON(req)
	if err != nil {
		return c, err
	}
	updates := make(chan []pkg.RollResult, 1)
	cmd := updateLoop(conn, updates)
	table := table.New(table.WithColumns(columns))
	return client{
		cmd:      cmd,
		table:    table,
		endpoint: endpoint,
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
		c.table = table.New(table.WithColumns(columns), table.WithRows(resultsToRows(msg)))
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
	c, err := newClient(args[0], args[1])
	if err != nil {
		return err
	}
	_, err = tea.NewProgram(c).Run()
	return err
}
