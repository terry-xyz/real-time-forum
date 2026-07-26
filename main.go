package main

import (
	"fmt"
	"net/http"
	"os"
	"strings"

	"real-time-forum/database"
	"real-time-forum/handlers"
)

const defaultPort = "8080"

func main() {
	if err := run(); err != nil {
		fmt.Println(err)
	}
}

func configuredAddr() string {
	port := strings.TrimSpace(os.Getenv("PORT"))
	if port == "" {
		port = defaultPort
	}
	if strings.Contains(port, ":") {
		return port
	}
	return ":" + port
}

func configuredDBPath() string {
	path := strings.TrimSpace(os.Getenv("DATABASE_PATH"))
	if path == "" {
		return database.DefaultDBPath
	}
	return path
}

func run() error {
	db, err := database.InitDB(configuredDBPath())
	if err != nil {
		return err
	}
	defer db.Close()
	app := handlers.NewApp(db)
	addr := configuredAddr()
	fmt.Printf("Listening on http://localhost%s\n", addr)
	return http.ListenAndServe(addr, app.Routes())
}
