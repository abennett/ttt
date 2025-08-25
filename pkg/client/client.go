package client

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/vmihailenco/msgpack/v5"

	"github.com/abennett/ttt/pkg/messages"
)

var ErrTooManyRedirects = errors.New("too many redirects")

type Client struct {
	mu   *sync.Mutex
	user string

	conn     *websocket.Conn
	logger   *slog.Logger
	messages chan messages.Message

	Room messages.RoomState
}

func connectLoop(wsUrl string) (*websocket.Conn, error) {
	for range 3 {
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

func setupLogger(user string, logWriter io.Writer) *slog.Logger {
	h := slog.NewTextHandler(logWriter, &slog.HandlerOptions{Level: slog.LevelDebug})
	logger := slog.New(h)
	slog.SetDefault(logger)
	logger = logger.With("user", user)
	return logger
}

func New(host, room, user string, logWriter io.Writer) (*Client, error) {
	logger := setupLogger(user, logWriter)

	endpoint, err := hostUrl(host, room)
	if err != nil {
		return nil, err
	}
	slog.Debug("using endpoint", "endpoint", endpoint)

	conn, err := connectLoop(endpoint)
	if err != nil {
		return nil, err
	}

	return &Client{
		mu:       new(sync.Mutex),
		user:     user,
		logger:   logger,
		conn:     conn,
		messages: make(chan messages.Message, 1),
		Room: messages.RoomState{
			Rolls: []messages.RollResult{},
		},
	}, nil
}

func (c *Client) Init() error {
	c.logger.Debug("running Init")
	req := messages.Message{
		Type: messages.RollRequestType,
		Payload: messages.RollResult{
			User: c.user,
		},
	}
	b, err := msgpack.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal: %w", err)
	}

	c.logger.Debug("writing initial message")
	err = c.conn.WriteMessage(websocket.BinaryMessage, b)
	if err != nil {
		return fmt.Errorf("unable to write server: %w", err)
	}

	go c.updateLoop(c.messages)
	return nil
}

func (c *Client) SubmitDone() error {
	doneReq := messages.DoneRequest{
		User: c.user,
	}
	m := messages.Message{
		Type:    messages.DoneRequestType,
		Version: "1",
		Payload: doneReq,
	}
	b, err := msgpack.Marshal(m)
	if err != nil {
		return err
	}
	err = c.conn.WriteMessage(websocket.BinaryMessage, b)
	if err != nil {
		return err
	}
	return nil
}

func (c *Client) ReadUpdate() any {
	c.logger.Debug("reading update")
	msg := <-c.messages
	c.logger.Debug("read from channel")
	switch payload := msg.Payload.(type) {
	case messages.RoomState:
		c.logger.Debug("room state message received", "version", payload.Version)
		if payload.Version <= c.Room.Version {
			c.logger.Debug("version hasn't changed, continuing")
			// return early or something
			return payload.Rolls
		}

		c.logger.Debug("new version")
		c.logger.Debug("pushing rolls on channel")
		c.Room = payload
		return payload.Rolls
	case messages.DoneRequest:
		panic("not implemented")
	default:
		panic(fmt.Sprintf("unexpected messages.Type: %#v", msg.Type))
	}
}

func (c *Client) Close() error {
	slog.Debug("closing connection")
	err := c.conn.WriteControl(
		websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
		time.Now().Add(time.Second),
	)
	if err != nil {
		slog.Error("close control message failed", "error", err)
		return fmt.Errorf("close control message failed: %w", err)
	}
	return nil
}

func (c *Client) updateLoop(updates chan<- messages.Message) {
	c.logger.Debug("running update loop")
	for {
		t, b, err := c.conn.ReadMessage()
		if err != nil {
			c.logger.Error(err.Error())
			return
		}
		if t != websocket.BinaryMessage {
			continue
		}
		c.logger.Debug("client recevied message")
		var msg messages.Message
		err = msgpack.Unmarshal(b, &msg)
		if err != nil {
			c.logger.Error("failed parsing room", "error", err, "payload", b)
			return
		}
		c.logger.Debug("message recieved", "type", msg.Type)
		switch payload := msg.Payload.(type) {
		case messages.RoomState:
			c.logger.Debug("new room version", "version", payload.Version)
			c.mu.Lock()
			c.Room = payload
			c.mu.Unlock()
		default:
			panic(fmt.Sprintf("support not implemented for %T", payload))
		}
		updates <- msg
	}
}
