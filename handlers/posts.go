package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"real-time-forum/database"
)

func (a *App) handleCategories(w http.ResponseWriter, r *http.Request) {
	categories, err := database.ListCategories(a.db)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "unable to load categories")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"categories": categories})
}

func (a *App) handlePosts(w http.ResponseWriter, r *http.Request) {
	user, err := a.currentUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	switch r.Method {
	case http.MethodGet:
		posts, err := database.ListPosts(a.db)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "unable to load posts")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"posts": posts})
	case http.MethodPost:
		var input struct {
			Title       string `json:"title"`
			Content     string `json:"content"`
			CategoryIDs []int  `json:"category_ids"`
		}
		if !decodeJSON(w, r, &input) {
			return
		}
		post, err := database.CreatePost(a.db, user.ID, input.Title, input.Content, input.CategoryIDs)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"post": post})
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (a *App) handlePostChildren(w http.ResponseWriter, r *http.Request) {
	user, err := a.currentUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/posts/"), "/")
	if len(parts) != 2 || parts[1] != "comments" {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	postID, err := strconv.Atoi(parts[0])
	if err != nil || postID <= 0 {
		writeError(w, http.StatusBadRequest, "invalid post id")
		return
	}
	switch r.Method {
	case http.MethodGet:
		comments, err := database.ListComments(a.db, postID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "unable to load comments")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"comments": comments})
	case http.MethodPost:
		var input struct {
			Content string `json:"content"`
		}
		if !decodeJSON(w, r, &input) {
			return
		}
		comment, err := database.CreateComment(a.db, user.ID, postID, input.Content)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"comment": comment})
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}
