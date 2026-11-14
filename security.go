package main

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

func protectRequests(next http.Handler) http.Handler {
	var origins http.CrossOriginProtection
	var mu sync.Mutex
	var window time.Time
	authAttempts := make(map[string]int)
	apiRequests := make(map[string]int)
	authSlots := make(chan struct{}, 8)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'self'; base-uri 'none'; object-src 'none'; frame-ancestors 'none'; form-action 'self'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		if r.TLS != nil {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000")
		}
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		if err := origins.Check(r); err != nil {
			http.Error(w, "Cross-origin request rejected", http.StatusForbidden)
			return
		}

		ip, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			ip = r.RemoteAddr
		}
		now := time.Now()
		authRequest := r.Method == http.MethodPost && (r.URL.Path == "/api/login" || r.URL.Path == "/api/register")
		mu.Lock()
		if !now.Before(window) {
			clear(authAttempts)
			clear(apiRequests)
			window = now.Add(time.Minute)
		}
		// shortcut: limits are per process and source IP, move them to the ingress when running behind a proxy or across replicas.
		limited := apiRequests[ip] >= 240 || (apiRequests[ip] == 0 && len(apiRequests) >= 4096)
		if !limited {
			apiRequests[ip]++
			if authRequest {
				limited = authAttempts[ip] >= 10
				if !limited {
					authAttempts[ip]++
				}
			}
		}
		mu.Unlock()
		if limited {
			w.Header().Set("Retry-After", "60")
			http.Error(w, "Too many requests", http.StatusTooManyRequests)
			return
		}
		if authRequest {
			select {
			case authSlots <- struct{}{}:
				defer func() { <-authSlots }()
			default:
				w.Header().Set("Retry-After", "1")
				http.Error(w, "Sign-in and registration are busy", http.StatusServiceUnavailable)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
