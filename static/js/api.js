// Callers keep the Response so each feature can preserve its own success/failure
// feedback and chat can discard stale requests before reading the response body.

// postJSON serializes the existing API payloads with the shared JSON request setup.
function postJSON(path, body) {
    return fetch(path, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(body)
    });
}

// readError preserves the server's plain-text feedback and the UI's empty-body fallback.
export async function readError(response) {
    const text = (await response.text()).trim();
    return text || "Something went wrong.";
}

// register converts form age to a number while retaining the registration API's field names.
export function register({ nickname, age, gender, first_name, last_name, email, password }) {
    return postJSON("/api/register", {
        nickname,
        age: Number(age),
        gender,
        first_name,
        last_name,
        email,
        password
    });
}

// login submits the nickname-or-email login field expected by the existing API.
export function login({ login, password }) {
    return postJSON("/api/login", { login, password });
}

// logout ends the cookie session with the existing bodyless POST request.
export function logout() {
    return fetch("/api/logout", { method: "POST" });
}

// checkAuth retrieves the current session without changing the API's user response shape.
export function checkAuth() {
    return fetch("/api/check-auth");
}

// getCategories retrieves the categories used by the feed tabs and sidebar.
export function getCategories() {
    return fetch("/api/categories");
}

// getPosts loads the full UI feed by default and preserves the optional backend category query.
export function getPosts(category = "") {
    const query = category ? `?category=${encodeURIComponent(category)}` : "";
    return fetch(`/api/posts${query}`);
}

// createPost submits the composer fields without trimming user-authored text.
export function createPost({ title, content, categories }) {
    return postJSON("/api/posts", { title, content, categories });
}

// getComments encodes the post ID using the existing comment query field.
export function getComments(postId) {
    return fetch(`/api/comments?post_id=${encodeURIComponent(postId)}`);
}

// createComment submits the post reference and reply text using the existing payload names.
export function createComment({ post_id, content }) {
    return postJSON("/api/comments", { post_id, content });
}

// getChatUsers retrieves the backend's conversation ordering and current presence values.
export function getChatUsers() {
    return fetch("/api/chat/users");
}

// getMessages shares initial-history and older-history queries without owning chat pagination state.
export function getMessages(userId, { limit = 10, beforeId } = {}) {
    let query = `with=${encodeURIComponent(userId)}&limit=${encodeURIComponent(limit)}`;
    if (beforeId !== undefined && beforeId !== null) {
        query += `&before_id=${encodeURIComponent(beforeId)}`;
    }
    return fetch(`/api/messages?${query}`);
}

// sendMessage retains the receiver_id field and original message text expected by the API.
export function sendMessage({ receiver_id, content }) {
    return postJSON("/api/messages", { receiver_id, content });
}

// getChatSocketURL follows the page's protocol and host for the existing same-origin socket endpoint.
export function getChatSocketURL() {
    const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
    return `${protocol}//${window.location.host}/api/ws`;
}
