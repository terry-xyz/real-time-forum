package database

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"time"

	"real-time-forum/models"
)

const SessionCookieName = "rtf_session"

var ErrUnauthorized = errors.New("unauthorized")

func CreateSession(db *sql.DB, userID int) (string, error) {
	token, err := randomToken()
	if err != nil {
		return "", err
	}
	_, err = db.Exec(`INSERT INTO sessions (id, user_id, expires_at) VALUES (?, ?, ?)`, token, userID, time.Now().Add(24*time.Hour).UTC().Format(time.RFC3339Nano))
	return token, err
}

func DeleteSession(db *sql.DB, sessionID string) error {
	_, err := db.Exec(`DELETE FROM sessions WHERE id = ?`, sessionID)
	return err
}

func CurrentUser(db *sql.DB, sessionID string) (models.User, error) {
	if sessionID == "" {
		return models.User{}, ErrUnauthorized
	}
	var userID int
	err := db.QueryRow(`SELECT user_id FROM sessions WHERE id = ? AND datetime(expires_at) > datetime('now')`, sessionID).Scan(&userID)
	if err != nil {
		return models.User{}, ErrUnauthorized
	}
	return UserByID(db, userID)
}

func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
