package realtime

import (
	"sync"

	"github.com/gorilla/websocket"
)

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
