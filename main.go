package main

import (
	"log"
	"net/http"

	"real-time-forum/database"
	"real-time-forum/handlers"
)

func main() {
	// Initialize database
	database.InitDB("./forum.db")
	defer database.DB.Close()

	mux := http.NewServeMux()

	// Auth Endpoints
	mux.HandleFunc("/api/register", handlers.RegisterHandler)
	mux.HandleFunc("/api/login", handlers.LoginHandler)
	mux.HandleFunc("/api/logout", handlers.LogoutHandler)
	mux.HandleFunc("/api/check-auth", handlers.CheckAuthHandler)
	mux.HandleFunc("/api/categories", handlers.CategoriesHandler)
	mux.HandleFunc("/api/posts", handlers.PostsHandler)
	mux.HandleFunc("/api/comments", handlers.CommentsHandler)
	mux.HandleFunc("/api/chat/users", handlers.ChatUsersHandler)
	mux.HandleFunc("/api/messages", handlers.MessagesHandler)
	mux.HandleFunc("/api/ws", handlers.WebSocketHandler)

	// Serve Frontend (SPA)
	fs := http.FileServer(http.Dir("./static"))
	mux.Handle("/", fs)

	log.Println("Server running on http://localhost:8080")
	if err := http.ListenAndServe(":8080", mux); err != nil {
		log.Fatal(err)
	}
}
