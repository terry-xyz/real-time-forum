package handlers

import (
	"net/http"
	"sync"
	"time"

	"real-time-forum/models"

	"github.com/gorilla/websocket"
)

type chatConnection struct {
	conn    *websocket.Conn
	writeMu sync.Mutex
}

type chatConnectionHub struct {
	mu          sync.RWMutex
	connections map[string]map[*chatConnection]struct{}
}

type websocketEvent struct {
	Type    string         `json:"type"`
	Message models.Message `json:"message"`
}

var chatHub = chatConnectionHub{
	connections: make(map[string]map[*chatConnection]struct{}),
}

var websocketUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		return origin == "" || origin == "http://"+r.Host || origin == "https://"+r.Host
	},
}

// WebSocketHandler opens an authenticated connection for future chat updates.
func WebSocketHandler(w http.ResponseWriter, r *http.Request) {
	user, err := getUserFromSession(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	conn, err := websocketUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	client := &chatConnection{conn: conn}
	chatHub.register(user.ID, client)
	defer chatHub.unregister(user.ID, client)

	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
	}
}

func (hub *chatConnectionHub) register(userID string, client *chatConnection) {
	hub.mu.Lock()
	defer hub.mu.Unlock()
	if hub.connections[userID] == nil {
		hub.connections[userID] = make(map[*chatConnection]struct{})
	}
	hub.connections[userID][client] = struct{}{}
}

func (hub *chatConnectionHub) unregister(userID string, client *chatConnection) {
	hub.mu.Lock()
	if clients := hub.connections[userID]; clients != nil {
		delete(clients, client)
		if len(clients) == 0 {
			delete(hub.connections, userID)
		}
	}
	hub.mu.Unlock()

	client.writeMu.Lock()
	client.conn.Close()
	client.writeMu.Unlock()
}

func (hub *chatConnectionHub) sendMessage(userID string, message models.Message) {
	hub.mu.RLock()
	clients := make([]*chatConnection, 0, len(hub.connections[userID]))
	for client := range hub.connections[userID] {
		clients = append(clients, client)
	}
	hub.mu.RUnlock()

	for _, client := range clients {
		client.writeMu.Lock()
		client.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		err := client.conn.WriteJSON(websocketEvent{Type: "message", Message: message})
		client.writeMu.Unlock()
		if err != nil {
			hub.unregister(userID, client)
		}
	}
}
