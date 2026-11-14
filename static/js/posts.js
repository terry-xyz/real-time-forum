import * as api from "./api.js";
import { $, el, icon, avatar, timeElement, autoGrow } from "./dom.js";
import { state } from "./state.js";
import { loadComments } from "./comments.js";

let initialized = false;
let showView = () => {};
let renderProfile = () => {};

// loadCategories refreshes the category names used by the local tabs and sidebar.
export async function loadCategories() {
    try {
        const response = await api.getCategories();
        if (!response.ok) return;
        state.categories = (await response.json()) || [];
    } catch (error) {
        console.error("Category loading error:", error);
    }
    renderCategories();
}

// loadPosts fetches the full feed so tabs, search, counts, and profile share the same posts.
export async function loadPosts() {
    if (!state.currentUser) return;

    const list = $("post-list");
    try {
        const response = await api.getPosts();
        if (!response.ok) {
            list.replaceChildren(el("p", { className: "feed-empty", text: "Failed to load posts." }));
            return;
        }
        state.allPosts = (await response.json()) || [];
    } catch (error) {
        console.error("Post loading error:", error);
        list.replaceChildren(el("p", { className: "feed-empty", text: "Failed to load posts." }));
        return;
    }

    renderFeed();
    renderCategories();
    if (state.currentView === "profile") renderProfile();
}

// selectCategory filters the loaded posts without issuing a request when switching tabs.
function selectCategory(name) {
    state.activeCategory = name;
    if (state.currentView !== "home") showView("home");
    renderFeed();
    renderCategories();
}

// renderCategories uses the complete feed for counts even while category or search filters are active.
function renderCategories() {
    const tabNames = ["", ...state.categories.map(category => category.name)];
    $("category-tabs").replaceChildren(...tabNames.map(name => {
        const tab = el("button", {
            className: `tab${name === state.activeCategory ? " active" : ""}`,
            attrs: { type: "button", role: "tab", "aria-selected": String(name === state.activeCategory) }
        }, [el("span", { text: name || "All posts" })]);
        tab.addEventListener("click", () => selectCategory(name));
        return tab;
    }));

    const side = $("side-category-list");
    if (state.categories.length === 0) {
        side.replaceChildren(el("p", { className: "muted category-empty", text: "No categories yet." }));
        return;
    }
    side.replaceChildren(...state.categories.map(category => {
        const count = state.allPosts.filter(post => (post.categories || []).includes(category.name)).length;
        const row = el("button", {
            className: `trend${category.name === state.activeCategory ? " active" : ""}`,
            attrs: { type: "button" }
        }, [
            el("span", { text: "Category" }),
            el("strong", { text: category.name }),
            el("span", { text: `${count} ${count === 1 ? "post" : "posts"}` })
        ]);
        row.addEventListener("click", () => selectCategory(category.name === state.activeCategory ? "" : category.name));
        return row;
    }));
}

// matchesSearch checks the UI's local query against post text, author, and category names.
function matchesSearch(post) {
    if (!state.searchQuery) return true;
    const haystack = `${post.title} ${post.content} ${post.author} ${(post.categories || []).join(" ")}`.toLowerCase();
    return haystack.includes(state.searchQuery);
}

// inActiveCategory includes every post when the All posts tab is selected.
function inActiveCategory(post) {
    return !state.activeCategory || (post.categories || []).includes(state.activeCategory);
}

// renderFeed applies both local filters and preserves the distinct search, category, and empty-feed feedback.
function renderFeed() {
    const list = $("post-list");
    const posts = state.allPosts.filter(post => inActiveCategory(post) && matchesSearch(post));
    if (posts.length === 0) {
        const text = state.searchQuery ? "No posts match your search." :
            state.activeCategory ? "No posts in this category yet." : "No posts yet. Write the first one!";
        list.replaceChildren(el("p", { className: "feed-empty", text }));
        return;
    }
    list.replaceChildren(...posts.map(createPostCard));
}

// createPostCard builds the shared feed/profile card with category links and expandable comments.
export function createPostCard(post) {
    const commentsSection = el("section", { className: "comments" });
    commentsSection.hidden = true;

    const replyButton = el("button", {
        className: "post-action",
        attrs: { type: "button", "aria-expanded": "false" }
    }, [icon("reply"), el("span", { text: "Comments" })]);

    replyButton.addEventListener("click", async () => {
        commentsSection.hidden = !commentsSection.hidden;
        replyButton.setAttribute("aria-expanded", String(!commentsSection.hidden));
        if (!commentsSection.hidden) {
            await loadComments(post, commentsSection);
            commentsSection.querySelector("textarea")?.focus();
        }
    });

    const tags = el("div", { className: "post-tags" }, (post.categories || []).map(name => {
        const tag = el("button", { className: "tag", text: `#${name}`, attrs: { type: "button" } });
        tag.addEventListener("click", () => selectCategory(name));
        return tag;
    }));

    const card = el("article", { className: "post" }, [
        avatar(post.author),
        el("div", { className: "post-body" }, [
            el("div", { className: "post-meta" }, [
                el("strong", { text: post.author }),
                el("span", { className: "handle", text: `@${post.author}` }),
                el("span", { text: "·" }),
                timeElement(post.created_at)
            ]),
            el("h2", { className: "post-title", text: post.title }),
            el("p", { className: "post-content", text: post.content }),
            tags,
            el("div", { className: "post-actions" }, [replyButton]),
            commentsSection
        ])
    ]);

    card.addEventListener("click", event => {
        // Card-wide expansion must not interrupt embedded controls or text selection.
        if (event.target.closest("button, a, input, textarea, form, .comments")) return;
        if (window.getSelection()?.toString()) return;
        replyButton.click();
    });
    return card;
}

// createPost submits the composer using the existing payload and refreshes posts and category counts.
async function createPost(event) {
    event.preventDefault();

    const form = event.currentTarget; // Capture before awaiting because currentTarget is cleared after dispatch.
    const message = $("post-message");
    const submit = $("post-submit");
    const categoryNames = $("post-categories").value
        .split(",")
        .map(category => category.trim())
        .filter(category => category !== "");

    if (categoryNames.length === 0) {
        message.textContent = "Add at least one category.";
        return;
    }

    submit.disabled = true;
    try {
        const response = await api.createPost({
            title: $("post-title").value,
            content: $("post-content").value,
            categories: categoryNames
        });

        if (!response.ok) {
            message.textContent = await api.readError(response);
            return;
        }

        form.reset();
        autoGrow($("post-content"));
        message.textContent = "";
        await Promise.all([loadCategories(), loadPosts()]);
    } catch (error) {
        console.error("Post creation error:", error);
        message.textContent = "Something went wrong.";
    } finally {
        updateComposerButton();
    }
}

// updateComposerButton enables posting only when each composer field contains non-whitespace text.
function updateComposerButton() {
    const ready = $("post-title").value.trim() && $("post-content").value.trim() && $("post-categories").value.trim();
    $("post-submit").disabled = !ready;
}

// initPosts binds persistent composer/search handlers once and accepts navigation callbacks to avoid import cycles.
export function initPosts(callbacks = {}) {
    if (initialized) return;
    showView = callbacks.showView || (() => {});
    renderProfile = callbacks.renderProfile || (() => {});
    initialized = true;

    $("post-form").addEventListener("submit", createPost);
    ["post-title", "post-content", "post-categories"].forEach(id => {
        $(id).addEventListener("input", updateComposerButton);
    });
    $("post-content").addEventListener("input", event => autoGrow(event.currentTarget));
    $("search-input").addEventListener("input", event => {
        state.searchQuery = event.currentTarget.value.trim().toLowerCase();
        if (state.currentView !== "home") showView("home");
        renderFeed();
    });
}
