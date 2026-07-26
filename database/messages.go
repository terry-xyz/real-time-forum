package database

import (
	"database/sql"
	"errors"
	"strings"
	"time"

	"real-time-forum/models"
)

func CreateMessage(db *sql.DB, fromID int, toID int, content string, createdAt time.Time) (int, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return 0, errors.New("message cannot be empty")
	}
	result, err := db.Exec(
		`INSERT INTO messages (sender_id, receiver_id, content, created_at) VALUES (?, ?, ?, ?)`,
		fromID,
		toID,
		content,
		createdAt.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return 0, err
	}
	id, err := result.LastInsertId()
	return int(id), err
}

func CreateMessageModel(db *sql.DB, fromID int, toID int, content string, createdAt time.Time) (models.Message, error) {
	id, err := CreateMessage(db, fromID, toID, content, createdAt)
	if err != nil {
		return models.Message{}, err
	}
	messages, err := listMessagesByIDs(db, []int{id})
	if err != nil || len(messages) == 0 {
		return models.Message{}, err
	}
	return messages[0], nil
}

func ChatUsers(db *sql.DB, currentUserID int, online map[int]bool) ([]models.User, error) {
	rows, err := db.Query(`
		SELECT u.id, u.nickname, MAX(m.created_at) AS last_message_at
		FROM users u
		LEFT JOIN messages m ON (
			(m.sender_id = ? AND m.receiver_id = u.id) OR
			(m.receiver_id = ? AND m.sender_id = u.id)
		)
		WHERE u.id != ?
		GROUP BY u.id, u.nickname
		ORDER BY last_message_at IS NULL ASC, datetime(last_message_at) DESC, lower(u.nickname) ASC
	`, currentUserID, currentUserID, currentUserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	users := make([]models.User, 0)
	for rows.Next() {
		var user models.User
		var last sql.NullString
		if err := rows.Scan(&user.ID, &user.Nickname, &last); err != nil {
			return nil, err
		}
		user.Online = online[user.ID]
		if last.Valid {
			parsed, err := parseDBTime(last.String)
			if err != nil {
				return nil, err
			}
			user.LastChat = &parsed
		}
		users = append(users, user)
	}
	return users, rows.Err()
}

func ListMessages(db *sql.DB, currentUserID int, otherUserID int, beforeID int, limit int) ([]models.Message, error) {
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	var rows *sql.Rows
	var err error
	if beforeID > 0 {
		rows, err = db.Query(`
			SELECT id FROM messages
			WHERE id < ? AND ((sender_id = ? AND receiver_id = ?) OR (sender_id = ? AND receiver_id = ?))
			ORDER BY id DESC
			LIMIT ?
		`, beforeID, currentUserID, otherUserID, otherUserID, currentUserID, limit)
	} else {
		rows, err = db.Query(`
			SELECT id FROM messages
			WHERE (sender_id = ? AND receiver_id = ?) OR (sender_id = ? AND receiver_id = ?)
			ORDER BY id DESC
			LIMIT ?
		`, currentUserID, otherUserID, otherUserID, currentUserID, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	reverseInts(ids)
	return listMessagesByIDs(db, ids)
}

func listMessagesByIDs(db *sql.DB, ids []int) ([]models.Message, error) {
	messages := make([]models.Message, 0, len(ids))
	for _, id := range ids {
		var message models.Message
		var createdAt string
		err := db.QueryRow(`
			SELECT m.id, m.content, m.created_at,
				s.id, s.nickname,
				r.id, r.nickname
			FROM messages m
			JOIN users s ON s.id = m.sender_id
			JOIN users r ON r.id = m.receiver_id
			WHERE m.id = ?
		`, id).Scan(&message.ID, &message.Content, &createdAt, &message.From.ID, &message.From.Nickname, &message.To.ID, &message.To.Nickname)
		if err != nil {
			return nil, err
		}
		message.CreatedAt, err = parseDBTime(createdAt)
		if err != nil {
			return nil, err
		}
		messages = append(messages, message)
	}
	return messages, nil
}

func reverseInts(values []int) {
	for i, j := 0, len(values)-1; i < j; i, j = i+1, j-1 {
		values[i], values[j] = values[j], values[i]
	}
}
