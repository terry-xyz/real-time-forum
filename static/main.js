const app = document.getElementById("app");
let currentUser = null;

function updateNavigation() {
    const loggedIn = currentUser !== null;
    document.getElementById("login-button").hidden = loggedIn;
    document.getElementById("register-button").hidden = loggedIn;
    document.getElementById("logout-button").hidden = !loggedIn;
}

async function loadPosts() {
    if (!currentUser) {
        showLogin();
        return;
    }

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

function showRegister() {
    app.innerHTML = `
        <h2>Register</h2>

        <form id="register-form">
            <input type="text" id="nickname" placeholder="Nickname" required>
            <input type="number" id="age" placeholder="Age" min="1" required>
            <input type="text" id="gender" placeholder="Gender" required>
            <input type="text" id="first-name" placeholder="First name" required>
            <input type="text" id="last-name" placeholder="Last name" required>
            <input type="email" id="email" placeholder="Email" required>
            <input type="password" id="register-password" placeholder="Password" required>
            <button type="submit">Register</button>
        </form>

        <p id="register-message"></p>
    `;

    document
        .getElementById("register-form")
        .addEventListener("submit", register);
}

async function register(event) {
    event.preventDefault();

    const message = document.getElementById("register-message");

    try {
        const response = await fetch("/api/register", {
            method: "POST",
            headers: {
                "Content-Type": "application/json"
            },
            body: JSON.stringify({
                nickname: document.getElementById("nickname").value,
                age: Number(document.getElementById("age").value),
                gender: document.getElementById("gender").value,
                first_name: document.getElementById("first-name").value,
                last_name: document.getElementById("last-name").value,
                email: document.getElementById("email").value,
                password: document.getElementById("register-password").value
            })
        });

        if (!response.ok) {
            message.textContent = (await response.text()).trim();
            return;
        }

        message.textContent = "Registered successfully";
    } catch (error) {
        console.error("Registration error:", error);
        message.textContent = "Something went wrong.";
    }
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

        if (!response.ok) {
            message.textContent = (await response.text()).trim();
            return;
        }

        const data = await response.json();
        currentUser = data.user;
        updateNavigation();
        await loadPosts();

    } catch (error) {
        console.error("Login error:", error);
        message.textContent = "Something went wrong.";
    }
}

async function logout() {
    try {
        const response = await fetch("/api/logout", { method: "POST" });
        if (!response.ok) {
            app.textContent = "Logout failed. Please try again.";
            return;
        }

        currentUser = null;
        updateNavigation();
        showLogin();
    } catch (error) {
        console.error("Logout error:", error);
        app.textContent = "Logout failed. Please try again.";
    }
}

async function restoreSession() {
    try {
        const response = await fetch("/api/check-auth");
        if (!response.ok) {
            currentUser = null;
            updateNavigation();
            showLogin();
            return;
        }

        currentUser = await response.json();
        updateNavigation();
        await loadPosts();
    } catch (error) {
        console.error("Session check error:", error);
        currentUser = null;
        updateNavigation();
        showLogin();
    }
}

document
    .getElementById("home-button")
    .addEventListener("click", loadPosts);

document
    .getElementById("login-button")
    .addEventListener("click", showLogin)

document
    .getElementById("register-button")
    .addEventListener("click", showRegister)

document
    .getElementById("logout-button")
    .addEventListener("click", logout)

restoreSession();
