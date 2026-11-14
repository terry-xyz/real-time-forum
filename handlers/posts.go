package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"

	"real-time-forum/database"
	"real-time-forum/models"

	"github.com/gofrs/uuid"
)

// CategoriesHandler returns all existing category names
func CategoriesHandler(w http.ResponseWriter, r *http.Request) {
	if _, err := getUserFromSession(r); err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	rows, err := database.DB.QueryContext(r.Context(), `SELECT id, name FROM categories ORDER BY name ASC`)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var categories []models.Category
	for rows.Next() {
		var cat models.Category
		if err := rows.Scan(&cat.ID, &cat.Name); err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
		categories = append(categories, cat)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(categories)
}

// PostsHandler dispatches post requests to the feed reader or creator.
func PostsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		getPosts(w, r)
	case http.MethodPost:
		createPost(w, r)
	default:
		w.Header().Set("Allow", "GET, POST")
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// createPost validates and stores a post with its selected categories in one transaction.
func createPost(w http.ResponseWriter, r *http.Request) {
	user, err := getUserFromSession(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusBadRequest)
		return
	}

	var req models.CreatePostRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	req.Title = strings.TrimSpace(req.Title)
	req.Content = strings.TrimSpace(req.Content)

	if req.Title == "" || len([]rune(req.Title)) > 200 || req.Content == "" || len(req.Content) > 32<<10 ||
		len(req.Categories) == 0 || len(req.Categories) > 10 {
		http.Error(w, "Provide a title up to 200 characters, content up to 32 KB, and 1 to 10 categories", http.StatusBadRequest)
		return
	}
	for i, name := range req.Categories {
		name = strings.ToLower(strings.TrimSpace(name))
		if name == "" || len(name) > 64 {
			http.Error(w, "Category names must be between 1 and 64 bytes", http.StatusBadRequest)
			return
		}
		req.Categories[i] = name
	}

	postID, err := uuid.NewV4()
	if err != nil {
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}

	ctx := r.Context()
	tx, err := database.DB.BeginTx(ctx, nil)
	if err != nil {
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	// Insert Post
	_, err = tx.ExecContext(ctx, `INSERT INTO posts (id, user_id, title, content) VALUES (?, ?, ?, ?)`,
		postID.String(), user.ID, req.Title, req.Content)
	if err != nil {
		http.Error(w, "Failed to create post", http.StatusInternalServerError)
		return
	}

	// Link Categories (Insert category into DB if missing, then insert join record)
	for _, catName := range req.Categories {
		var categoryID int
		err := tx.QueryRowContext(ctx, `SELECT id FROM categories WHERE LOWER(name) = ? ORDER BY id LIMIT 1`, catName).Scan(&categoryID)
		if err == sql.ErrNoRows {
			err = tx.QueryRowContext(ctx, `INSERT INTO categories (name) VALUES (?) RETURNING id`, catName).Scan(&categoryID)
		}
		if err != nil {
			http.Error(w, "Failed to process categories", http.StatusInternalServerError)
			return
		}

		_, err = tx.ExecContext(ctx, `INSERT INTO post_categories (post_id, category_id) VALUES (?,?) ON CONFLICT DO NOTHING`, postID.String(), categoryID)
		if err != nil {
			http.Error(w, "Failed to link category", http.StatusInternalServerError)
			return
		}
	}

	if err := tx.Commit(); err != nil {
		http.Error(w, "Failed to save post", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Post created successfully",
		"id":      postID.String(),
	})
}

// getPosts returns the newest posts, optionally filtered by category, with their category names.
func getPosts(w http.ResponseWriter, r *http.Request) {
	if _, err := getUserFromSession(r); err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	categoryFilter := r.URL.Query().Get("category")

	rows, err := database.DB.QueryContext(r.Context(), `
		SELECT p.id, p.user_id, u.nickname, p.title, p.content, p.created_at,
			(SELECT json_group_array(c.name)
			 FROM post_categories pc JOIN categories c ON c.id = pc.category_id
			 WHERE pc.post_id = p.id)
		FROM posts p JOIN users u ON u.id = p.user_id
		WHERE ? = '' OR EXISTS (
			SELECT 1 FROM post_categories pc JOIN categories c ON c.id = pc.category_id
			WHERE pc.post_id = p.id AND LOWER(c.name) = ?
		)
		ORDER BY p.created_at DESC, p.id DESC
	`, categoryFilter, strings.ToLower(categoryFilter))

	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var posts []models.Post
	for rows.Next() {
		var p models.Post
		var categories string
		if err := rows.Scan(&p.ID, &p.UserID, &p.Author, &p.Title, &p.Content, &p.CreatedAt, &categories); err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
		if err := json.Unmarshal([]byte(categories), &p.Categories); err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
		posts = append(posts, p)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(posts)
}

// CommentsHandler manages fetching and adding comments to a post
func CommentsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		getComments(w, r)
	case http.MethodPost:
		createComment(w, r)
	default:
		w.Header().Set("Allow", "GET, POST")
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// createComment validates and stores a comment for the authenticated user.
func createComment(w http.ResponseWriter, r *http.Request) {
	user, err := getUserFromSession(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req models.CreateCommentRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	req.Content = strings.TrimSpace(req.Content)
	if req.Content == "" || len(req.Content) > 8<<10 || req.PostID == "" {
		http.Error(w, "Post ID and content up to 8 KB are required", http.StatusBadRequest)
		return
	}

	commentID, err := uuid.NewV4()
	if err != nil {
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}
	var createdID string
	err = database.DB.QueryRowContext(r.Context(), `
		INSERT INTO comments (id, post_id, user_id, content)
		SELECT ?, id, ?, ? FROM posts WHERE id = ? RETURNING id
	`, commentID.String(), user.ID, req.Content, req.PostID).Scan(&createdID)
	if err == sql.ErrNoRows {
		http.Error(w, "Post not found", http.StatusNotFound)
		return
	}

	if err != nil {
		http.Error(w, "Failed to create comment", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Comment added successfully",
	})
}

// getComments returns a post’s comments in chronological order.
func getComments(w http.ResponseWriter, r *http.Request) {
	if _, err := getUserFromSession(r); err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	postID := r.URL.Query().Get("post_id")
	if postID == "" {
		http.Error(w, "post_id parameter is required", http.StatusBadRequest)
		return
	}

	query := `
			SELECT c.id, c.post_id, c.user_id, u.nickname, c.content, c.created_at
			FROM comments c
			JOIN users u ON c.user_id = u.id
			WHERE c.post_id = ?
			ORDER BY c.created_at ASC`

	rows, err := database.DB.QueryContext(r.Context(), query, postID)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var comments []models.Comment
	for rows.Next() {
		var c models.Comment
		if err := rows.Scan(&c.ID, &c.PostID, &c.UserID, &c.Author, &c.Content, &c.CreatedAt); err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
		comments = append(comments, c)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(comments)
}
