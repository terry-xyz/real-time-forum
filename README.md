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

- Go 1.26.11 or newer (see `go.mod`)
- GCC on your PATH and CGO enabled (`CGO_ENABLED=1`) to build the SQLite driver

## Run locally

From the project root:

```sh
make run
```

On Windows, `make run` tries the configured Go compiler settings first. If the build fails and w64devkit is installed at `C:\w64devkit`, Make retries with its GCC. If you run Go directly, you can select that compiler in PowerShell:

```powershell
$env:CC = 'C:\w64devkit\bin\gcc.exe'
go run .
```

The server listens on `http://localhost:8080`. Open that address in a browser and create an account. SQLite creates `forum.db` in the project root on first launch; the database schema is initialized automatically.

To build a standalone executable:

```sh
go build -o real-time-forum .
./real-time-forum
```

Run the executable from the project root so it can find the `static/` frontend directory and `forum.db`. HTTP listens on localhost for development. To serve HTTPS, set `FORUM_TLS_CERT` and `FORUM_TLS_KEY` to the certificate and private key paths. Set `FORUM_ADDR` to an address such as `:443` to accept HTTPS connections beyond localhost. If you use a proxy, preserve the public host and HTTPS connection to the app. Request limits use the connecting IP, which is the proxy's IP in that setup.

## API overview

The frontend is served from `static/`. The Go server exposes these endpoints:

| Method | Endpoint | Purpose |
| --- | --- | --- |
| `POST` | `/api/register` | Create an account (age 13–120, gender Male or Female) |
| `POST` | `/api/login` | Sign in and create a session |
| `POST` | `/api/logout` | End the current session |
| `GET` | `/api/check-auth` | Check the current session |
| `GET` | `/api/categories` | List post categories |
| `GET`, `POST` | `/api/posts` | List posts or create a post |
| `GET`, `POST` | `/api/comments` | List or add comments (`post_id` query parameter for listing) |
| `GET` | `/api/chat/users` | List other members and their online status |
| `GET`, `POST` | `/api/messages` | Load conversation history or send a direct message |
| WebSocket | `/api/ws` | Receive live messages and exchange typing events |

All forum reads and writes require the `session_token` cookie set at login. JSON requests must set `Content-Type: application/json`. New passwords must be 12–72 bytes. Posts can be filtered by category with `/api/posts?category=name`. Message history accepts a peer's ID in `with` and returns up to 10 messages per page. Pass `before_id` to load an older page.

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
