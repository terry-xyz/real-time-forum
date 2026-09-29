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

    app.innerHTML = `
        <section>
            <h2>Create a post</h2>
            <form id="post-form">
                <input type="text" id="post-title" placeholder="Title" required>
                <textarea id="post-content" placeholder="Write your post" required></textarea>
                <input type="text" id="post-categories" placeholder="Categories, separated by commas" required>
                <button type="submit">Create post</button>
            </form>
            <p id="post-message"></p>
        </section>
        <section>
            <h2>Posts</h2>
            <div id="post-list"></div>
        </section>
    `;

    document
        .getElementById("post-form")
        .addEventListener("submit", createPost);

    const postList = document.getElementById("post-list");

    posts.forEach(post => {
        const article = document.createElement("article");
        const title = document.createElement("h3");
        const content = document.createElement("p");
        const author = document.createElement("small");
        const categories = document.createElement("p");
        const commentsButton = document.createElement("button");
        const commentsSection = document.createElement("section");

        title.textContent = post.title;
        content.textContent = post.content;
        author.textContent = `Posted by ${post.author}`;
        categories.textContent = `Categories: ${(post.categories || []).join(", ")}`;
        commentsButton.type = "button";
        commentsButton.textContent = "Show comments";
        commentsSection.hidden = true;
        commentsSection.innerHTML = `
            <div class="comment-list"></div>
            <form class="comment-form">
                <textarea placeholder="Write a comment" required></textarea>
                <button type="submit">Add comment</button>
            </form>
            <p class="comment-message"></p>
        `;

        commentsButton.addEventListener("click", async () => {
            commentsSection.hidden = !commentsSection.hidden;
            commentsButton.textContent = commentsSection.hidden ? "Show comments" : "Hide comments";
            if (!commentsSection.hidden) {
                await loadComments(post, commentsSection);
            }
        });
        commentsSection
            .querySelector(".comment-form")
            .addEventListener("submit", event => addComment(event, post, commentsSection));

        article.append(title, content, author, categories, commentsButton, commentsSection);

        postList.appendChild(article);
    });
}

async function loadComments(post, commentsSection) {
    const list = commentsSection.querySelector(".comment-list");

    try {
        const response = await fetch(`/api/comments?post_id=${encodeURIComponent(post.id)}`);
        if (!response.ok) {
            list.textContent = "Failed to load comments.";
            return;
        }

        const comments = await response.json();
        list.textContent = "";
        if (comments.length === 0) {
            list.textContent = "No comments yet.";
            return;
        }

        comments.forEach(comment => {
            const item = document.createElement("p");
            item.textContent = `${comment.author}: ${comment.content}`;
            list.appendChild(item);
        });
    } catch (error) {
        console.error("Comment loading error:", error);
        list.textContent = "Failed to load comments.";
    }
}

async function addComment(event, post, commentsSection) {
    event.preventDefault();

    const form = event.currentTarget;
    const message = commentsSection.querySelector(".comment-message");

    try {
        const response = await fetch("/api/comments", {
            method: "POST",
            headers: {
                "Content-Type": "application/json"
            },
            body: JSON.stringify({
                post_id: post.id,
                content: form.querySelector("textarea").value
            })
        });

        if (!response.ok) {
            message.textContent = (await response.text()).trim();
            return;
        }

        form.reset();
        message.textContent = "";
        await loadComments(post, commentsSection);
    } catch (error) {
        console.error("Comment submission error:", error);
        message.textContent = "Something went wrong.";
    }
}

async function createPost(event) {
    event.preventDefault();

    const message = document.getElementById("post-message");
    const categories = document.getElementById("post-categories").value
        .split(",")
        .map(category => category.trim())
        .filter(category => category !== "");

    try {
        const response = await fetch("/api/posts", {
            method: "POST",
            headers: {
                "Content-Type": "application/json"
            },
            body: JSON.stringify({
                title: document.getElementById("post-title").value,
                content: document.getElementById("post-content").value,
                categories
            })
        });

        if (!response.ok) {
            message.textContent = (await response.text()).trim();
            return;
        }

        await loadPosts();
    } catch (error) {
        console.error("Post creation error:", error);
        message.textContent = "Something went wrong.";
    }
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
