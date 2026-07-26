package handlers

import (
	"net/http"
	"strings"
	"time"

	"real-time-forum/database"
	"real-time-forum/models"
)

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
