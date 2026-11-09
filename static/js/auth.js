import * as api from "./api.js";
import { $, showToast } from "./dom.js";
import { state } from "./state.js";

let initialized = false;
let onAuthenticated = async () => {};
let onLoggedOut = () => {};

// showAuth switches the persistent forms and focuses the first field of the selected flow.
export function showAuth(mode = "login") {
    $("app-view").hidden = true;
    $("chat-drawer").hidden = true;
    $("chat-fab").hidden = true;
    $("auth-view").hidden = false;
    $("login-form").hidden = mode !== "login";
    $("register-form").hidden = mode !== "register";
    $("auth-heading").textContent = mode === "login" ? "Sign in to Real-Time Forum" : "Join today.";
    $(mode === "login" ? "login" : "first-name").focus();
}

// enterApp reveals the authenticated shell before main paints identity and starts app features.
async function enterApp() {
    $("auth-view").hidden = true;
    $("app-view").hidden = false;
    $("chat-fab").hidden = false;
    await onAuthenticated(state.currentUser);
}

// leaveApp clears shared session state while main disposes of feature-owned chat resources.
function leaveApp() {
    state.currentUser = null;
    state.unreadUserIds.clear();
    onLoggedOut();
    showAuth("login");
}

// register preserves the UI's registration-to-login confirmation without replacing either form.
async function register(event) {
    event.preventDefault();
    const form = event.currentTarget; // Capture before awaiting because currentTarget is cleared after dispatch.
    const message = $("register-message");
    message.classList.remove("success");

    try {
        const response = await api.register({
            nickname: $("nickname").value,
            age: $("age").value,
            gender: $("gender").value,
            first_name: $("first-name").value,
            last_name: $("last-name").value,
            email: $("email").value,
            password: $("register-password").value
        });
        if (!response.ok) {
            message.textContent = await api.readError(response);
            return;
        }

        const nickname = $("nickname").value;
        form.reset();
        showAuth("login");
        $("login").value = nickname;
        $("password").focus();
        const loginMessage = $("login-message");
        loginMessage.textContent = "Account created. Sign in to continue.";
        loginMessage.classList.add("success");
    } catch (error) {
        console.error("Registration error:", error);
        message.textContent = "Something went wrong.";
    }
}

// login stores the API's nested user response before entering the authenticated shell.
async function login(event) {
    event.preventDefault();
    const form = event.currentTarget; // Capture before awaiting so a successful login can reset this form.
    const message = $("login-message");
    message.classList.remove("success");

    try {
        const response = await api.login({ login: $("login").value, password: $("password").value });
        if (!response.ok) {
            message.textContent = await api.readError(response);
            return;
        }

        const data = await response.json();
        state.currentUser = data.user;
        form.reset();
        message.textContent = "";
        await enterApp();
    } catch (error) {
        console.error("Login error:", error);
        message.textContent = "Something went wrong.";
    }
}

// logout keeps the session visible on failure and leaves the app only after server confirmation.
async function logout() {
    closeAccountMenu();
    try {
        const response = await api.logout();
        if (!response.ok) {
            showToast("Logout failed. Please try again.");
            return;
        }
        leaveApp();
    } catch (error) {
        console.error("Logout error:", error);
        showToast("Logout failed. Please try again.");
    }
}

// restoreSession applies the session transition and lifecycle callbacks, returning the restored user or null.
export async function restoreSession() {
    try {
        const response = await api.checkAuth();
        if (!response.ok) {
            leaveApp();
            return null;
        }
        const user = await response.json();
        state.currentUser = { id: user.id, nickname: user.nickname };
        await enterApp();
        return state.currentUser;
    } catch (error) {
        console.error("Session check error:", error);
        leaveApp();
        return null;
    }
}

// toggleAccountMenu keeps the menu visibility and accessibility state synchronized.
function toggleAccountMenu() {
    const menu = $("account-menu");
    menu.hidden = !menu.hidden;
    $("account-button").setAttribute("aria-expanded", String(!menu.hidden));
}

// closeAccountMenu collapses the account actions for logout, outside clicks, and Escape.
export function closeAccountMenu() {
    $("account-menu").hidden = true;
    $("account-button").setAttribute("aria-expanded", "false");
}

// initAuth binds persistent forms once and accepts main's authenticated and logged-out lifecycle callbacks.
export function initAuth(callbacks = {}) {
    onAuthenticated = callbacks.onAuthenticated || (async () => {});
    onLoggedOut = callbacks.onLoggedOut || (() => {});
    if (initialized) return;
    initialized = true;

    $("login-form").addEventListener("submit", login);
    $("register-form").addEventListener("submit", register);
    document.querySelectorAll("[data-auth]").forEach(button => {
        button.addEventListener("click", () => {
            $("login-message").textContent = "";
            $("register-message").textContent = "";
            showAuth(button.dataset.auth);
        });
    });
    $("account-button").addEventListener("click", event => {
        event.stopPropagation();
        toggleAccountMenu();
    });
    $("logout-button").addEventListener("click", logout);
    $("nav-logout-button").addEventListener("click", logout);
    document.addEventListener("click", event => {
        if (!$("account-menu").hidden && !event.target.closest(".account")) closeAccountMenu();
    });
    document.addEventListener("keydown", event => {
        if (event.key !== "Escape" || $("account-menu").hidden) return;
        // Chat's Escape handler skips handled events so only the account menu closes on this press.
        event.preventDefault();
        closeAccountMenu();
    });
}
