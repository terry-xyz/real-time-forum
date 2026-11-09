import { $, el, paintAvatar } from "./dom.js";
import { state } from "./state.js";
import { createPostCard, loadPosts } from "./posts.js";

let initialized = false;

// showView switches the home/profile panels and keeps navigation highlighting and scroll position aligned.
export function showView(view) {
    state.currentView = view;
    $("home-view").hidden = view !== "home";
    $("profile-view").hidden = view !== "profile";
    document.querySelectorAll(".nav-item[data-view]").forEach(item => {
        const active = item.dataset.view === view;
        item.classList.toggle("active", active);
        if (active) item.setAttribute("aria-current", "page");
        else item.removeAttribute("aria-current");
    });
    if (view === "profile") renderProfile();
    window.scrollTo({ top: 0 });
}

// renderProfile derives the signed-in user's cards and category totals from the full shared feed.
export function renderProfile() {
    if (!state.currentUser) return;
    const posts = state.allPosts.filter(post => post.user_id === state.currentUser.id);

    $("profile-title").textContent = state.currentUser.nickname;
    $("profile-count").textContent = `${posts.length} ${posts.length === 1 ? "post" : "posts"}`;
    $("profile-name").textContent = state.currentUser.nickname;
    $("profile-handle").textContent = `@${state.currentUser.nickname}`;
    paintAvatar($("profile-avatar"), state.currentUser.nickname);

    const categoryCount = new Set(posts.flatMap(post => post.categories || [])).size;
    $("profile-facts").replaceChildren(
        el("li", { text: `${posts.length} ${posts.length === 1 ? "post" : "posts"}` }),
        el("li", { text: `${categoryCount} ${categoryCount === 1 ? "category" : "categories"}` })
    );

    $("profile-post-list").replaceChildren(...(posts.length
        ? posts.map(createPostCard)
        : [el("p", { className: "feed-empty", text: "You haven't posted yet." })]));
}

// initProfile binds home/profile navigation once, including the home refresh and composer shortcut.
export function initProfile() {
    if (initialized) return;
    initialized = true;

    document.querySelectorAll("[data-view]").forEach(button => {
        button.addEventListener("click", () => {
            if (button.dataset.view === "home" && state.currentView === "home") loadPosts();
            showView(button.dataset.view);
        });
    });
    $("sidebar-post-button").addEventListener("click", () => {
        showView("home");
        $("post-title").focus();
    });
}
