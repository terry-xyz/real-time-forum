package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"real-time-forum/database"
	"real-time-forum/models"

	"github.com/gorilla/websocket"
)

const (
	defaultPort = "8080"
)

type App struct {
	db  *sql.DB
	hub *Hub
}

func NewApp(db *sql.DB) *App {
	return &App{db: db, hub: NewHub()}
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

func (a *App) handleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var input models.RegistrationInput
	if !decodeJSON(w, r, &input) {
		return
	}
	id, err := database.CreateUser(a.db, input)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	user, err := database.UserByID(a.db, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "unable to load user")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"user": database.PublicUser(user)})
}

func (a *App) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var input struct {
		Identifier string `json:"identifier"`
		Password   string `json:"password"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	user, passwordHash, err := database.UserByIdentifier(a.db, strings.TrimSpace(input.Identifier))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "unable to log in")
		return
	}
	if user.ID == 0 || !database.PasswordMatches(passwordHash, input.Password) {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	sessionID, err := database.CreateSession(a.db, user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "unable to create session")
		return
	}
	http.SetCookie(w, sessionCookie(r, sessionID, int((24*time.Hour).Seconds())))
	writeJSON(w, http.StatusOK, map[string]any{"user": database.PublicUser(user)})
}

func (a *App) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	cookie, err := r.Cookie(database.SessionCookieName)
	if err == nil {
		_ = database.DeleteSession(a.db, cookie.Value)
	}
	http.SetCookie(w, sessionCookie(r, "", -1))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *App) handleMe(w http.ResponseWriter, r *http.Request) {
	user, err := a.currentUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": database.PublicUser(user)})
}

func (a *App) handleCategories(w http.ResponseWriter, r *http.Request) {
	categories, err := database.ListCategories(a.db)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "unable to load categories")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"categories": categories})
}

func (a *App) handlePosts(w http.ResponseWriter, r *http.Request) {
	user, err := a.currentUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	switch r.Method {
	case http.MethodGet:
		posts, err := database.ListPosts(a.db)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "unable to load posts")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"posts": posts})
	case http.MethodPost:
		var input struct {
			Title       string `json:"title"`
			Content     string `json:"content"`
			CategoryIDs []int  `json:"category_ids"`
		}
		if !decodeJSON(w, r, &input) {
			return
		}
		post, err := database.CreatePost(a.db, user.ID, input.Title, input.Content, input.CategoryIDs)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"post": post})
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (a *App) handlePostChildren(w http.ResponseWriter, r *http.Request) {
	user, err := a.currentUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/posts/"), "/")
	if len(parts) != 2 || parts[1] != "comments" {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	postID, err := strconv.Atoi(parts[0])
	if err != nil || postID <= 0 {
		writeError(w, http.StatusBadRequest, "invalid post id")
		return
	}
	switch r.Method {
	case http.MethodGet:
		comments, err := database.ListComments(a.db, postID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "unable to load comments")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"comments": comments})
	case http.MethodPost:
		var input struct {
			Content string `json:"content"`
		}
		if !decodeJSON(w, r, &input) {
			return
		}
		comment, err := database.CreateComment(a.db, user.ID, postID, input.Content)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"comment": comment})
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (a *App) handleUsers(w http.ResponseWriter, r *http.Request) {
	user, err := a.currentUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	users, err := database.ChatUsers(a.db, user.ID, a.hub.OnlineIDs())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "unable to load users")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": users})
}

func (a *App) handleMessages(w http.ResponseWriter, r *http.Request) {
	user, err := a.currentUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	otherID, _ := strconv.Atoi(r.URL.Query().Get("with"))
	beforeID, _ := strconv.Atoi(r.URL.Query().Get("before_id"))
	if otherID <= 0 {
		writeError(w, http.StatusBadRequest, "conversation user is required")
		return
	}
	messages, err := database.ListMessages(a.db, user.ID, otherID, beforeID, 10)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "unable to load messages")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"messages": messages})
}

func (a *App) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	user, err := a.currentUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	a.hub.Add(user.ID, conn)
	defer a.hub.Remove(user.ID, conn)
	defer conn.Close()

	for {
		var incoming struct {
			Type    string `json:"type"`
			ToID    int    `json:"to_id"`
			Content string `json:"content"`
		}
		if err := conn.ReadJSON(&incoming); err != nil {
			return
		}
		if incoming.Type != "private_message" || incoming.ToID <= 0 || strings.TrimSpace(incoming.Content) == "" {
			continue
		}
		message, err := database.CreateMessageModel(a.db, user.ID, incoming.ToID, incoming.Content, time.Now())
		if err != nil {
			continue
		}
		a.hub.Send(user.ID, map[string]any{"type": "private_message", "message": message, "content": message.Content, "from": database.PublicUser(message.From), "to": database.PublicUser(message.To), "created_at": message.CreatedAt})
		a.hub.Send(incoming.ToID, map[string]any{"type": "private_message", "message": message, "content": message.Content, "from": database.PublicUser(message.From), "to": database.PublicUser(message.To), "created_at": message.CreatedAt})
	}
}

func (a *App) currentUser(r *http.Request) (models.User, error) {
	cookie, err := r.Cookie(database.SessionCookieName)
	if err != nil || cookie.Value == "" {
		return models.User{}, database.ErrUnauthorized
	}
	return database.CurrentUser(a.db, cookie.Value)
}

func sessionCookie(r *http.Request, value string, maxAge int) *http.Cookie {
	return &http.Cookie{
		Name:     database.SessionCookieName,
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   r.TLS != nil,
	}
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
		return database.DefaultDBPath
	}
	return path
}

func run() error {
	db, err := database.InitDB(configuredDBPath())
	if err != nil {
		return err
	}
	defer db.Close()
	app := NewApp(db)
	addr := configuredAddr()
	fmt.Printf("Listening on http://localhost%s\n", addr)
	return http.ListenAndServe(addr, app.Routes())
}
