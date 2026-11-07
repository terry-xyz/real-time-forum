# CLAUDE.md

Rules for AI agents and contributors working in this repository.

## UI work stays in `ui/`

All UI improvements go in the dedicated `ui/` directory at the repo root. The existing code must stay as it is.

- **Do not modify, move, rename, or delete** anything outside `ui/`. That includes `static/`, `handlers/`, `models/`, `database/`, `main.go`, `go.mod`, and `go.sum`.
- **Create new files only inside `ui/`.** Copy files from `static/` into `ui/` if you want to build on them, and leave the originals untouched.
- **Use the existing backend API as it is** (`/api/register`, `/api/login`, `/api/logout`, `/api/check-auth`, `/api/categories`, `/api/posts`, `/api/comments`, `/api/chat/users`, `/api/messages`, `/api/ws`). Do not change endpoints, request/response shapes, or the database schema to fit the UI.
- **Ask first** before any change outside `ui/`, even a one-line one, such as pointing `main.go` at `ui/` instead of `static/`. Explain why the change is needed and wait for approval.
