# Old Forum Style Refactor Design

## Goal

Refactor the current real-time forum code so it follows the architecture of the previous `forum/` project while preserving the current audit-facing behavior. The refactor should make the code easier to read, test, and learn from, especially the WebSocket flow.

This is not a rewrite from scratch. The current project already implements most required behavior and has passing tests. The work should reorganize that behavior into clearer files and packages.

## Constraints

- Keep the real-time forum audit requirements intact.
- Keep the single-page frontend: one HTML file, plain JavaScript, and CSS.
- Use only allowed backend packages: Go standard library, Gorilla WebSocket, SQLite driver, and bcrypt.
- Follow the previous forum's simple package style instead of introducing a large service/repository architecture.
- Keep tests passing during the refactor.

## Architecture

The backend should be split into packages similar to the previous forum:

- `models/`: shared data structs such as `User`, `Post`, `Comment`, `Message`, and request payload structs where useful.
- `database/`: SQLite setup, schema loading, and database functions for users, sessions, categories, posts, comments, and messages.
- `handlers/`: HTTP routing helpers and API handlers. Handlers receive shared dependencies and call database functions directly, matching the previous forum style.
- `realtime/`: WebSocket hub, connection tracking, online-user state, and direct private-message delivery.
- `main.go`: configuration, database initialization, route registration, static file serving, and server startup.

The schema should move from the inline `schemaSQL` string in `app.go` into `database/schema.sql`, matching the old forum project.

## Data Flow

Authentication stays cookie-session based:

1. A user registers or logs in through JSON API routes.
2. The handler validates input and calls the database package.
3. A session row is created and a secure session cookie is returned.
4. Authenticated HTTP routes resolve the current user from the session cookie.
5. The WebSocket route also resolves the current user before upgrading the connection.

Posts and comments stay request/response based:

1. The frontend sends JSON to API handlers.
2. Handlers validate the current user and request body.
3. Database functions create or list records.
4. Handlers return JSON models for the SPA to render.

Private messages use both HTTP and WebSockets:

1. Opening a conversation loads the latest 10 messages over HTTP.
2. Scrolling upward loads 10 older messages using the existing throttled frontend scroll handler.
3. Sending a message goes through the authenticated WebSocket connection.
4. The backend persists the message first, then sends the saved message to both sender and receiver if connected.
5. The user list continues to include online state and ordering by most recent conversation activity, falling back to nickname order for users without messages.

## Error Handling

Handlers should keep the current JSON error style:

- `400` for invalid request bodies or missing required fields.
- `401` for unauthenticated users.
- `405` for unsupported methods where routes have a fixed method.
- `500` for unexpected database or server failures.

Database functions should return Go errors without writing HTTP responses. Handlers own HTTP status codes and response formatting.

The WebSocket loop should ignore malformed or invalid message payloads without crashing the server. Connection read errors should close only that socket.

## Testing

Existing tests should be migrated with the code they cover rather than removed. The target verification command is:

```sh
go test ./...
```

The refactor should preserve coverage for:

- full registration requirements and login by nickname or email
- rejected invalid registration data
- authenticated and unauthenticated route behavior
- post and comment creation/listing
- chat user ordering and online state
- private-message persistence, pagination, and WebSocket delivery
- frontend audit details that are currently checked by tests

## Out Of Scope

- Rebuilding the project from scratch.
- Adding frontend frameworks.
- Adding new bonus features such as profiles or image messages.
- Changing the UI design except where needed to keep behavior working after route or payload cleanup.
- Introducing a complex production framework or dependency injection layer.
