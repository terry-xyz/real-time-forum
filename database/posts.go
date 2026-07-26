package database

import (
	"database/sql"
	"errors"
	"strings"

	"real-time-forum/models"
)

func CreatePost(db *sql.DB, authorID int, title string, content string, categoryIDs []int) (models.Post, error) {
	title = strings.TrimSpace(title)
	content = strings.TrimSpace(content)
	categoryIDs = uniquePositive(categoryIDs)
	if title == "" || content == "" || len(categoryIDs) == 0 {
		return models.Post{}, errors.New("title, content, and at least one category are required")
	}
	tx, err := db.Begin()
	if err != nil {
		return models.Post{}, err
	}
	defer tx.Rollback()
	result, err := tx.Exec(`INSERT INTO posts (author_id, title, content) VALUES (?, ?, ?)`, authorID, title, content)
	if err != nil {
		return models.Post{}, err
	}
	postID64, err := result.LastInsertId()
	if err != nil {
		return models.Post{}, err
	}
	postID := int(postID64)
	for _, categoryID := range categoryIDs {
		if _, err := tx.Exec(`INSERT INTO post_categories (post_id, category_id) VALUES (?, ?)`, postID, categoryID); err != nil {
			return models.Post{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return models.Post{}, err
	}
	posts, err := listPostByID(db, postID)
	if err != nil {
		return models.Post{}, err
	}
	return posts, nil
}

func ListPosts(db *sql.DB) ([]models.Post, error) {
	rows, err := db.Query(`
		SELECT p.id, p.title, p.content, p.created_at, u.id, u.nickname, COALESCE(group_concat(c.name, '|'), '')
		FROM posts p
		JOIN users u ON u.id = p.author_id
		LEFT JOIN post_categories pc ON pc.post_id = p.id
		LEFT JOIN categories c ON c.id = pc.category_id
		GROUP BY p.id, p.title, p.content, p.created_at, u.id, u.nickname
		ORDER BY datetime(p.created_at) DESC, p.id DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	posts := make([]models.Post, 0)
	for rows.Next() {
		var post models.Post
		var createdAt string
		var categories string
		if err := rows.Scan(&post.ID, &post.Title, &post.Content, &createdAt, &post.Author.ID, &post.Author.Nickname, &categories); err != nil {
			return nil, err
		}
		post.CreatedAt, err = parseDBTime(createdAt)
		if err != nil {
			return nil, err
		}
		post.Categories = splitCategories(categories)
		posts = append(posts, post)
	}
	return posts, rows.Err()
}

func listPostByID(db *sql.DB, postID int) (models.Post, error) {
	var post models.Post
	var createdAt string
	var categories string
	err := db.QueryRow(`
		SELECT p.id, p.title, p.content, p.created_at, u.id, u.nickname, COALESCE(group_concat(c.name, '|'), '')
		FROM posts p
		JOIN users u ON u.id = p.author_id
		LEFT JOIN post_categories pc ON pc.post_id = p.id
		LEFT JOIN categories c ON c.id = pc.category_id
		WHERE p.id = ?
		GROUP BY p.id, p.title, p.content, p.created_at, u.id, u.nickname
	`, postID).Scan(&post.ID, &post.Title, &post.Content, &createdAt, &post.Author.ID, &post.Author.Nickname, &categories)
	if err != nil {
		return models.Post{}, err
	}
	post.CreatedAt, err = parseDBTime(createdAt)
	if err != nil {
		return models.Post{}, err
	}
	post.Categories = splitCategories(categories)
	return post, nil
}

func uniquePositive(ids []int) []int {
	seen := make(map[int]bool, len(ids))
	var out []int
	for _, id := range ids {
		if id <= 0 || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

func splitCategories(value string) []string {
	if value == "" {
		return nil
	}
	return strings.Split(value, "|")
}
