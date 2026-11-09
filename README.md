# Real-Time Forum

A small forum built with Go, SQLite, and a browser-based single-page interface. Members can publish categorized posts, discuss them in comments, view their profile and chat with other members through real-time direct messages.

## Features

- Account registration and sign-in with nickname or email
- Cookie-backed sessions and bcrypt password hashing
- Post feed with category filtering and comments
- Profile view with the signed-in user's posts
- Direct messaging with conversation history, online status, and typing indicators
- Light and dark themes

## Requirements

- Go 1.26 or newer (see `go.mod`)
- A C compiler for the SQLite driver (`github.com/mattn/go-sqlite3` uses CGO)

## Run locally

From the project root:

```sh
go run .
```

The server listens on `http://localhost:8080`. Open that address in a browser and create an account. SQLite creates `forum.db` in the project root on first launch; the database schema is initialized automatically.

To build a standalone executable:

```sh
go build -o real-time-forum .
./real-time-forum
```

Run the executable from the project root so it can find the `static/` frontend directory. The database path is currently set to `./forum.db` in `main.go`, and the listen address is `:8080`.

## API overview

The frontend is served from `static/`. The Go server exposes these endpoints:

| Method | Endpoint | Purpose |
| --- | --- | --- |
| `POST` | `/api/register` | Create an account |
| `POST` | `/api/login` | Sign in and create a session |
| `POST` | `/api/logout` | End the current session |
| `GET` | `/api/check-auth` | Check the current session |
| `GET` | `/api/categories` | List post categories |
| `GET`, `POST` | `/api/posts` | List posts or create a post |
| `GET`, `POST` | `/api/comments` | List or add comments (`post_id` query parameter for listing) |
| `GET` | `/api/chat/users` | List other members and their online status |
| `GET`, `POST` | `/api/messages` | Load conversation history or send a direct message |
| WebSocket | `/api/ws` | Receive live messages and exchange typing events |

Post creation, comments, chat, and session checks require the `session_token` cookie set at login. Posts can be filtered by category with `/api/posts?category=name`. Message history accepts a peer's ID in `with`; it returns up to 10 messages per page and supports `before_id` pagination.

## Project layout

```text
.
├── database/    SQLite initialization and schema
├── handlers/    HTTP and WebSocket handlers
├── models/      API and database data structures
├── static/      HTML, CSS, and JavaScript frontend
├── main.go      Server setup and route registration
└── go.mod       Go module and dependencies
```

## Data storage

The application stores users, sessions, categories, posts, comments, and messages in SQLite. The database is configured with foreign keys and WAL journaling. To reset local data, stop the server and remove `forum.db` (and any SQLite sidecar files); the next launch will create a fresh database.
