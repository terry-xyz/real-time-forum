package handlers

import (
	"net/http"

	"github.com/gorilla/websocket"
)

var websocketUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		return origin == "" || origin == "http://"+r.Host || origin == "https://"+r.Host
	},
}

// WebSocketHandler opens an authenticated connection for future chat updates.
func WebSocketHandler(w http.ResponseWriter, r *http.Request) {
	if _, err := getUserFromSession(r); err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	conn, err := websocketUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
	}
}
