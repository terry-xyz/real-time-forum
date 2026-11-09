package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"real-time-forum/database"
	"real-time-forum/models"

	"github.com/gofrs/uuid"
)

const maxMessagesPerPage = 10

// MessagesHandler loads conversation history or stores a new message.
func MessagesHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		getMessages(w, r)
	case http.MethodPost:
		createMessage(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// getMessages requests a conversation page, optionally before a message ID.
func getMessages(w http.ResponseWriter, r *http.Request) {

	user, err := getUserFromSession(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	peerID := r.URL.Query().Get("with")
	if peerID == "" || peerID == user.ID {
		http.Error(w, "A different user ID is required in the with parameter", http.StatusBadRequest)
		return
	}

	var existingPeerID string
	err = database.DB.QueryRow(`SELECT id FROM users WHERE id = ?`, peerID).Scan(&existingPeerID)
	if err == sql.ErrNoRows {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	limit := maxMessagesPerPage
	if rawLimit := r.URL.Query().Get("limit"); rawLimit != "" {
		limit, err = strconv.Atoi(rawLimit)
		if err != nil || limit < 1 {
			http.Error(w, "limit must be a positive integer", http.StatusBadRequest)
			return
		}
		if limit > maxMessagesPerPage {
			limit = maxMessagesPerPage
		}
	}

	beforeID := r.URL.Query().Get("before_id")
	beforeTime := ""
	if beforeID != "" {
		err = database.DB.QueryRow(`
			SELECT CAST(created_at AS TEXT) FROM messages
			WHERE id = ?
			  AND ((sender_id = ? AND receiver_id = ?) OR (sender_id = ? AND receiver_id = ?))
		`, beforeID, user.ID, peerID, peerID, user.ID).Scan(&beforeTime)
		if err == sql.ErrNoRows {
			http.Error(w, "before_id must identify a message in this conversation", http.StatusBadRequest)
			return
		}
		if err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
	}

	rows, err := database.DB.Query(`
		SELECT m.id, m.sender_id, sender.nickname, m.receiver_id, m.content, m.created_at
		FROM messages m
		JOIN users sender ON sender.id = m.sender_id
		WHERE ((m.sender_id = ? AND m.receiver_id = ?) OR (m.sender_id = ? AND m.receiver_id = ?))
		  AND (? = '' OR m.created_at < ? OR (m.created_at = ? AND m.id < ?))
		ORDER BY m.created_at DESC, m.id DESC
		LIMIT ?
	`, user.ID, peerID, peerID, user.ID, beforeID, beforeTime, beforeTime, beforeID, limit)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	messages := make([]models.Message, 0, limit)
	for rows.Next() {
		var message models.Message
		if err := rows.Scan(&message.ID, &message.SenderID, &message.SenderName, &message.ReceiverID, &message.Content, &message.CreatedAt); err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
		messages = append(messages, message)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	// SQLite returns the newest page first; reverse it for chronological display.
	for left, right := 0, len(messages)-1; left < right; left, right = left+1, right-1 {
		messages[left], messages[right] = messages[right], messages[left]
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(messages)
}

// createMessage saves an authenticated user’s message and delivers it to connected recipients.
func createMessage(w http.ResponseWriter, r *http.Request) {
	user, err := getUserFromSession(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req models.CreateMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}
	req.ReceiverID = strings.TrimSpace(req.ReceiverID)
	req.Content = strings.TrimSpace(req.Content)
	if req.ReceiverID == "" || req.Content == "" {
		http.Error(w, "Receiver ID and message content are required", http.StatusBadRequest)
		return
	}
	if req.ReceiverID == user.ID {
		http.Error(w, "Cannot send a message to yourself", http.StatusBadRequest)
		return
	}

	var existingPeerID string
	err = database.DB.QueryRow(`SELECT id FROM users WHERE id = ?`, req.ReceiverID).Scan(&existingPeerID)
	if err == sql.ErrNoRows {
		http.Error(w, "Recipient not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	messageID, err := uuid.NewV4()
	if err != nil {
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}
	_, err = database.DB.Exec(`
		INSERT INTO messages (id, sender_id, receiver_id, content)
		VALUES (?, ?, ?, ?)
	`, messageID.String(), user.ID, req.ReceiverID, req.Content)
	if err != nil {
		http.Error(w, "Failed to save message", http.StatusInternalServerError)
		return
	}

	var message models.Message
	err = database.DB.QueryRow(`
		SELECT m.id, m.sender_id, sender.nickname, m.receiver_id, m.content, m.created_at
		FROM messages m
		JOIN users sender ON sender.id = m.sender_id
		WHERE m.id = ?
	`, messageID.String()).Scan(
		&message.ID, &message.SenderID, &message.SenderName,
		&message.ReceiverID, &message.Content, &message.CreatedAt,
	)
	if err != nil {
		http.Error(w, "Failed to load saved message", http.StatusInternalServerError)
		return
	}
	chatHub.sendMessage(message.ReceiverID, message)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(message)
}
