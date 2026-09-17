const app = document.getElementById("app");

async function loadPosts() {
    const response = await fetch("/api/posts");

    if (!response.ok) {
        app.innerHTML = "<p>Failed to load posts.</p>";
        return;
    }

    const posts = await response.json();

    app.innerHTML = "";

    posts.forEach(post => {
        const article = document.createElement("article");

        article.innerHTML = `
            <h2>${post.title}</h2>
            <p>${post.content}</p>
            <small>Posted by ${post.author}</small>
        `;

        app.appendChild(article);
    });
}

function showLogin() {
    const app = document.getElementById("app");

    app.innerHTML = `
        <h2>Login</h2>

        <form id="login-form">
            <input
                type="text"
                id="login"
                placeholder="Nickname or email"
                required
            >

            <input
                type="password"
                id="password"
                placeholder="Password"
                required
            >

            <button type="submit">Login</button>
        </form>

        <p id="login-message"></p>
    `;

    document
        .getElementById("login-form")
        .addEventListener("submit", login);
}

async function login(event) {
    event.preventDefault();

    const login = document.getElementById("login").value;
    const password = document.getElementById("password").value;
    const message = document.getElementById("login-message");

    try {
        const response = await fetch("/api/login", {
            method: "POST",
            headers: {
                "Content-Type": "application/json"
            },
            body: JSON.stringify({
                login: login,
                password: password
            })
        });

        const text = await response.text();
        if (!response.ok) {
            message.textContent = text;
            return;
        }

        const data = JSON.parse(text);
        message.textContent = data.message;

    } catch (error) {
        console.error("Login error:", error);
        message.textContent = "Something went wrong.";
    }
}

document
    .getElementById("home-button")
    .addEventListener("click", loadPosts);

document
    .getElementById("login-button")
    .addEventListener("click", showLogin)
