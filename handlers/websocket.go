package handlers

import (
	"encoding/json"
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
	Type     string          `json:"type"`
	SenderID string          `json:"sender_id,omitempty"`
	Message  *models.Message `json:"message,omitempty"`
}

var chatHub = chatConnectionHub{
	connections: make(map[string]map[*chatConnection]struct{}),
}

type incomingEvent struct {
	Type       string `json:"type"`
	ReceiverID string `json:"receiver_id"`
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
		_, payload, err := conn.ReadMessage()
		if err != nil {
			return
		}

		var event incomingEvent
		if err := json.Unmarshal(payload, &event); err != nil {
			continue
		}

		if event.Type != "typing" || event.ReceiverID == "" {
			continue
		}

		chatHub.sendTyping(event.ReceiverID, user.ID)
	}
}

// register adds a chat connection to the set of live connections for its user.
func (hub *chatConnectionHub) register(userID string, client *chatConnection) {
	hub.mu.Lock()
	defer hub.mu.Unlock()
	if hub.connections[userID] == nil {
		hub.connections[userID] = make(map[*chatConnection]struct{})
	}
	hub.connections[userID][client] = struct{}{}
}

// unregister removes a chat connection and closes its underlying socket.
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

// isOnline reports whether the user currently has a registered chat connection.
func (hub *chatConnectionHub) isOnline(userID string) bool {
	hub.mu.RLock()
	defer hub.mu.RUnlock()
	return len(hub.connections[userID]) > 0
}

// sendMessage delivers a saved message to every live connection belonging to the recipient.
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
		err := client.conn.WriteJSON(websocketEvent{Type: "message", Message: &message})
		client.writeMu.Unlock()
		if err != nil {
			hub.unregister(userID, client)
		}
	}
}

// sendTyping relays a typing event to every live connection belonging to the recipient.
func (hub *chatConnectionHub) sendTyping(userID, senderID string) {
	hub.mu.RLock()
	clients := make([]*chatConnection, 0, len(hub.connections[userID]))
	for client := range hub.connections[userID] {
		clients = append(clients, client)
	}
	hub.mu.RUnlock()

	for _, client := range clients {
		client.writeMu.Lock()
		client.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		err := client.conn.WriteJSON(websocketEvent{Type: "typing", SenderID: senderID})
		client.writeMu.Unlock()
		if err != nil {
			hub.unregister(userID, client)
		}
	}
}
