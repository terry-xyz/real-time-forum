package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"real-time-forum/database"
	"real-time-forum/models"
)

// ChatUsersHandler lists the other users, with recent conversations first.
func ChatUsersHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user, err := getUserFromSession(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	rows, err := database.DB.Query(`
		SELECT u.id, u.nickname, MAX(m.created_at) AS last_message_at
		FROM users u
		LEFT JOIN messages m ON
			(m.sender_id = ? AND m.receiver_id = u.id) OR
			(m.sender_id = u.id AND m.receiver_id = ?)
		WHERE u.id != ?
		GROUP BY u.id, u.nickname
		ORDER BY
			CASE WHEN last_message_at IS NULL THEN 1 ELSE 0 END,
			last_message_at DESC,
			LOWER(u.nickname) ASC,
			u.id ASC
	`, user.ID, user.ID, user.ID)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	users := make([]models.ChatUser, 0)
	for rows.Next() {
		var chatUser models.ChatUser
		var lastMessageAt sql.NullString
		if err := rows.Scan(&chatUser.ID, &chatUser.Nickname, &lastMessageAt); err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
		if lastMessageAt.Valid {
			chatUser.LastMessageAt = &lastMessageAt.String
		}
		users = append(users, chatUser)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(users)
}
