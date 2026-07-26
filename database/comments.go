package database

import (
	"database/sql"
	"errors"
	"strings"

	"real-time-forum/models"
)

func CreateComment(db *sql.DB, authorID int, postID int, content string) (models.Comment, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return models.Comment{}, errors.New("comment cannot be empty")
	}
	result, err := db.Exec(`INSERT INTO comments (post_id, author_id, content) VALUES (?, ?, ?)`, postID, authorID, content)
	if err != nil {
		return models.Comment{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return models.Comment{}, err
	}
	comments, err := ListComments(db, postID)
	if err != nil {
		return models.Comment{}, err
	}
	for _, comment := range comments {
		if comment.ID == int(id) {
			return comment, nil
		}
	}
	return models.Comment{}, sql.ErrNoRows
}

func ListComments(db *sql.DB, postID int) ([]models.Comment, error) {
	rows, err := db.Query(`
		SELECT c.id, c.post_id, c.content, c.created_at, u.id, u.nickname
		FROM comments c
		JOIN users u ON u.id = c.author_id
		WHERE c.post_id = ?
		ORDER BY datetime(c.created_at) ASC, c.id ASC
	`, postID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	comments := make([]models.Comment, 0)
	for rows.Next() {
		var comment models.Comment
		var createdAt string
		if err := rows.Scan(&comment.ID, &comment.PostID, &comment.Content, &createdAt, &comment.Author.ID, &comment.Author.Nickname); err != nil {
			return nil, err
		}
		comment.CreatedAt, err = parseDBTime(createdAt)
		if err != nil {
			return nil, err
		}
		comments = append(comments, comment)
	}
	return comments, rows.Err()
}
