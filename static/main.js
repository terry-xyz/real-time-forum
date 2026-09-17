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
                id="login-identifier"
                placeholder="Nickname or email"
                required
            >

            <input
                type="password"
                id="login-password"
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
}

document
    .getElementById("home-button")
    .addEventListener("click", loadPosts);

document
    .getElementById("login-button")
    .addEventListener("click", showLogin)
