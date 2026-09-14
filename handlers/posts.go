package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"real-time-forum/database"
	"real-time-forum/models"

	"github.com/gofrs/uuid"
)

// Helper: Authenticate request from cookie session
func getUserFromSession(r *http.Request) (*models.User, error) {
	cookie, err := r.Cookie("session_token")
	if err != nil {
		return nil, err
	}

	var user models.User
	query := `SELECT u.id, u.nickname FROM users u
			  JOIN sessions s ON u.id = s.user_id
			  WHERE s.id = ? AND s.expires_at > ?`

	err = database.DB.QueryRow(query, cookie.Value, time.Now()).Scan(&user.ID, &user.Nickname)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// CategoriesHandler returns all existing category names
func CategoriesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	rows, err := database.DB.Query(`SELECT id, name FROM categories ORDER BY name ASC`)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var categories []models.Category
	for rows.Next() {
		var cat models.Category
		if err := rows.Scan(&cat.ID, &cat.Name); err == nil {
			categories = append(categories, cat)
		}
	}

	json.NewEncoder(w).Encode(categories)
}

// PostHandler handles listing posts (GET) and creating posts (POST)
func PostsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		getPosts(w, r)
	case http.MethodPost:
		createPost(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func createPost(w http.ResponseWriter, r *http.Request) {
	user, err := getUserFromSession(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusBadRequest)
		return
	}

	var req models.CreatePostRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}

	req.Title = strings.TrimSpace(req.Title)
	req.Content = strings.TrimSpace(req.Content)

	if req.Title == "" || req.Content == "" || len(req.Categories) == 0 {
		http.Error(w, "Title, content, and at least one category are required", http.StatusBadRequest)
		return
	}

	postID, _ := uuid.NewV4()

	tx, err := database.DB.Begin()
	if err != nil {
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	// Insert Post
	_, err = tx.Exec(`INSERT INTO posts (id, user_id, title, content) VALUES (?, ?, ?, ?)`,
		postID.String(), user.ID, req.Title, req.Content)
	if err != nil {
		http.Error(w, "Failed to create post", http.StatusInternalServerError)
		return
	}

	// Link Categories (Insert category into DB if missing, then insert join record)
	for _, catName := range req.Categories {
		catName = strings.TrimSpace(strings.ToLower(catName))
		if catName == "" {
			continue
		}

		var categoryID int
		err := tx.QueryRow(`SELECT id FROM categories WHERE LOWER(name) = ?`, catName).Scan(&categoryID)
		if err == sql.ErrNoRows {
			res, err := tx.Exec(`INSERT INTO categories (name) VALUES (?)`, catName)
			if err != nil {
				http.Error(w, "Failed to process categories", http.StatusInternalServerError)
				return
			}
			id, _ := res.LastInsertId()
			categoryID = int(id)
		}

		_, err = tx.Exec(`INSERT INTO post_categories (post_id, category_id) VALUES (?,?)`, postID.String(), categoryID)
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

func getPosts(w http.ResponseWriter, r *http.Request) {
	categoryFilter := r.URL.Query().Get("category")

	var rows *sql.Rows
	var err error

	if categoryFilter != "" {
		//Filter feed posts be category
		query := `
				SELECT p.id, p.user_id, u.nickname, p.title, p.content, p.created_at
				FROM posts p
				JOIN users u ON p.user_id = u.id
				Join post_categories pc ON p.id = pc.post_id
				JOIN categories c ON pc.category_id = c.id
				WHERE LOWER(c.name) = ?
				ORDER BY p.created_at DESC`
		rows, err = database.DB.Query(query, strings.ToLower(categoryFilter))
	} else {
		// Fetch all posts feed
		query := `
				SELECT p.id, p.user_id, u.nickname, p.title, p.content, p.created_at
				FROM posts p
				JOIN users u ON p.user_id = u.id
				ORDER BY p.created_at DESC`
		rows, err = database.DB.Query(query)
	}

	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var posts []models.Post
	for rows.Next() {
		var p models.Post
		if err := rows.Scan(&p.ID, &p.UserID, &p.Author, &p.Title, &p.Content, &p.CreatedAt); err != nil {
			continue
		}

		// Fetch linked categories for each post
		catRows, err := database.DB.Query(`
				SELECT c.name FROM categories c
				JOIN post_categories pc ON c.id = pc.category_id
				WHERE pc.post_id = ?`, p.ID)
		if err == nil {
			for catRows.Next() {
				var cName string
				if err := catRows.Scan(&cName); err == nil {
					p.Categories = append(p.Categories, cName)
				}
			}
			catRows.Close()
		}

		posts = append(posts, p)
	}

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
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func createComment(w http.ResponseWriter, r *http.Request) {
	user, err := getUserFromSession(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req models.CreateCommentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	req.Content = strings.TrimSpace(req.Content)
	if req.Content == "" || req.PostID == "" {
		http.Error(w, "Post ID and content are required", http.StatusBadRequest)
		return
	}

	commentID, _ := uuid.NewV4()
	_, err = database.DB.Exec(`INSERT INTO comments (id, post_id, user_id, content) VALUES (?, ?, ?, ?)`,
		commentID.String(), req.PostID, user.ID, req.Content)

	if err != nil {
		http.Error(w, "Failed to create comment", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Comment added successfully",
	})
}

func getComments(w http.ResponseWriter, r *http.Request) {
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

	rows, err := database.DB.Query(query, postID)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var comments []models.Comment
	for rows.Next() {
		var c models.Comment
		if err := rows.Scan(&c.ID, &c.PostID, &c.UserID, &c.Author, &c.Content, &c.CreatedAt); err == nil {
			comments = append(comments, c)
		}
	}

	json.NewEncoder(w).Encode(comments)
}
