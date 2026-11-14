package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"real-time-forum/database"
	"real-time-forum/models"

	"github.com/gofrs/uuid"
	"golang.org/x/crypto/bcrypt"
)

const sessionCookie = "session_token"

// A fixed bcrypt hash makes an unknown account take roughly as long as a wrong password.
const invalidCredentialsHash = "$2a$10$vyY86gdxLBnOEzYuQU93neRMU3t8Y7Jkhi5/ls54Ny4seGM3T49Lq"

func RegisterHandler(w http.ResponseWriter, r *http.Request) {
	var req models.RegisterRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	req.Nickname = strings.TrimSpace(req.Nickname)
	req.Gender = strings.TrimSpace(req.Gender)
	req.FirstName = strings.TrimSpace(req.FirstName)
	req.LastName = strings.TrimSpace(req.LastName)
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))

	if req.Age < 13 || req.Age > 120 {
		http.Error(w, "Age must be between 13 and 120", http.StatusBadRequest)
		return
	}
	if req.Gender != "Male" && req.Gender != "Female" {
		http.Error(w, "Select Male or Female", http.StatusBadRequest)
		return
	}
	if req.Nickname == "" || len(req.Nickname) > 64 || strings.Contains(req.Nickname, "@") ||
		req.FirstName == "" || len(req.FirstName) > 100 || req.LastName == "" || len(req.LastName) > 100 {
		http.Error(w, "Missing or invalid account details", http.StatusBadRequest)
		return
	}
	address, err := mail.ParseAddress(req.Email)
	if err != nil || address.Address != req.Email || len(req.Email) > 254 {
		http.Error(w, "Invalid email address", http.StatusBadRequest)
		return
	}
	if len(req.Password) < 12 || len(req.Password) > 72 {
		http.Error(w, "Password must be between 12 and 72 bytes", http.StatusBadRequest)
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}
	userID, err := uuid.NewV4()
	if err != nil {
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}
	var createdID string
	err = database.DB.QueryRowContext(r.Context(), `
		INSERT INTO users (id, nickname, age, gender, first_name, last_name, email, password_hash)
		SELECT ?, ?, ?, ?, ?, ?, ?, ?
		WHERE NOT EXISTS (
			SELECT 1 FROM users
			WHERE LOWER(nickname) IN (LOWER(?), LOWER(?)) OR LOWER(email) IN (LOWER(?), LOWER(?))
		)
		ON CONFLICT DO NOTHING
		RETURNING id
	`, userID.String(), req.Nickname, req.Age, req.Gender, req.FirstName, req.LastName, req.Email, string(hash),
		req.Nickname, req.Email, req.Nickname, req.Email).Scan(&createdID)
	if err == sql.ErrNoRows {
		http.Error(w, "Nickname or email already taken", http.StatusConflict)
		return
	}
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"message": "Registration successful"})
}

func LoginHandler(w http.ResponseWriter, r *http.Request) {
	var req models.LoginRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	identifier := strings.TrimSpace(req.Login)
	if identifier == "" || len(req.Password) > 72 {
		http.Error(w, "Invalid credentials", http.StatusUnauthorized)
		return
	}
	var user models.User
	var hash string
	err := database.DB.QueryRowContext(r.Context(), `
		SELECT id, nickname, password_hash FROM users
		WHERE LOWER(nickname) = LOWER(?) OR LOWER(email) = LOWER(?)
		ORDER BY CASE WHEN nickname = ? THEN 0 WHEN email = ? THEN 1 ELSE 2 END, id LIMIT 1
	`, identifier, identifier, identifier, identifier).Scan(&user.ID, &user.Nickname, &hash)
	if err == sql.ErrNoRows {
		hash = invalidCredentialsHash
	} else if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.Password)) != nil || err == sql.ErrNoRows {
		http.Error(w, "Invalid credentials", http.StatusUnauthorized)
		return
	}

	sessionID, err := uuid.NewV4()
	if err != nil {
		http.Error(w, "Session creation failed", http.StatusInternalServerError)
		return
	}
	expiresAt := time.Now().UTC().Add(24 * time.Hour).Truncate(time.Second)
	if err := replaceSession(r, user.ID, sessionID.String(), expiresAt); err != nil {
		http.Error(w, "Session creation failed", http.StatusInternalServerError)
		return
	}
	setSessionCookie(w, r, sessionID.String(), expiresAt, 0)
	json.NewEncoder(w).Encode(map[string]any{
		"message": "Login successful",
		"user":    map[string]string{"id": user.ID, "nickname": user.Nickname},
	})
}

func replaceSession(r *http.Request, userID, sessionID string, expiresAt time.Time) error {
	ctx := r.Context()
	tx, err := database.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, "DELETE FROM sessions WHERE user_id = ? RETURNING id", userID)
	if err != nil {
		return err
	}
	var revoked []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		revoked = append(revoked, id)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM sessions WHERE expires_at <= CURRENT_TIMESTAMP"); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO sessions (id, user_id, expires_at) VALUES (?, ?, ?)",
		sessionID, userID, expiresAt.Format(time.DateTime)); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	for _, id := range revoked {
		chatHub.revoke(userID, id)
	}
	return nil
}

func LogoutHandler(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(sessionCookie); err == nil {
		var userID string
		err := database.DB.QueryRowContext(r.Context(), "DELETE FROM sessions WHERE id = ? RETURNING user_id", cookie.Value).Scan(&userID)
		if err != nil && err != sql.ErrNoRows {
			http.Error(w, "Logout failed", http.StatusInternalServerError)
			return
		}
		if err == nil {
			chatHub.revoke(userID, cookie.Value)
		}
	}
	setSessionCookie(w, r, "", time.Unix(1, 0), -1)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "Logged out successfully"})
}

func setSessionCookie(w http.ResponseWriter, r *http.Request, value string, expiresAt time.Time, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: value, Expires: expiresAt, MaxAge: maxAge, Path: "/",
		HttpOnly: true, Secure: r.TLS != nil, SameSite: http.SameSiteStrictMode,
	})
}

func CheckAuthHandler(w http.ResponseWriter, r *http.Request) {
	user, err := getUserFromSession(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"id": user.ID, "nickname": user.Nickname})
}

type authenticatedUser struct {
	ID, Nickname string
	sessionID    string
	expiresAt    time.Time
}

func getUserFromSession(r *http.Request) (*authenticatedUser, error) {
	cookie, err := r.Cookie(sessionCookie)
	if err != nil {
		return nil, err
	}
	user := authenticatedUser{sessionID: cookie.Value}
	err = database.DB.QueryRowContext(r.Context(), `
		SELECT u.id, u.nickname, s.expires_at
		FROM sessions s JOIN users u ON u.id = s.user_id
		WHERE s.id = ? AND s.expires_at > CURRENT_TIMESTAMP
	`, cookie.Value).Scan(&user.ID, &user.Nickname, &user.expiresAt)
	if err != nil {
		return nil, err
	}
	return &user, nil
}
