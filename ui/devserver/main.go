// Command devserver previews the new UI without changing main.go.
//
// It serves the ui/ folder on :3000 and forwards every /api/ request
// (including the /api/ws WebSocket) to the real forum server on :8080.
//
// Run both from the repository root:
//
//	go run .              # the forum backend on :8080
//	go run ./ui/devserver # the new UI on http://localhost:3000
package main

import (
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
)

func main() {
	backend, err := url.Parse("http://localhost:8080")
	if err != nil {
		log.Fatal(err)
	}

	// ReverseProxy passes WebSocket upgrades through, so real-time chat works too.
	proxy := httputil.NewSingleHostReverseProxy(backend)

	mux := http.NewServeMux()
	mux.Handle("/api/", proxy)
	mux.Handle("/devserver/", http.NotFoundHandler()) // don't serve this file
	mux.Handle("/", http.FileServer(http.Dir("./ui")))

	log.Println("UI preview running on http://localhost:3000 (API proxied to :8080)")
	if err := http.ListenAndServe(":3000", mux); err != nil {
		log.Fatal(err)
	}
}
