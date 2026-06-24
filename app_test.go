package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

type testClient struct {
	server *httptest.Server
	client *http.Client
}

func newTestClient(t *testing.T) (*testClient, *sql.DB) {
	t.Helper()

	db, err := InitDB(":memory:")
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}

	app := NewApp(db)
	server := httptest.NewServer(app.Routes())
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookie jar: %v", err)
	}
	client := server.Client()
	client.Jar = jar
	t.Cleanup(func() {
		server.Close()
		db.Close()
	})

	return &testClient{server: server, client: client}, db
}

func (c *testClient) postJSON(t *testing.T, path string, body any, status int) map[string]any {
	t.Helper()

	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, c.server.URL+path, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	res, err := c.client.Do(req)
	if err != nil {
		t.Fatalf("post %s: %v", path, err)
	}
	defer res.Body.Close()

	if res.StatusCode != status {
		t.Fatalf("post %s status = %d, want %d", path, res.StatusCode, status)
	}

	var decoded map[string]any
	if err := json.NewDecoder(res.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return decoded
}

func (c *testClient) getJSON(t *testing.T, path string, status int) map[string]any {
	t.Helper()

	res, err := c.client.Get(c.server.URL + path)
	if err != nil {
		t.Fatalf("get %s: %v", path, err)
	}
	defer res.Body.Close()

	if res.StatusCode != status {
		t.Fatalf("get %s status = %d, want %d", path, res.StatusCode, status)
	}

	var decoded map[string]any
	if err := json.NewDecoder(res.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return decoded
}

func registerAndLogin(t *testing.T, c *testClient, nickname string) map[string]any {
	t.Helper()

	email := strings.ToLower(nickname) + "@example.com"
	c.postJSON(t, "/api/register", map[string]any{
		"nickname":   nickname,
		"age":        29,
		"gender":     "Male",
		"first_name": nickname,
		"last_name":  "Tester",
		"email":      email,
		"password":   "password123",
	}, http.StatusCreated)

	return c.postJSON(t, "/api/login", map[string]any{
		"identifier": nickname,
		"password":   "password123",
	}, http.StatusOK)
}

func TestAuthRequiresFullRegistrationAndAllowsNicknameOrEmailLogin(t *testing.T) {
	c, _ := newTestClient(t)

	c.getJSON(t, "/api/me", http.StatusUnauthorized)
	c.postJSON(t, "/api/register", map[string]any{
		"nickname":   "Ada",
		"age":        34,
		"gender":     "female",
		"first_name": "Ada",
		"last_name":  "Lovelace",
		"email":      "ada@example.com",
		"password":   "password123",
	}, http.StatusCreated)
	c.postJSON(t, "/api/login", map[string]any{
		"identifier": "missing@example.com",
		"password":   "password123",
	}, http.StatusUnauthorized)

	login := c.postJSON(t, "/api/login", map[string]any{
		"identifier": "ada@example.com",
		"password":   "password123",
	}, http.StatusOK)
	user := login["user"].(map[string]any)
	if user["nickname"] != "Ada" {
		t.Fatalf("nickname = %v, want Ada", user["nickname"])
	}

	c.getJSON(t, "/api/me", http.StatusOK)
	c.postJSON(t, "/api/logout", map[string]any{}, http.StatusOK)
	c.getJSON(t, "/api/me", http.StatusUnauthorized)
}

func TestRegistrationRejectsIncompleteEmailAndUnsupportedGender(t *testing.T) {
	c, _ := newTestClient(t)

	valid := map[string]any{
		"nickname":   "Ada",
		"age":        34,
		"gender":     "Female",
		"first_name": "Ada",
		"last_name":  "Lovelace",
		"email":      "ada@example.com",
		"password":   "password123",
	}

	badEmail := cloneMap(valid)
	badEmail["email"] = "ada@example"
	c.postJSON(t, "/api/register", badEmail, http.StatusBadRequest)

	badGender := cloneMap(valid)
	badGender["gender"] = "unspecified"
	c.postJSON(t, "/api/register", badGender, http.StatusBadRequest)
}

func TestRegistrationFormUsesRestrictedGenderSelect(t *testing.T) {
	html, err := os.ReadFile("static/index.html")
	if err != nil {
		t.Fatalf("read index: %v", err)
	}
	source := string(html)
	if !strings.Contains(source, `<select name="gender" required>`) {
		t.Fatalf("registration form must use a required gender select")
	}
	if !strings.Contains(source, `<option value="Male">Male</option>`) || !strings.Contains(source, `<option value="Female">Female</option>`) {
		t.Fatalf("gender select must offer Male and Female options")
	}
	if strings.Contains(source, `name="gender" required></label>`) {
		t.Fatalf("gender must not be a free-text input")
	}
}

func TestRegistrationEmailInputUsesNativeDomainPattern(t *testing.T) {
	html, err := os.ReadFile("static/index.html")
	if err != nil {
		t.Fatalf("read index: %v", err)
	}
	source := string(html)
	if !strings.Contains(source, `name="email" type="email"`) {
		t.Fatalf("registration form must use a native email input")
	}
	if !strings.Contains(source, `pattern="[^\s@]+@[^\s@]+\.[^\s@]+"`) {
		t.Fatalf("email input must require a dotted domain through native browser validation")
	}

	script, err := os.ReadFile("static/app.js")
	if err != nil {
		t.Fatalf("read app js: %v", err)
	}
	if !strings.Contains(string(script), "clearAuthError") {
		t.Fatalf("frontend should clear stale auth errors while users edit the auth forms")
	}
}

func TestFrontendUsesNamedControlsForPostsAndMessages(t *testing.T) {
	script, err := os.ReadFile("static/app.js")
	if err != nil {
		t.Fatalf("read app js: %v", err)
	}
	source := string(script)
	for _, unsafeAccess := range []string{"form.title.value", "form.content.value", "event.currentTarget.content"} {
		if strings.Contains(source, unsafeAccess) {
			t.Fatalf("frontend should not use ambiguous form property access %q", unsafeAccess)
		}
	}
	if !strings.Contains(source, "controlValue(form, 'title')") || !strings.Contains(source, "controlValue(form, 'content')") {
		t.Fatalf("post form should read controls through named elements")
	}
	if !strings.Contains(source, "messageInput()") {
		t.Fatalf("message form should read the message input through a stable selector")
	}
}

func TestPostFormShowsCategorySelectionError(t *testing.T) {
	html, err := os.ReadFile("static/index.html")
	if err != nil {
		t.Fatalf("read index: %v", err)
	}
	if !strings.Contains(string(html), `id="post-error"`) {
		t.Fatalf("post form must include a visible error target")
	}

	script, err := os.ReadFile("static/app.js")
	if err != nil {
		t.Fatalf("read app js: %v", err)
	}
	source := string(script)
	if !strings.Contains(source, "Please select at least one category") {
		t.Fatalf("post form must tell the user to select a category")
	}
	if !strings.Contains(source, "$('#post-error').textContent") {
		t.Fatalf("post submit errors must be rendered in the post error target")
	}
}

func TestPostsAndCommentsRequireLoginAndAreReadableFromFeed(t *testing.T) {
	c, _ := newTestClient(t)

	c.postJSON(t, "/api/posts", map[string]any{
		"title":        "Denied",
		"content":      "No session",
		"category_ids": []int{1},
	}, http.StatusUnauthorized)

	registerAndLogin(t, c, "Grace")
	created := c.postJSON(t, "/api/posts", map[string]any{
		"title":        "First post",
		"content":      "Hello from the SPA",
		"category_ids": []int{1, 2},
	}, http.StatusCreated)
	post := created["post"].(map[string]any)
	postID := int(post["id"].(float64))

	feed := c.getJSON(t, "/api/posts", http.StatusOK)
	posts := feed["posts"].([]any)
	if len(posts) != 1 || posts[0].(map[string]any)["title"] != "First post" {
		t.Fatalf("feed posts = %#v", posts)
	}

	comment := c.postJSON(t, "/api/posts/"+strconv.Itoa(postID)+"/comments", map[string]any{
		"content": "This is a comment",
	}, http.StatusCreated)
	if comment["comment"].(map[string]any)["author"].(map[string]any)["nickname"] != "Grace" {
		t.Fatalf("comment author = %#v", comment["comment"])
	}

	comments := c.getJSON(t, "/api/posts/"+strconv.Itoa(postID)+"/comments", http.StatusOK)
	if len(comments["comments"].([]any)) != 1 {
		t.Fatalf("comments = %#v", comments)
	}
}

func TestEmptyCollectionsSerializeAsArrays(t *testing.T) {
	c, db := newTestClient(t)

	registerAndLogin(t, c, "Grace")

	posts := c.getJSON(t, "/api/posts", http.StatusOK)["posts"]
	if posts == nil {
		t.Fatalf("posts should be an empty array, got nil")
	}
	if len(posts.([]any)) != 0 {
		t.Fatalf("posts = %#v, want empty", posts)
	}

	users := c.getJSON(t, "/api/users", http.StatusOK)["users"]
	if users == nil {
		t.Fatalf("users should be an empty array, got nil")
	}
	if len(users.([]any)) != 0 {
		t.Fatalf("users = %#v, want empty", users)
	}

	created := c.postJSON(t, "/api/posts", map[string]any{
		"title":        "First post",
		"content":      "Hello from the SPA",
		"category_ids": []int{1},
	}, http.StatusCreated)
	postID := int(created["post"].(map[string]any)["id"].(float64))

	comments := c.getJSON(t, "/api/posts/"+strconv.Itoa(postID)+"/comments", http.StatusOK)["comments"]
	if comments == nil {
		t.Fatalf("comments should be an empty array, got nil")
	}
	if len(comments.([]any)) != 0 {
		t.Fatalf("comments = %#v, want empty", comments)
	}

	bobID, err := CreateUser(db, RegistrationInput{Nickname: "Bob", Age: 30, Gender: "Male", FirstName: "Bob", LastName: "Beta", Email: "bob@example.com", Password: "password123"})
	if err != nil {
		t.Fatalf("create bob: %v", err)
	}
	messages := c.getJSON(t, "/api/messages?with="+strconv.Itoa(bobID), http.StatusOK)["messages"]
	if messages == nil {
		t.Fatalf("messages should be an empty array, got nil")
	}
	if len(messages.([]any)) != 0 {
		t.Fatalf("messages = %#v, want empty", messages)
	}
}

func TestChatUsersSortByRecentConversationThenAlphabeticallyForNewUsers(t *testing.T) {
	c, db := newTestClient(t)

	alice := registerAndLogin(t, c, "Alice")
	aliceID := int(alice["user"].(map[string]any)["id"].(float64))
	bobID, err := CreateUser(db, RegistrationInput{Nickname: "Bob", Age: 30, Gender: "male", FirstName: "Bob", LastName: "Beta", Email: "bob@example.com", Password: "password123"})
	if err != nil {
		t.Fatalf("create bob: %v", err)
	}
	carolID, err := CreateUser(db, RegistrationInput{Nickname: "Carol", Age: 31, Gender: "female", FirstName: "Carol", LastName: "Gamma", Email: "carol@example.com", Password: "password123"})
	if err != nil {
		t.Fatalf("create carol: %v", err)
	}
	if _, err := CreateMessage(db, carolID, aliceID, "latest", time.Now().Add(time.Minute)); err != nil {
		t.Fatalf("create message: %v", err)
	}

	users := c.getJSON(t, "/api/users", http.StatusOK)["users"].([]any)
	first := users[0].(map[string]any)
	second := users[1].(map[string]any)
	if first["nickname"] != "Carol" || second["nickname"] != "Bob" || int(second["id"].(float64)) != bobID {
		t.Fatalf("users order = %#v", users)
	}
}

func TestMessagesReturnLatestTenAndOlderPages(t *testing.T) {
	c, db := newTestClient(t)

	alice := registerAndLogin(t, c, "Alice")
	aliceID := int(alice["user"].(map[string]any)["id"].(float64))
	bobID, err := CreateUser(db, RegistrationInput{Nickname: "Bob", Age: 30, Gender: "male", FirstName: "Bob", LastName: "Beta", Email: "bob@example.com", Password: "password123"})
	if err != nil {
		t.Fatalf("create bob: %v", err)
	}
	for i := 1; i <= 25; i++ {
		if _, err := CreateMessage(db, aliceID, bobID, "message "+strconv.Itoa(i), time.Now().Add(time.Duration(i)*time.Second)); err != nil {
			t.Fatalf("create message %d: %v", i, err)
		}
	}

	firstPage := c.getJSON(t, "/api/messages?with="+strconv.Itoa(bobID), http.StatusOK)["messages"].([]any)
	if len(firstPage) != 10 || firstPage[0].(map[string]any)["content"] != "message 16" || firstPage[9].(map[string]any)["content"] != "message 25" {
		t.Fatalf("first page = %#v", firstPage)
	}

	beforeID := int(firstPage[0].(map[string]any)["id"].(float64))
	older := c.getJSON(t, "/api/messages?with="+strconv.Itoa(bobID)+"&before_id="+strconv.Itoa(beforeID), http.StatusOK)["messages"].([]any)
	if len(older) != 10 || older[0].(map[string]any)["content"] != "message 6" || older[9].(map[string]any)["content"] != "message 15" {
		t.Fatalf("older page = %#v", older)
	}
}

func TestMessagesReturnEmptyArrayForNewConversation(t *testing.T) {
	c, db := newTestClient(t)

	registerAndLogin(t, c, "Alice")
	bobID, err := CreateUser(db, RegistrationInput{Nickname: "Bob", Age: 30, Gender: "Male", FirstName: "Bob", LastName: "Beta", Email: "bob@example.com", Password: "password123"})
	if err != nil {
		t.Fatalf("create bob: %v", err)
	}

	messages := c.getJSON(t, "/api/messages?with="+strconv.Itoa(bobID), http.StatusOK)["messages"]
	if messages == nil {
		t.Fatalf("messages should be an empty array, got nil")
	}
	if len(messages.([]any)) != 0 {
		t.Fatalf("messages = %#v, want empty", messages)
	}
}

func TestWebSocketDeliversPrivateMessagesInRealTime(t *testing.T) {
	c1, db := newTestClient(t)
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookie jar: %v", err)
	}
	c2Client := &http.Client{Transport: c1.client.Transport, Jar: jar}
	c2 := &testClient{server: c1.server, client: c2Client}

	alice := registerAndLogin(t, c1, "Alice")
	bobID, err := CreateUser(db, RegistrationInput{Nickname: "Bob", Age: 30, Gender: "male", FirstName: "Bob", LastName: "Beta", Email: "bob@example.com", Password: "password123"})
	if err != nil {
		t.Fatalf("create bob: %v", err)
	}
	c2.postJSON(t, "/api/login", map[string]any{"identifier": "Bob", "password": "password123"}, http.StatusOK)

	wsURL := "ws" + strings.TrimPrefix(c1.server.URL, "http") + "/ws"
	aliceWS := dialWS(t, c1, wsURL)
	defer aliceWS.Close()
	bobWS := dialWS(t, c2, wsURL)
	defer bobWS.Close()

	if err := aliceWS.WriteJSON(map[string]any{"type": "private_message", "to_id": bobID, "content": "hello now"}); err != nil {
		t.Fatalf("write websocket: %v", err)
	}

	var received map[string]any
	if err := bobWS.ReadJSON(&received); err != nil {
		t.Fatalf("read websocket: %v", err)
	}
	if received["type"] != "private_message" || received["content"] != "hello now" {
		t.Fatalf("received = %#v", received)
	}
	if int(received["from"].(map[string]any)["id"].(float64)) != int(alice["user"].(map[string]any)["id"].(float64)) {
		t.Fatalf("sender = %#v", received["from"])
	}
}

func dialWS(t *testing.T, c *testClient, wsURL string) *websocket.Conn {
	t.Helper()

	header := http.Header{}
	u, err := url.Parse(c.server.URL)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	for _, cookie := range c.client.Jar.Cookies(u) {
		header.Add("Cookie", cookie.String())
	}

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, header)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	return conn
}

func cloneMap(input map[string]any) map[string]any {
	out := make(map[string]any, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}
