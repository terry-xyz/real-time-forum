package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"real-time-forum/models"

	"github.com/gorilla/websocket"
	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/crypto/bcrypt"
)

const (
	sessionCookieName = "rtf_session"
	defaultPort       = "8080"
	defaultDBPath     = "real-time-forum.db"
)

var errUnauthorized = errors.New("unauthorized")

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
	if _, err := db.Exec(schemaSQL); err != nil {
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

const schemaSQL = `
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

CREATE TABLE IF NOT EXISTS sessions (
	id TEXT PRIMARY KEY,
	user_id INTEGER NOT NULL,
	expires_at TIMESTAMP NOT NULL,
	created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS categories (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL UNIQUE
);

INSERT OR IGNORE INTO categories (id, name) VALUES
	(1, 'General'),
	(2, 'Programming'),
	(3, 'Gaming'),
	(4, 'Music');

CREATE TABLE IF NOT EXISTS posts (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	author_id INTEGER NOT NULL,
	title TEXT NOT NULL,
	content TEXT NOT NULL,
	created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (author_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS post_categories (
	post_id INTEGER NOT NULL,
	category_id INTEGER NOT NULL,
	PRIMARY KEY (post_id, category_id),
	FOREIGN KEY (post_id) REFERENCES posts(id) ON DELETE CASCADE,
	FOREIGN KEY (category_id) REFERENCES categories(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS comments (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	post_id INTEGER NOT NULL,
	author_id INTEGER NOT NULL,
	content TEXT NOT NULL,
	created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (post_id) REFERENCES posts(id) ON DELETE CASCADE,
	FOREIGN KEY (author_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS messages (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	sender_id INTEGER NOT NULL,
	receiver_id INTEGER NOT NULL,
	content TEXT NOT NULL,
	created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (sender_id) REFERENCES users(id) ON DELETE CASCADE,
	FOREIGN KEY (receiver_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_posts_created ON posts(created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_comments_post ON comments(post_id, created_at ASC, id ASC);
CREATE INDEX IF NOT EXISTS idx_messages_pair ON messages(sender_id, receiver_id, id DESC);
`

func CreateUser(db *sql.DB, input models.RegistrationInput) (int, error) {
	normalized, err := validateRegistration(input)
	if err != nil {
		return 0, err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(normalized.Password), bcrypt.DefaultCost)
	if err != nil {
		return 0, err
	}
	result, err := db.Exec(
		`INSERT INTO users (nickname, age, gender, first_name, last_name, email, password_hash) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		normalized.Nickname,
		normalized.Age,
		normalized.Gender,
		normalized.FirstName,
		normalized.LastName,
		strings.ToLower(normalized.Email),
		string(hash),
	)
	if err != nil {
		return 0, err
	}
	id, err := result.LastInsertId()
	return int(id), err
}

func validateRegistration(input models.RegistrationInput) (models.RegistrationInput, error) {
	input.Nickname = strings.TrimSpace(input.Nickname)
	input.Gender = strings.TrimSpace(input.Gender)
	input.FirstName = strings.TrimSpace(input.FirstName)
	input.LastName = strings.TrimSpace(input.LastName)
	input.Email = strings.TrimSpace(input.Email)
	if input.Nickname == "" || input.Age <= 0 || input.Gender == "" || input.FirstName == "" || input.LastName == "" || input.Email == "" || len(input.Password) < 8 {
		return input, errors.New("all registration fields are required")
	}
	switch strings.ToLower(input.Gender) {
	case "male":
		input.Gender = "Male"
	case "female":
		input.Gender = "Female"
	default:
		return input, errors.New("gender must be Male or Female")
	}
	if !validEmail(input.Email) {
		return input, errors.New("invalid email")
	}
	return input, nil
}

func validEmail(value string) bool {
	if strings.ContainsAny(value, " \t\r\n") || strings.Count(value, "@") != 1 {
		return false
	}
	parts := strings.Split(value, "@")
	local, domain := parts[0], parts[1]
	if local == "" || domain == "" || !strings.Contains(domain, ".") {
		return false
	}
	for _, label := range strings.Split(domain, ".") {
		if label == "" {
			return false
		}
	}
	return true
}

func CreateMessage(db *sql.DB, fromID int, toID int, content string, createdAt time.Time) (int, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return 0, errors.New("message cannot be empty")
	}
	result, err := db.Exec(
		`INSERT INTO messages (sender_id, receiver_id, content, created_at) VALUES (?, ?, ?, ?)`,
		fromID,
		toID,
		content,
		createdAt.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return 0, err
	}
	id, err := result.LastInsertId()
	return int(id), err
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
	id, err := CreateUser(a.db, input)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	user, err := userByID(a.db, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "unable to load user")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"user": publicUser(user)})
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
	user, passwordHash, err := userByIdentifier(a.db, strings.TrimSpace(input.Identifier))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "unable to log in")
		return
	}
	if user.ID == 0 || bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(input.Password)) != nil {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	sessionID, err := createSession(a.db, user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "unable to create session")
		return
	}
	http.SetCookie(w, sessionCookie(r, sessionID, int((24*time.Hour).Seconds())))
	writeJSON(w, http.StatusOK, map[string]any{"user": publicUser(user)})
}

func (a *App) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	cookie, err := r.Cookie(sessionCookieName)
	if err == nil {
		_, _ = a.db.Exec(`DELETE FROM sessions WHERE id = ?`, cookie.Value)
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
	writeJSON(w, http.StatusOK, map[string]any{"user": publicUser(user)})
}

func (a *App) handleCategories(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.Query(`SELECT id, name FROM categories ORDER BY id ASC`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "unable to load categories")
		return
	}
	defer rows.Close()
	categories := make([]map[string]any, 0)
	for rows.Next() {
		var id int
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			writeError(w, http.StatusInternalServerError, "unable to load categories")
			return
		}
		categories = append(categories, map[string]any{"id": id, "name": name})
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
		posts, err := listPosts(a.db)
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
		post, err := createPost(a.db, user.ID, input.Title, input.Content, input.CategoryIDs)
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
		comments, err := listComments(a.db, postID)
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
		comment, err := createComment(a.db, user.ID, postID, input.Content)
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
	users, err := chatUsers(a.db, user.ID, a.hub.OnlineIDs())
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
	messages, err := listMessages(a.db, user.ID, otherID, beforeID, 10)
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
		message, err := createMessageModel(a.db, user.ID, incoming.ToID, incoming.Content, time.Now())
		if err != nil {
			continue
		}
		a.hub.Send(user.ID, map[string]any{"type": "private_message", "message": message, "content": message.Content, "from": publicUser(message.From), "to": publicUser(message.To), "created_at": message.CreatedAt})
		a.hub.Send(incoming.ToID, map[string]any{"type": "private_message", "message": message, "content": message.Content, "from": publicUser(message.From), "to": publicUser(message.To), "created_at": message.CreatedAt})
	}
}

func (a *App) currentUser(r *http.Request) (models.User, error) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || cookie.Value == "" {
		return models.User{}, errUnauthorized
	}
	var userID int
	err = a.db.QueryRow(`SELECT user_id FROM sessions WHERE id = ? AND datetime(expires_at) > datetime('now')`, cookie.Value).Scan(&userID)
	if err != nil {
		return models.User{}, errUnauthorized
	}
	return userByID(a.db, userID)
}

func createSession(db *sql.DB, userID int) (string, error) {
	token, err := randomToken()
	if err != nil {
		return "", err
	}
	_, err = db.Exec(`INSERT INTO sessions (id, user_id, expires_at) VALUES (?, ?, ?)`, token, userID, time.Now().Add(24*time.Hour).UTC().Format(time.RFC3339Nano))
	return token, err
}

func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func sessionCookie(r *http.Request, value string, maxAge int) *http.Cookie {
	return &http.Cookie{
		Name:     sessionCookieName,
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   r.TLS != nil,
	}
}

func userByIdentifier(db *sql.DB, identifier string) (models.User, string, error) {
	var user models.User
	var passwordHash string
	err := db.QueryRow(
		`SELECT id, nickname, age, gender, first_name, last_name, email, password_hash FROM users WHERE lower(nickname) = lower(?) OR lower(email) = lower(?)`,
		identifier,
		identifier,
	).Scan(&user.ID, &user.Nickname, &user.Age, &user.Gender, &user.FirstName, &user.LastName, &user.Email, &passwordHash)
	if errors.Is(err, sql.ErrNoRows) {
		return models.User{}, "", nil
	}
	return user, passwordHash, err
}

func userByID(db *sql.DB, id int) (models.User, error) {
	var user models.User
	err := db.QueryRow(`SELECT id, nickname, age, gender, first_name, last_name, email FROM users WHERE id = ?`, id).
		Scan(&user.ID, &user.Nickname, &user.Age, &user.Gender, &user.FirstName, &user.LastName, &user.Email)
	return user, err
}

func publicUser(user models.User) models.User {
	user.Email = ""
	user.Age = 0
	user.Gender = ""
	user.FirstName = ""
	user.LastName = ""
	return user
}

func createPost(db *sql.DB, authorID int, title string, content string, categoryIDs []int) (models.Post, error) {
	title = strings.TrimSpace(title)
	content = strings.TrimSpace(content)
	if title == "" || content == "" || len(categoryIDs) == 0 {
		return models.Post{}, errors.New("title, content, and at least one category are required")
	}
	tx, err := db.Begin()
	if err != nil {
		return models.Post{}, err
	}
	defer tx.Rollback()
	result, err := tx.Exec(`INSERT INTO posts (author_id, title, content) VALUES (?, ?, ?)`, authorID, title, content)
	if err != nil {
		return models.Post{}, err
	}
	postID64, err := result.LastInsertId()
	if err != nil {
		return models.Post{}, err
	}
	postID := int(postID64)
	for _, categoryID := range uniquePositive(categoryIDs) {
		if _, err := tx.Exec(`INSERT INTO post_categories (post_id, category_id) VALUES (?, ?)`, postID, categoryID); err != nil {
			return models.Post{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return models.Post{}, err
	}
	posts, err := listPostByID(db, postID)
	if err != nil {
		return models.Post{}, err
	}
	return posts, nil
}

func listPosts(db *sql.DB) ([]models.Post, error) {
	rows, err := db.Query(`
		SELECT p.id, p.title, p.content, p.created_at, u.id, u.nickname, COALESCE(group_concat(c.name, '|'), '')
		FROM posts p
		JOIN users u ON u.id = p.author_id
		LEFT JOIN post_categories pc ON pc.post_id = p.id
		LEFT JOIN categories c ON c.id = pc.category_id
		GROUP BY p.id, p.title, p.content, p.created_at, u.id, u.nickname
		ORDER BY datetime(p.created_at) DESC, p.id DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	posts := make([]models.Post, 0)
	for rows.Next() {
		var post models.Post
		var createdAt string
		var categories string
		if err := rows.Scan(&post.ID, &post.Title, &post.Content, &createdAt, &post.Author.ID, &post.Author.Nickname, &categories); err != nil {
			return nil, err
		}
		post.CreatedAt, err = parseDBTime(createdAt)
		if err != nil {
			return nil, err
		}
		post.Categories = splitCategories(categories)
		posts = append(posts, post)
	}
	return posts, rows.Err()
}

func listPostByID(db *sql.DB, postID int) (models.Post, error) {
	var post models.Post
	var createdAt string
	var categories string
	err := db.QueryRow(`
		SELECT p.id, p.title, p.content, p.created_at, u.id, u.nickname, COALESCE(group_concat(c.name, '|'), '')
		FROM posts p
		JOIN users u ON u.id = p.author_id
		LEFT JOIN post_categories pc ON pc.post_id = p.id
		LEFT JOIN categories c ON c.id = pc.category_id
		WHERE p.id = ?
		GROUP BY p.id, p.title, p.content, p.created_at, u.id, u.nickname
	`, postID).Scan(&post.ID, &post.Title, &post.Content, &createdAt, &post.Author.ID, &post.Author.Nickname, &categories)
	if err != nil {
		return models.Post{}, err
	}
	post.CreatedAt, err = parseDBTime(createdAt)
	if err != nil {
		return models.Post{}, err
	}
	post.Categories = splitCategories(categories)
	return post, nil
}

func uniquePositive(ids []int) []int {
	seen := make(map[int]bool, len(ids))
	var out []int
	for _, id := range ids {
		if id <= 0 || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

func createComment(db *sql.DB, authorID int, postID int, content string) (models.Comment, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return models.Comment{}, errors.New("comment cannot be empty")
	}
	result, err := db.Exec(`INSERT INTO comments (post_id, author_id, content) VALUES (?, ?, ?)`, postID, authorID, content)
	if err != nil {
		return models.Comment{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return models.Comment{}, err
	}
	comments, err := listComments(db, postID)
	if err != nil {
		return models.Comment{}, err
	}
	for _, comment := range comments {
		if comment.ID == int(id) {
			return comment, nil
		}
	}
	return models.Comment{}, sql.ErrNoRows
}

func listComments(db *sql.DB, postID int) ([]models.Comment, error) {
	rows, err := db.Query(`
		SELECT c.id, c.post_id, c.content, c.created_at, u.id, u.nickname
		FROM comments c
		JOIN users u ON u.id = c.author_id
		WHERE c.post_id = ?
		ORDER BY datetime(c.created_at) ASC, c.id ASC
	`, postID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	comments := make([]models.Comment, 0)
	for rows.Next() {
		var comment models.Comment
		var createdAt string
		if err := rows.Scan(&comment.ID, &comment.PostID, &comment.Content, &createdAt, &comment.Author.ID, &comment.Author.Nickname); err != nil {
			return nil, err
		}
		comment.CreatedAt, err = parseDBTime(createdAt)
		if err != nil {
			return nil, err
		}
		comments = append(comments, comment)
	}
	return comments, rows.Err()
}

func chatUsers(db *sql.DB, currentUserID int, online map[int]bool) ([]models.User, error) {
	rows, err := db.Query(`
		SELECT u.id, u.nickname, MAX(m.created_at) AS last_message_at
		FROM users u
		LEFT JOIN messages m ON (
			(m.sender_id = ? AND m.receiver_id = u.id) OR
			(m.receiver_id = ? AND m.sender_id = u.id)
		)
		WHERE u.id != ?
		GROUP BY u.id, u.nickname
		ORDER BY last_message_at IS NULL ASC, datetime(last_message_at) DESC, lower(u.nickname) ASC
	`, currentUserID, currentUserID, currentUserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	users := make([]models.User, 0)
	for rows.Next() {
		var user models.User
		var last sql.NullString
		if err := rows.Scan(&user.ID, &user.Nickname, &last); err != nil {
			return nil, err
		}
		user.Online = online[user.ID]
		if last.Valid {
			parsed, err := parseDBTime(last.String)
			if err != nil {
				return nil, err
			}
			user.LastChat = &parsed
		}
		users = append(users, user)
	}
	return users, rows.Err()
}

func createMessageModel(db *sql.DB, fromID int, toID int, content string, createdAt time.Time) (models.Message, error) {
	id, err := CreateMessage(db, fromID, toID, content, createdAt)
	if err != nil {
		return models.Message{}, err
	}
	messages, err := listMessagesByIDs(db, []int{id})
	if err != nil || len(messages) == 0 {
		return models.Message{}, err
	}
	return messages[0], nil
}

func listMessages(db *sql.DB, currentUserID int, otherUserID int, beforeID int, limit int) ([]models.Message, error) {
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	var rows *sql.Rows
	var err error
	if beforeID > 0 {
		rows, err = db.Query(`
			SELECT id FROM messages
			WHERE id < ? AND ((sender_id = ? AND receiver_id = ?) OR (sender_id = ? AND receiver_id = ?))
			ORDER BY id DESC
			LIMIT ?
		`, beforeID, currentUserID, otherUserID, otherUserID, currentUserID, limit)
	} else {
		rows, err = db.Query(`
			SELECT id FROM messages
			WHERE (sender_id = ? AND receiver_id = ?) OR (sender_id = ? AND receiver_id = ?)
			ORDER BY id DESC
			LIMIT ?
		`, currentUserID, otherUserID, otherUserID, currentUserID, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	reverseInts(ids)
	return listMessagesByIDs(db, ids)
}

func listMessagesByIDs(db *sql.DB, ids []int) ([]models.Message, error) {
	messages := make([]models.Message, 0, len(ids))
	for _, id := range ids {
		var message models.Message
		var createdAt string
		err := db.QueryRow(`
			SELECT m.id, m.content, m.created_at,
				s.id, s.nickname,
				r.id, r.nickname
			FROM messages m
			JOIN users s ON s.id = m.sender_id
			JOIN users r ON r.id = m.receiver_id
			WHERE m.id = ?
		`, id).Scan(&message.ID, &message.Content, &createdAt, &message.From.ID, &message.From.Nickname, &message.To.ID, &message.To.Nickname)
		if err != nil {
			return nil, err
		}
		message.CreatedAt, err = parseDBTime(createdAt)
		if err != nil {
			return nil, err
		}
		messages = append(messages, message)
	}
	return messages, nil
}

func splitCategories(value string) []string {
	if value == "" {
		return nil
	}
	return strings.Split(value, "|")
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

func reverseInts(values []int) {
	for i, j := 0, len(values)-1; i < j; i, j = i+1, j-1 {
		values[i], values[j] = values[j], values[i]
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
		return defaultDBPath
	}
	return path
}

func run() error {
	db, err := InitDB(configuredDBPath())
	if err != nil {
		return err
	}
	defer db.Close()
	app := NewApp(db)
	addr := configuredAddr()
	fmt.Printf("Listening on http://localhost%s\n", addr)
	return http.ListenAndServe(addr, app.Routes())
}
