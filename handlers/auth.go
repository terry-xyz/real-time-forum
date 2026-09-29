package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"real-time-forum/database"
	"real-time-forum/models"

	"github.com/gofrs/uuid"
	"golang.org/x/crypto/bcrypt"
)

// RegisterHandler processes new user registration
func RegisterHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req models.RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}

	// Basic Validation
	req.Nickname = strings.TrimSpace(req.Nickname)
	req.Gender = strings.TrimSpace(req.Gender)
	req.FirstName = strings.TrimSpace(req.FirstName)
	req.LastName = strings.TrimSpace(req.LastName)
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))

	if req.Nickname == "" || req.Email == "" || req.Password == "" || req.Age <= 0 ||
		req.Gender == "" || req.FirstName == "" || req.LastName == "" {
		http.Error(w, "Missing required fields or invalid age", http.StatusBadRequest)
		return
	}

	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}

	// Generate UUID
	userID, err := uuid.NewV4()
	if err != nil {
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}

	// Insert into SQLite
	query := `INSERT INTO users (id, nickname, age, gender, first_name, last_name, email, password_hash)
			  VALUES (?, ?, ?, ?, ?, ?, ?, ?)`

	_, err = database.DB.Exec(query, userID.String(), req.Nickname, req.Age, req.Gender, req.FirstName, req.LastName, req.Email, string(hashedPassword))
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			http.Error(w, "Nickname or Email already taken", http.StatusConflict)
			return
		}
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"message": "Registration successful"})
}

// LoginHandler supports logging in via Nickname Or Email
func LoginHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req models.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	loginIdentifier := strings.TrimSpace(strings.ToLower(req.Login))

	// Fetch user by Nickname OR Email
	query := `SELECT id, nickname, password_hash FROM users WHERE LOWER(nickname) = ? OR LOWER(email) = ?`
	row := database.DB.QueryRow(query, loginIdentifier, loginIdentifier)

	var user models.User
	var passwordHash string
	err := row.Scan(&user.ID, &user.Nickname, &passwordHash)
	if err == sql.ErrNoRows {
		http.Error(w, "Invalid credentials", http.StatusUnauthorized)
		return
	} else if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	// Compare passwords
	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(req.Password)); err != nil {
		http.Error(w, "Invalid credentials", http.StatusUnauthorized)
		return
	}

	// Create session
	sessionID, _ := uuid.NewV4()
	expiresAt := time.Now().Add(24 * time.Hour)

	// Clean up old sessions for this user
	database.DB.Exec(`DELETE FROM sessions WHERE user_id = ?`, user.ID)

	// Save new session
	_, err = database.DB.Exec(`INSERT INTO sessions (id, user_id, expires_at) VALUES (?, ?, ?)`,
		sessionID.String(), user.ID, expiresAt)
	if err != nil {
		http.Error(w, "Session creation failed", http.StatusInternalServerError)
		return
	}

	// Set cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "session_token",
		Value:    sessionID.String(),
		Expires:  expiresAt,
		HttpOnly: true,
		Path:     "/",
		SameSite: http.SameSiteLaxMode,
	})

	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "Login successful",
		"user": map[string]string{
			"id":       user.ID,
			"nickname": user.Nickname,
		},
	})
}

// LogoutHandler removes session from database and clears cookie
func LogoutHandler(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("session_token")
	if err == nil {
		database.DB.Exec(`DELETE FROM sessions WHERE id = ?`, cookie.Value)
	}

	// Clear Cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "session_token",
		Value:    "",
		Expires:  time.Now().Add(-1 * time.Hour),
		HttpOnly: true,
		Path:     "/",
	})

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"message": "Logged out successfully"})
}

// CheckAuthHandler verifies session status on page reloads
func CheckAuthHandler(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("session_token")
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var user models.User
	query := `SELECT u.id, u.nickname FROM users u
			  JOIN sessions s ON u.id = s.user_id
			  WHERE s.id = ? AND s.expires_at > ?`

	err = database.DB.QueryRow(query, cookie.Value, time.Now()).Scan(&user.ID, &user.Nickname)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	json.NewEncoder(w).Encode(user)
}
