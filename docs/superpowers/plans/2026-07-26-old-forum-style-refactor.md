# Old Forum Style Refactor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Refactor the current single-file backend into the previous forum project's package style without changing audit-facing behavior.

**Architecture:** Keep the simple old-forum pattern: handlers receive shared dependencies and call database functions directly. Move shared structs to `models`, SQLite/schema/query functions to `database`, websocket connection tracking to `realtime`, and keep `main.go` focused on startup and route registration.

**Tech Stack:** Go 1.24, standard `net/http`, SQLite through `github.com/mattn/go-sqlite3`, `github.com/gorilla/websocket`, `golang.org/x/crypto/bcrypt`, plain HTML/CSS/JavaScript.

---

## File Structure

- Create `models/user.go`: `RegistrationInput` and `User`.
- Create `models/post.go`: `Post`.
- Create `models/comment.go`: `Comment`.
- Create `models/message.go`: `Message`.
- Create `database/database.go`: `InitDB`, SQLite DSN/config, schema loading, and timestamp helpers.
- Create `database/schema.sql`: existing schema from `app.go`.
- Create `database/users.go`: registration validation, user creation, lookup, and public user projection.
- Create `database/sessions.go`: session creation/deletion/current user lookup.
- Create `database/categories.go`: category listing.
- Create `database/posts.go`: post creation/listing and category splitting.
- Create `database/comments.go`: comment creation/listing.
- Create `database/messages.go`: message creation, chat-user ordering, pagination, and ID lookups.
- Create `realtime/hub.go`: websocket client map, add/remove/send/online functions.
- Create `handlers/app.go`: `App`, `NewApp`, `Routes`, static index serving, security headers, JSON helpers.
- Create `handlers/auth.go`: register, login, logout, and current-user handlers.
- Create `handlers/posts.go`: posts and comments HTTP handlers.
- Create `handlers/chat.go`: users and messages HTTP handlers plus websocket handler.
- Modify `main.go`: use `database`, `handlers`, and `realtime` packages.
- Modify `app_test.go`: update imports and helper calls to package-qualified names.
- Delete `app.go` after all functionality has moved.

---

### Task 1: Move Models

**Files:**
- Create: `models/user.go`
- Create: `models/post.go`
- Create: `models/comment.go`
- Create: `models/message.go`
- Modify: `app.go`
- Test: `go test ./...`

- [ ] **Step 1: Create `models/user.go`**

```go
package models

import "time"

type RegistrationInput struct {
	Nickname  string `json:"nickname"`
	Age       int    `json:"age"`
	Gender    string `json:"gender"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Email     string `json:"email"`
	Password  string `json:"password"`
}

type User struct {
	ID        int        `json:"id"`
	Nickname  string     `json:"nickname"`
	Age       int        `json:"age,omitempty"`
	Gender    string     `json:"gender,omitempty"`
	FirstName string     `json:"first_name,omitempty"`
	LastName  string     `json:"last_name,omitempty"`
	Email     string     `json:"email,omitempty"`
	Online    bool       `json:"online,omitempty"`
	LastChat  *time.Time `json:"last_message_at,omitempty"`
}
```

- [ ] **Step 2: Create `models/post.go`**

```go
package models

import "time"

type Post struct {
	ID         int       `json:"id"`
	Title      string    `json:"title"`
	Content    string    `json:"content"`
	Author     User      `json:"author"`
	Categories []string  `json:"categories"`
	CreatedAt  time.Time `json:"created_at"`
}
```

- [ ] **Step 3: Create `models/comment.go`**

```go
package models

import "time"

type Comment struct {
	ID        int       `json:"id"`
	PostID    int       `json:"post_id"`
	Content   string    `json:"content"`
	Author    User      `json:"author"`
	CreatedAt time.Time `json:"created_at"`
}
```

- [ ] **Step 4: Create `models/message.go`**

```go
package models

import "time"

type Message struct {
	ID        int       `json:"id"`
	From      User      `json:"from"`
	To        User      `json:"to"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}
```

- [ ] **Step 5: Update `app.go` to import and use `models`**

Add the import:

```go
import "real-time-forum/models"
```

Remove the local `RegistrationInput`, `User`, `Post`, `Comment`, and `Message` type declarations from `app.go`.

Replace type references:

```go
RegistrationInput -> models.RegistrationInput
User              -> models.User
Post              -> models.Post
Comment           -> models.Comment
Message           -> models.Message
```

- [ ] **Step 6: Run tests**

Run:

```sh
go test ./...
```

Expected: all packages pass.

- [ ] **Step 7: Commit**

```sh
git add app.go models
git commit -m "refactor(models): move shared forum models"
```

---

### Task 2: Move Database Schema And Query Functions

**Files:**
- Create: `database/schema.sql`
- Create: `database/database.go`
- Create: `database/users.go`
- Create: `database/sessions.go`
- Create: `database/categories.go`
- Create: `database/posts.go`
- Create: `database/comments.go`
- Create: `database/messages.go`
- Modify: `app.go`
- Test: `go test ./...`

- [ ] **Step 1: Create `database/schema.sql`**

Move the full SQL currently stored in `const schemaSQL` from `app.go` into `database/schema.sql`, starting with:

```sql
CREATE TABLE IF NOT EXISTS users (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	nickname TEXT NOT NULL UNIQUE,
	age INTEGER NOT NULL,
	gender TEXT NOT NULL,
	first_name TEXT NOT NULL,
	last_name TEXT NOT NULL,
	email TEXT NOT NULL UNIQUE,
	password_hash TEXT NOT NULL,
	created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
```

The file must also include the existing `sessions`, `categories`, `posts`, `post_categories`, `comments`, and `messages` tables, the four default category inserts, and the existing indexes.

- [ ] **Step 2: Create `database/database.go`**

```go
package database

import (
	"database/sql"
	"fmt"
	"os"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

const defaultDBPath = "real-time-forum.db"

func InitDB(path string) (*sql.DB, error) {
	if strings.TrimSpace(path) == "" {
		path = defaultDBPath
	}
	db, err := sql.Open("sqlite3", sqliteDSN(path))
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	schema, err := os.ReadFile("database/schema.sql")
	if err != nil {
		db.Close()
		return nil, err
	}
	if _, err := db.Exec(string(schema)); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func sqliteDSN(path string) string {
	separator := "?"
	if strings.Contains(path, "?") {
		separator = "&"
	}
	return path + separator + "_foreign_keys=on&_busy_timeout=5000"
}

func parseDBTime(value string) (time.Time, error) {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05"} {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid database timestamp %q", value)
}
```

- [ ] **Step 3: Create database files by moving existing functions**

Move these functions from `app.go` to `database/users.go` and export only the ones handlers/tests need:

```go
func CreateUser(db *sql.DB, input models.RegistrationInput) (int, error)
func UserByIdentifier(db *sql.DB, identifier string) (models.User, string, error)
func UserByID(db *sql.DB, id int) (models.User, error)
func PublicUser(user models.User) models.User
```

Keep these helper functions unexported in `database/users.go`:

```go
func validateRegistration(input models.RegistrationInput) (models.RegistrationInput, error)
func validEmail(value string) bool
```

Move session functions to `database/sessions.go`:

```go
const SessionCookieName = "rtf_session"

var ErrUnauthorized = errors.New("unauthorized")

func CreateSession(db *sql.DB, userID int) (string, error)
func DeleteSession(db *sql.DB, sessionID string) error
func CurrentUser(db *sql.DB, sessionID string) (models.User, error)
```

Move category loading to `database/categories.go`:

```go
type Category struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

func ListCategories(db *sql.DB) ([]Category, error)
```

Move post functions to `database/posts.go`:

```go
func CreatePost(db *sql.DB, authorID int, title string, content string, categoryIDs []int) (models.Post, error)
func ListPosts(db *sql.DB) ([]models.Post, error)
func ListPostByID(db *sql.DB, postID int) (models.Post, error)
```

Move comment functions to `database/comments.go`:

```go
func CreateComment(db *sql.DB, authorID int, postID int, content string) (models.Comment, error)
func ListComments(db *sql.DB, postID int) ([]models.Comment, error)
```

Move message functions to `database/messages.go`:

```go
func CreateMessage(db *sql.DB, fromID int, toID int, content string, createdAt time.Time) (int, error)
func CreateMessageModel(db *sql.DB, fromID int, toID int, content string, createdAt time.Time) (models.Message, error)
func ChatUsers(db *sql.DB, currentUserID int, online map[int]bool) ([]models.User, error)
func ListMessages(db *sql.DB, currentUserID int, otherUserID int, beforeID int, limit int) ([]models.Message, error)
```

- [ ] **Step 4: Update `app.go` call sites**

Add the import:

```go
import "real-time-forum/database"
```

Replace calls:

```go
InitDB               -> database.InitDB
CreateUser           -> database.CreateUser
CreateMessage        -> database.CreateMessage
createSession        -> database.CreateSession
userByIdentifier     -> database.UserByIdentifier
userByID             -> database.UserByID
publicUser           -> database.PublicUser
createPost           -> database.CreatePost
listPosts            -> database.ListPosts
listComments         -> database.ListComments
createComment        -> database.CreateComment
chatUsers            -> database.ChatUsers
createMessageModel   -> database.CreateMessageModel
listMessages         -> database.ListMessages
sessionCookieName    -> database.SessionCookieName
errUnauthorized      -> database.ErrUnauthorized
```

Replace inline category query in `handleCategories` with:

```go
categories, err := database.ListCategories(a.db)
if err != nil {
	writeError(w, http.StatusInternalServerError, "unable to load categories")
	return
}
writeJSON(w, http.StatusOK, map[string]any{"categories": categories})
```

- [ ] **Step 5: Run tests**

Run:

```sh
go test ./...
```

Expected: all packages pass.

- [ ] **Step 6: Commit**

```sh
git add app.go database
git commit -m "refactor(database): move sqlite forum operations"
```

---

### Task 3: Move Realtime Hub

**Files:**
- Create: `realtime/hub.go`
- Modify: `app.go`
- Test: `go test ./...`

- [ ] **Step 1: Create `realtime/hub.go`**

```go
package realtime

import (
	"sync"

	"github.com/gorilla/websocket"
)

type Hub struct {
	mu      sync.RWMutex
	clients map[int]map[*websocket.Conn]bool
}

func NewHub() *Hub {
	return &Hub{clients: make(map[int]map[*websocket.Conn]bool)}
}

func (h *Hub) Add(userID int, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.clients[userID] == nil {
		h.clients[userID] = make(map[*websocket.Conn]bool)
	}
	h.clients[userID][conn] = true
}

func (h *Hub) Remove(userID int, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.clients[userID], conn)
	if len(h.clients[userID]) == 0 {
		delete(h.clients, userID)
	}
}

func (h *Hub) Send(userID int, payload any) {
	h.mu.RLock()
	var conns []*websocket.Conn
	for conn := range h.clients[userID] {
		conns = append(conns, conn)
	}
	h.mu.RUnlock()
	for _, conn := range conns {
		_ = conn.WriteJSON(payload)
	}
}

func (h *Hub) OnlineIDs() map[int]bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	online := make(map[int]bool, len(h.clients))
	for userID := range h.clients {
		online[userID] = true
	}
	return online
}
```

- [ ] **Step 2: Update `app.go` to use `realtime.Hub`**

Add:

```go
import "real-time-forum/realtime"
```

Change:

```go
type App struct {
	db  *sql.DB
	hub *realtime.Hub
}

func NewApp(db *sql.DB) *App {
	return &App{db: db, hub: realtime.NewHub()}
}
```

Remove the local `Hub` type and its methods from `app.go`.

- [ ] **Step 3: Run tests**

Run:

```sh
go test ./...
```

Expected: all packages pass.

- [ ] **Step 4: Commit**

```sh
git add app.go realtime
git commit -m "refactor(realtime): move websocket hub"
```

---

### Task 4: Move Handlers And App Routing

**Files:**
- Create: `handlers/app.go`
- Create: `handlers/auth.go`
- Create: `handlers/posts.go`
- Create: `handlers/chat.go`
- Modify: `main.go`
- Modify: `app.go`
- Test: `go test ./...`

- [ ] **Step 1: Create `handlers/app.go`**

```go
package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"path/filepath"

	"real-time-forum/realtime"
)

type App struct {
	db  *sql.DB
	hub *realtime.Hub
}

func NewApp(db *sql.DB) *App {
	return &App{db: db, hub: realtime.NewHub()}
}

func (a *App) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", a.handleIndex)
	mux.HandleFunc("/api/register", a.handleRegister)
	mux.HandleFunc("/api/login", a.handleLogin)
	mux.HandleFunc("/api/logout", a.handleLogout)
	mux.HandleFunc("/api/me", a.handleMe)
	mux.HandleFunc("/api/categories", a.handleCategories)
	mux.HandleFunc("/api/posts", a.handlePosts)
	mux.HandleFunc("/api/posts/", a.handlePostChildren)
	mux.HandleFunc("/api/users", a.handleUsers)
	mux.HandleFunc("/api/messages", a.handleMessages)
	mux.HandleFunc("/ws", a.handleWebSocket)
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
	return securityHeaders(mux)
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		next.ServeHTTP(w, r)
	})
}

func (a *App) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, filepath.Join("static", "index.html"))
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(target); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{"error": message})
}
```

- [ ] **Step 2: Move auth handlers to `handlers/auth.go`**

Move `handleRegister`, `handleLogin`, `handleLogout`, `handleMe`, `currentUser`, and `sessionCookie` into `handlers/auth.go`.

Use these imports:

```go
package handlers

import (
	"net/http"
	"strings"
	"time"

	"real-time-forum/database"
	"real-time-forum/models"

	"golang.org/x/crypto/bcrypt"
)
```

`currentUser` should call:

```go
cookie, err := r.Cookie(database.SessionCookieName)
if err != nil || cookie.Value == "" {
	return models.User{}, database.ErrUnauthorized
}
return database.CurrentUser(a.db, cookie.Value)
```

- [ ] **Step 3: Move post and comment handlers to `handlers/posts.go`**

Move `handleCategories`, `handlePosts`, and `handlePostChildren`.

Use these imports:

```go
package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"real-time-forum/database"
)
```

- [ ] **Step 4: Move chat and websocket handlers to `handlers/chat.go`**

Move `handleUsers`, `handleMessages`, and `handleWebSocket`.

Use these imports:

```go
package handlers

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"real-time-forum/database"

	"github.com/gorilla/websocket"
)
```

The websocket send payload should remain:

```go
map[string]any{
	"type":       "private_message",
	"message":    message,
	"content":    message.Content,
	"from":       database.PublicUser(message.From),
	"to":         database.PublicUser(message.To),
	"created_at": message.CreatedAt,
}
```

- [ ] **Step 5: Update `main.go`**

Replace its contents with:

```go
package main

import (
	"fmt"
	"net/http"
	"os"
	"strings"

	"real-time-forum/database"
	"real-time-forum/handlers"
)

const (
	defaultPort   = "8080"
	defaultDBPath = "real-time-forum.db"
)

func main() {
	if err := run(); err != nil {
		fmt.Println(err)
	}
}

func configuredAddr() string {
	port := strings.TrimSpace(os.Getenv("PORT"))
	if port == "" {
		port = defaultPort
	}
	if strings.Contains(port, ":") {
		return port
	}
	return ":" + port
}

func configuredDBPath() string {
	path := strings.TrimSpace(os.Getenv("DATABASE_PATH"))
	if path == "" {
		return defaultDBPath
	}
	return path
}

func run() error {
	db, err := database.InitDB(configuredDBPath())
	if err != nil {
		return err
	}
	defer db.Close()
	app := handlers.NewApp(db)
	addr := configuredAddr()
	fmt.Printf("Listening on http://localhost%s\n", addr)
	return http.ListenAndServe(addr, app.Routes())
}
```

- [ ] **Step 6: Leave `app.go` empty of moved behavior**

After the handlers compile from their new package, `app.go` should contain no remaining unique behavior. Delete it in Task 6 after tests have been moved.

- [ ] **Step 7: Run tests**

Run:

```sh
go test ./...
```

Expected: all packages pass or only `app_test.go` compile failures remain because it still references `NewApp`, `InitDB`, `CreateUser`, `RegistrationInput`, or `CreateMessage` from `main`.

- [ ] **Step 8: Commit if tests pass**

If `go test ./...` passes:

```sh
git add app.go main.go handlers
git commit -m "refactor(handlers): move api routing"
```

If tests fail only because `app_test.go` still references moved symbols, skip the commit and continue to Task 5 before committing.

---

### Task 5: Update Tests For New Packages

**Files:**
- Modify: `app_test.go`
- Test: `go test ./...`

- [ ] **Step 1: Update imports in `app_test.go`**

Add:

```go
import (
	"real-time-forum/database"
	"real-time-forum/handlers"
	"real-time-forum/models"
)
```

- [ ] **Step 2: Update test helper setup**

Change:

```go
db, err := InitDB(":memory:")
app := NewApp(db)
```

To:

```go
db, err := database.InitDB(":memory:")
app := handlers.NewApp(db)
```

- [ ] **Step 3: Update direct test database calls**

Replace:

```go
CreateUser(db, RegistrationInput{...})
CreateMessage(db, fromID, toID, content, createdAt)
```

With:

```go
database.CreateUser(db, models.RegistrationInput{...})
database.CreateMessage(db, fromID, toID, content, createdAt)
```

- [ ] **Step 4: Run tests**

Run:

```sh
go test ./...
```

Expected: all packages pass.

- [ ] **Step 5: Commit**

If Task 4 was not committed because of test compile failures, include those files here:

```sh
git add app_test.go app.go main.go handlers
git commit -m "refactor(app_test.go): use split backend packages"
```

If Task 4 was already committed:

```sh
git add app_test.go
git commit -m "refactor(app_test.go): use split backend packages"
```

---

### Task 6: Delete Old Monolith And Verify Audit Behavior

**Files:**
- Delete: `app.go`
- Modify only if needed: `main.go`, `handlers/*.go`, `database/*.go`, `realtime/*.go`
- Test: `go test ./...`

- [ ] **Step 1: Delete `app.go`**

Delete `app.go` only after all moved code compiles from the new packages.

- [ ] **Step 2: Run package tests**

Run:

```sh
go test ./...
```

Expected: all packages pass.

- [ ] **Step 3: Run allowed-package check**

Run:

```sh
go list -deps -f '{{if not .Standard}}{{.ImportPath}}{{end}}' ./... | sed '/^$/d' | sort -u
```

Expected output should include only:

```text
github.com/gorilla/websocket
github.com/mattn/go-sqlite3
golang.org/x/crypto/bcrypt
golang.org/x/crypto/blowfish
real-time-forum
real-time-forum/database
real-time-forum/handlers
real-time-forum/models
real-time-forum/realtime
```

- [ ] **Step 4: Commit**

```sh
git add -A
git commit -m "refactor(app.go): remove backend monolith"
```

---

## Self-Review

- Spec coverage: Tasks move models, database/schema, handlers, realtime hub, tests, and startup exactly as requested. The plan preserves auth, posts/comments, chat users, message pagination, and websocket delivery.
- Placeholder scan: No task relies on unresolved placeholders. The only large move is the schema copy, which references the existing complete SQL source and names every required table/index category.
- Type consistency: New exported names use `database.CreateUser`, `database.CreateMessage`, `database.CurrentUser`, `database.PublicUser`, `handlers.NewApp`, and `realtime.NewHub` consistently across tasks.
