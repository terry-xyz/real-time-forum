package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"real-time-forum/database"
	"real-time-forum/models"

	"github.com/gorilla/websocket"
)

func TestForumSecurityAndAuditFlows(t *testing.T) {
	dir := t.TempDir()
	database.InitDB(filepath.Join(dir, "forum.db"))
	t.Cleanup(func() { database.DB.Close() })
	server := httptest.NewTLSServer(forumHandler())
	defer server.Close()
	client := server.Client()

	request := func(method, path string, cookie *http.Cookie, headers http.Header, body any, want int, out any) *http.Response {
		t.Helper()
		var payload io.Reader
		if raw, ok := body.(string); ok {
			payload = strings.NewReader(raw)
		} else if body != nil {
			var err error
			data, err := json.Marshal(body)
			if err != nil {
				t.Fatal(err)
			}
			payload = bytes.NewReader(data)
		}
		req, err := http.NewRequest(method, server.URL+path, payload)
		if err != nil {
			t.Fatal(err)
		}
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		for key, values := range headers {
			req.Header[key] = values
		}
		if cookie != nil {
			req.AddCookie(cookie)
		}
		res, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		if res.StatusCode != want {
			b, _ := io.ReadAll(res.Body)
			t.Fatalf("%s %s: got %d, want %d: %s", method, path, res.StatusCode, want, b)
		}
		if out != nil {
			if err := json.NewDecoder(res.Body).Decode(out); err != nil {
				t.Fatalf("%s %s: %v", method, path, err)
			}
		}
		return res
	}

	res := request("GET", "/", nil, nil, nil, 200, nil)
	if res.Header.Get("Content-Security-Policy") == "" || res.Header.Get("Strict-Transport-Security") == "" {
		t.Fatal("missing browser security headers")
	}
	request("GET", "/js/theme-preload.js", nil, nil, nil, 200, nil)
	request("GET", "/js/", nil, nil, nil, 404, nil)
	for _, path := range []string{"posts", "categories", "comments", "chat/users", "messages", "ws", "check-auth"} {
		res := request("GET", "/api/"+path, nil, nil, nil, 401, nil)
		if res.Header.Get("Cache-Control") != "no-store" {
			t.Fatalf("missing no-store on %s", path)
		}
	}
	request("GET", "/api/logout", nil, nil, nil, 405, nil)
	request("POST", "/api/login", nil, http.Header{"Origin": {"https://untrusted.example"}}, "{}", 403, nil)
	request("POST", "/api/login", nil, http.Header{"Content-Type": {"text/plain"}}, "{}", 415, nil)
	request("POST", "/api/login", nil, nil, "{} {}", 400, nil)
	request("POST", "/api/login", nil, nil, `{"login":"`+strings.Repeat("x", 64<<10)+`"}`, 413, nil)
	request("POST", "/api/login", nil, nil, models.LoginRequest{Login: "unknown", Password: "unknown"}, 401, nil)

	const password = "a-long-passphrase"
	register := func(nick, email, gender string, age, want int) {
		t.Helper()
		request("POST", "/api/register", nil, nil, models.RegisterRequest{
			Nickname: nick, Email: email, Password: password, Age: age,
			Gender: gender, FirstName: "Forum", LastName: "Member",
		}, want, nil)
	}
	register("Alice", "alice@example.com", "Female", 13, 201)
	register("Bob", "bob@example.com", "Male", 25, 201)
	register("aLiCe", "different@example.com", "Female", 25, 409)
	if _, err := database.DB.Exec(`
		INSERT INTO users (id, nickname, age, gender, first_name, last_name, email, password_hash)
		VALUES ('carol-id', 'Carol', 25, 'Other', 'Forum', 'Member', 'carol@example.com', 'unused')
	`); err != nil {
		t.Fatal(err)
	}

	login := func(identifier string) (*http.Cookie, string) {
		t.Helper()
		var result struct{ User map[string]string }
		res := request("POST", "/api/login", nil, nil, models.LoginRequest{Login: identifier, Password: password}, 200, &result)
		cookies := res.Cookies()
		if len(cookies) != 1 || !cookies[0].Secure || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteStrictMode {
			t.Fatal("expected a protected session cookie")
		}
		return cookies[0], result.User["id"]
	}
	alice, aliceID := login("ALICE")
	bob, bobID := login("BOB@example.com")
	request("GET", "/api/check-auth", alice, nil, nil, 200, nil)
	var chatUsers []models.ChatUser
	request("GET", "/api/chat/users", alice, nil, nil, 200, &chatUsers)
	if len(chatUsers) != 2 || chatUsers[0].Nickname != "Bob" || chatUsers[1].Nickname != "Carol" {
		t.Fatalf("unexpected initial chat order: %+v", chatUsers)
	}

	var post map[string]string
	request("POST", "/api/posts", alice, nil,
		models.CreatePostRequest{Title: "Title", Content: "Content", Categories: []string{"General", "general", "Help"}}, 201, &post)
	var feed []models.Post
	request("GET", "/api/posts?category=general", bob, nil, nil, 200, &feed)
	if len(feed) != 1 || len(feed[0].Categories) != 2 || feed[0].ID != post["id"] {
		t.Fatalf("unexpected categorized feed: %+v", feed)
	}
	request("POST", "/api/comments", bob, nil, models.CreateCommentRequest{PostID: "missing", Content: "Reply"}, 404, nil)
	request("POST", "/api/comments", bob, nil, models.CreateCommentRequest{PostID: post["id"], Content: "Reply"}, 201, nil)
	var comments []models.Comment
	request("GET", "/api/comments?post_id="+post["id"], alice, nil, nil, 200, &comments)
	if len(comments) != 1 || comments[0].UserID != bobID {
		t.Fatalf("unexpected comments: %+v", comments)
	}

	wsURL := "wss" + strings.TrimPrefix(server.URL, "https") + "/api/ws"
	dialer := websocket.Dialer{TLSClientConfig: client.Transport.(*http.Transport).TLSClientConfig}
	dial := func(cookie *http.Cookie, origin string) (*websocket.Conn, *http.Response, error) {
		t.Helper()
		headers := http.Header{"Cookie": {cookie.Name + "=" + cookie.Value}}
		if origin != "" {
			headers.Set("Origin", origin)
		}
		return dialer.Dial(wsURL, headers)
	}
	for _, origin := range []string{"", "http" + strings.TrimPrefix(server.URL, "https"), "https://untrusted.example"} {
		conn, response, err := dial(bob, origin)
		if err == nil || response == nil || response.StatusCode != 403 {
			if conn != nil {
				conn.Close()
			}
			t.Fatalf("unexpected socket handshake for origin %q: %v", origin, err)
		}
		response.Body.Close()
	}
	openSocket := func(cookie *http.Cookie) *websocket.Conn {
		t.Helper()
		socket, _, err := dial(cookie, server.URL)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { socket.Close() })
		socket.SetReadDeadline(time.Now().Add(5 * time.Second))
		return socket
	}
	bobSocket := openSocket(bob)
	aliceSocket := openSocket(alice)

	request("POST", "/api/logout", bob, http.Header{"Sec-Fetch-Site": {"same-site"}}, nil, 403, nil)
	if err := aliceSocket.WriteJSON(map[string]string{"type": "typing", "receiver_id": bobID}); err != nil {
		t.Fatal(err)
	}
	var event struct {
		Type     string
		SenderID string `json:"sender_id"`
		Message  models.Message
	}
	if err := bobSocket.ReadJSON(&event); err != nil || event.Type != "typing" || event.SenderID != aliceID {
		t.Fatalf("unexpected typing event: %+v, %v", event, err)
	}
	for i := range 12 {
		request("POST", "/api/messages", alice, nil, models.CreateMessageRequest{
			ReceiverID: bobID, Content: fmt.Sprintf("Message %d", i),
		}, 201, nil)
		if err := bobSocket.ReadJSON(&event); err != nil || event.Type != "message" || event.Message.SenderName != "Alice" || event.Message.CreatedAt.IsZero() {
			t.Fatalf("unexpected message event: %+v, %v", event, err)
		}
	}
	var history, older []models.Message
	request("GET", "/api/messages?with="+aliceID, bob, nil, nil, 200, &history)
	if len(history) != 10 {
		t.Fatalf("got %d messages, want 10", len(history))
	}
	request("GET", "/api/messages?with="+aliceID+"&before_id="+history[0].ID, bob, nil, nil, 200, &older)
	if len(older) != 2 {
		t.Fatalf("got %d older messages, want 2", len(older))
	}
	var unrelated []models.Message
	request("GET", "/api/messages?with=carol-id", bob, nil, nil, 200, &unrelated)
	if len(unrelated) != 0 {
		t.Fatal("another conversation appeared in Carol's history")
	}
	request("GET", "/api/messages?with=carol-id&before_id="+history[0].ID, bob, nil, nil, 400, nil)
	if _, err := database.DB.Exec("UPDATE messages SET created_at = CURRENT_TIMESTAMP"); err != nil {
		t.Fatal(err)
	}
	request("GET", "/api/messages?with="+aliceID, bob, nil, nil, 200, &history)
	request("GET", "/api/messages?with="+aliceID+"&before_id="+history[0].ID, bob, nil, nil, 200, &older)
	if len(older) != 2 {
		t.Fatalf("same-timestamp cursor returned %d older messages, want 2", len(older))
	}

	request("POST", "/api/logout", bob, nil, nil, 200, nil)
	request("GET", "/api/check-auth", bob, nil, nil, 401, nil)
	assertSocketClosed(t, bobSocket)

	aliceNew, _ := login("alice@example.com")
	request("GET", "/api/check-auth", alice, nil, nil, 401, nil)
	request("GET", "/api/check-auth", aliceNew, nil, nil, 200, nil)
	assertSocketClosed(t, aliceSocket)
	request("POST", "/api/login", nil, nil, models.LoginRequest{Login: "unknown", Password: password}, 429, nil)
	if _, err := database.DB.Exec("UPDATE sessions SET expires_at = datetime('now', '+2 seconds') WHERE id = ?", aliceNew.Value); err != nil {
		t.Fatal(err)
	}
	assertSocketClosed(t, openSocket(aliceNew))
	request("GET", "/api/check-auth", aliceNew, nil, nil, 401, nil)
}

func TestRegistrationRejectsInvalidAgeAndGender(t *testing.T) {
	handler := forumHandler()
	for body, want := range map[string]string{
		`{"age":12,"gender":"Female"}`: "Age must be between 13 and 120",
		`{"age":13,"gender":"Other"}`:  "Select Male or Female",
	} {
		req := httptest.NewRequest("POST", "/api/register", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != http.StatusBadRequest || strings.TrimSpace(res.Body.String()) != want {
			t.Fatalf("registration with %s returned %d: %s", body, res.Code, res.Body.String())
		}
	}
}

func assertSocketClosed(t *testing.T, socket *websocket.Conn) {
	t.Helper()
	socket.SetReadDeadline(time.Now().Add(4 * time.Second))
	if _, _, err := socket.ReadMessage(); !websocket.IsCloseError(err, websocket.CloseAbnormalClosure, websocket.CloseNormalClosure) {
		t.Fatalf("expected server to close the socket, got %v", err)
	}
}

func TestExistingSQLiteSession(t *testing.T) {
	path := filepath.Join(t.TempDir(), "existing.db")
	schema, err := os.ReadFile("database/schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	defer legacy.Close()
	if _, err := legacy.Exec(string(schema)); err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.Exec(`
		INSERT INTO users (id, nickname, age, gender, first_name, last_name, email, password_hash)
		VALUES ('legacy-user', 'Legacy', 25, 'Other', 'Forum', 'Member', 'legacy@example.com', 'unused');
		INSERT INTO sessions (id, user_id, expires_at) VALUES
			('modernc-token', 'legacy-user', '2099-01-01 01:02:03.123 +0200 EET m=+86400.000001'),
			('mattn-token', 'legacy-user', '2099-01-01 01:02:03.123+02:00'),
			('expired-token', 'legacy-user', '2000-01-01 01:02:03 +0200 EET m=+86400.000001');
	`); err != nil {
		t.Fatal(err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}

	database.InitDB(path)
	defer database.DB.Close()
	handler := forumHandler()
	for token, want := range map[string]int{"modernc-token": 200, "mattn-token": 200, "expired-token": 401} {
		req := httptest.NewRequest("GET", "/api/check-auth", nil)
		req.AddCookie(&http.Cookie{Name: "session_token", Value: token})
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != want {
			t.Fatalf("%s got %d, want %d", token, res.Code, want)
		}
	}
	var expires string
	if err := database.DB.QueryRow("SELECT CAST(expires_at AS TEXT) FROM sessions WHERE id = 'modernc-token'").Scan(&expires); err != nil || expires != "2098-12-31 23:02:03" {
		t.Fatalf("unexpected normalized timestamp %q: %v", expires, err)
	}
}
