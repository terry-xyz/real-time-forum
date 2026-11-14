package handlers

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"real-time-forum/models"

	"github.com/gorilla/websocket"
)

const (
	socketPingInterval = 25 * time.Second
	socketReadTimeout  = 60 * time.Second
	socketWriteTimeout = 5 * time.Second
	maxUserSockets     = 8
)

type chatConnection struct {
	conn     *websocket.Conn
	session  *authenticatedUser
	outgoing chan websocketEvent
	done     chan struct{}
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

var websocketUpgrader = websocket.Upgrader{
	HandshakeTimeout: socketWriteTimeout,
	CheckOrigin: func(r *http.Request) bool {
		scheme := "http://"
		if r.TLS != nil {
			scheme = "https://"
		}
		return r.Header.Get("Origin") == scheme+r.Host
	},
}

func WebSocketHandler(w http.ResponseWriter, r *http.Request) {
	user, err := getUserFromSession(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	conn, err := websocketUpgrader.Upgrade(w, r, w.Header())
	if err != nil {
		return
	}
	defer conn.Close()

	client := &chatConnection{
		conn: conn, session: user, outgoing: make(chan websocketEvent, 16), done: make(chan struct{}),
	}
	if !chatHub.register(client) {
		return
	}
	defer chatHub.unregister(client)

	// Close a socket upgraded while logout or another login was revoking its session.
	if _, err := getUserFromSession(r); err != nil {
		return
	}
	expiry := time.AfterFunc(time.Until(user.expiresAt), func() { chatHub.unregister(client) })
	defer expiry.Stop()

	conn.SetReadLimit(1024)
	conn.SetReadDeadline(time.Now().Add(socketReadTimeout))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(socketReadTimeout))
	})
	go client.writeEvents()

	var lastTyping time.Time
	for {
		messageType, payload, err := conn.ReadMessage()
		if err != nil {
			return
		}
		if messageType != websocket.TextMessage || time.Since(lastTyping) < time.Second {
			continue
		}
		var event struct {
			Type       string `json:"type"`
			ReceiverID string `json:"receiver_id"`
		}
		if json.Unmarshal(payload, &event) != nil || event.Type != "typing" ||
			event.ReceiverID == "" || event.ReceiverID == user.ID {
			continue
		}
		select {
		case <-client.done:
			return
		default:
			lastTyping = time.Now()
			chatHub.send(event.ReceiverID, websocketEvent{Type: "typing", SenderID: user.ID})
		}
	}
}

func (client *chatConnection) writeEvents() {
	defer chatHub.unregister(client)
	pings := time.NewTicker(socketPingInterval)
	defer pings.Stop()
	for {
		select {
		case <-client.done:
			return
		case event := <-client.outgoing:
			if !time.Now().Before(client.session.expiresAt) {
				return
			}
			client.conn.SetWriteDeadline(time.Now().Add(socketWriteTimeout))
			if err := client.conn.WriteJSON(event); err != nil {
				return
			}
		case <-pings.C:
			if err := client.conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(socketWriteTimeout)); err != nil {
				return
			}
		}
	}
}

func (hub *chatConnectionHub) register(client *chatConnection) bool {
	hub.mu.Lock()
	defer hub.mu.Unlock()
	userID := client.session.ID
	if len(hub.connections[userID]) >= maxUserSockets {
		return false
	}
	if hub.connections[userID] == nil {
		hub.connections[userID] = make(map[*chatConnection]struct{})
	}
	hub.connections[userID][client] = struct{}{}
	return true
}

func (hub *chatConnectionHub) unregister(client *chatConnection) {
	hub.mu.Lock()
	defer hub.mu.Unlock()
	hub.remove(client)
}

// remove requires the hub lock.
func (hub *chatConnectionHub) remove(client *chatConnection) {
	userID := client.session.ID
	clients := hub.connections[userID]
	if _, registered := clients[client]; !registered {
		return
	}
	delete(clients, client)
	if len(clients) == 0 {
		delete(hub.connections, userID)
	}
	close(client.done)
	client.conn.Close()
}

func (hub *chatConnectionHub) revoke(userID, sessionID string) {
	hub.mu.Lock()
	defer hub.mu.Unlock()
	for client := range hub.connections[userID] {
		if client.session.sessionID == sessionID {
			hub.remove(client)
		}
	}
}

func (hub *chatConnectionHub) isOnline(userID string) bool {
	hub.mu.RLock()
	defer hub.mu.RUnlock()
	return len(hub.connections[userID]) > 0
}

func (hub *chatConnectionHub) send(userID string, event websocketEvent) {
	hub.mu.Lock()
	defer hub.mu.Unlock()
	for client := range hub.connections[userID] {
		select {
		case client.outgoing <- event:
		default:
			hub.remove(client)
		}
	}
}
