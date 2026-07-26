package handlers

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"real-time-forum/database"

	"github.com/gorilla/websocket"
)

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
