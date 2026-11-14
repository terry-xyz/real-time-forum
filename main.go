package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"real-time-forum/database"
	"real-time-forum/handlers"
)

// main initializes SQLite, registers the API and static frontend routes, and starts the HTTP server.
func main() {
	stop, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	certFile, keyFile := os.Getenv("FORUM_TLS_CERT"), os.Getenv("FORUM_TLS_KEY")
	if (certFile == "") != (keyFile == "") {
		log.Fatal("Set both FORUM_TLS_CERT and FORUM_TLS_KEY to enable HTTPS")
	}
	address := os.Getenv("FORUM_ADDR")
	if address != "" && certFile == "" {
		log.Fatal("FORUM_ADDR requires HTTPS certificates")
	}
	if address == "" {
		address = "localhost:8080"
	}
	database.InitDB("./forum.db")
	defer database.DB.Close()

	server := &http.Server{
		Addr:              address,
		Handler:           forumHandler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      20 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 << 10,
	}
	serverDone := make(chan error, 1)
	go func() {
		if certFile != "" {
			log.Printf("Forum listening with HTTPS on %s", address)
			serverDone <- server.ListenAndServeTLS(certFile, keyFile)
		} else {
			log.Printf("Forum listening at http://%s", address)
			serverDone <- server.ListenAndServe()
		}
	}()

	select {
	case err := <-serverDone:
		if !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	case <-stop.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdown); err != nil {
			log.Printf("Forum shutdown: %v", err)
			server.Close()
		}
	}
}

func forumHandler() http.Handler {
	mux := http.NewServeMux()

	// Auth Endpoints
	mux.HandleFunc("POST /api/register", handlers.RegisterHandler)
	mux.HandleFunc("POST /api/login", handlers.LoginHandler)
	mux.HandleFunc("POST /api/logout", handlers.LogoutHandler)
	mux.HandleFunc("GET /api/check-auth", handlers.CheckAuthHandler)
	mux.HandleFunc("GET /api/categories", handlers.CategoriesHandler)
	mux.HandleFunc("/api/posts", handlers.PostsHandler)
	mux.HandleFunc("/api/comments", handlers.CommentsHandler)
	mux.HandleFunc("GET /api/chat/users", handlers.ChatUsersHandler)
	mux.HandleFunc("/api/messages", handlers.MessagesHandler)
	mux.HandleFunc("GET /api/ws", handlers.WebSocketHandler)

	// Serve Frontend (SPA)
	fs := http.FileServer(http.Dir("./static"))
	mux.Handle("GET /{$}", fs)
	mux.Handle("GET /style.css", fs)
	mux.Handle("GET /js/{file}", fs)
	return protectRequests(mux)
}
