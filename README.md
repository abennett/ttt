# ttt - Turn Taking Tool

`ttt` is a real-time, multiplayer CLI tool designed to manage turn order and initiative for team meetings. Built with Go, it uses WebSockets for low-latency updates and to bring a bit more joy into your day.

## Features

- **Real-time Synchronization:** See rolls and turn updates from all players instantly.
- **TUI Interface:** Clean, table-based interface built with Bubble Tea.
- **Dice Parsing:** Support for standard dice notation (e.g., `1d20+5`, `2d6-1`).
- **Room-based organization:** Multiple separate games can be hosted on a single server.
- **Binary Protocol:** Uses `msgpack` over WebSockets for efficient communication.

## Installation

Ensure you have [Go](https://go.dev/dl/) installed (version 1.24 or later).

```bash
go install github.com/abennett/ttt@latest
```

## Usage

### 1. Start the Server

To host a session, start the `ttt` server:

```bash
ttt serve --port 8080
```

### 2. Join a Room

Players can join a room by providing the server URL, a room name, and their username:

```bash
ttt roll http://localhost:8080 my-game-room Alice
```

Once in the room, `ttt` will automatically roll initiative for you (based on the room's default dice).

**Controls:**
- `Space`: Toggle your "Done" status (useful for tracking who has taken their turn).
- `q` or `Ctrl+C`: Quit the session.

### 3. Local Dice Rolling

You can also use `ttt` as a simple local dice roller:

```bash
ttt roll_local 2d20+5
```

## Technical Architecture

- **Backend:** Go using `chi` for HTTP routing and `gorilla/websocket` for real-time communication.
- **Frontend:** Terminal UI built with `charmbracelet/bubbletea`, `bubbles`, and `lipgloss`.
- **Serialization:** `vmihailenco/msgpack` for compact binary messaging.
- **CLI Framework:** `peterbourgon/ff` for robust command-line argument parsing.
