"use strict";

/* ==========================================================================
   State
   ========================================================================== */

const MESSAGE_PAGE_SIZE = 10;
const USER_REFRESH_MS = 20000;
const OLDER_MESSAGES_THROTTLE_MS = 300;
const TYPING_THROTTLE_MS = 1000;

let currentUser = null;
let currentView = "home";
let allPosts = [];          // every post; the feed, tabs, counts and profile all filter this
let categories = [];
let activeCategory = "";
let searchQuery = "";

let chatUsers = [];
let selectedChatUser = null;
const unreadUserIds = new Set();

let chatSocket = null;
let typingStatusTimeout = null;
let toastTimeout = null;
let userRefreshTimer = null;
let hasOlderMessages = false;
let loadingOlderMessages = false;
let lastOlderMessagesLoadAt = 0;
let lastTypingSentAt = 0;

const $ = id => document.getElementById(id);

/* ==========================================================================
   Small DOM helpers
   ========================================================================== */

// Builds an element; text always goes through textContent, never innerHTML.
function el(tag, options = {}, children = []) {
    const node = document.createElement(tag);
    if (options.className) node.className = options.className;
    if (options.text !== undefined) node.textContent = options.text;
    if (options.attrs) {
        Object.entries(options.attrs).forEach(([name, value]) => node.setAttribute(name, value));
    }
    children.forEach(child => child && node.append(child));
    return node;
}

function icon(name) {
    const svg = document.createElementNS("http://www.w3.org/2000/svg", "svg");
    svg.setAttribute("aria-hidden", "true");
    const use = document.createElementNS("http://www.w3.org/2000/svg", "use");
    use.setAttribute("href", `#icon-${name}`);
    svg.append(use);
    return svg;
}

// Same name always gets the same color, so people are recognizable at a glance.
const AVATAR_COLORS = ["#1d9bf0", "#7856ff", "#f91880", "#00ba7c", "#ff7a00", "#536471", "#e0245e", "#17bf63"];

function paintAvatar(node, name) {
    const label = (name || "?").trim();
    let hash = 0;
    for (const char of label) hash = (hash * 31 + char.codePointAt(0)) >>> 0;
    node.textContent = [...label][0] || "?";
    node.style.setProperty("--avatar-color", AVATAR_COLORS[hash % AVATAR_COLORS.length]);
    node.setAttribute("aria-hidden", "true");
    return node;
}

function avatar(name, extraClass = "") {
    return paintAvatar(el("span", { className: `avatar ${extraClass}`.trim() }), name);
}

function relativeTime(value) {
    const date = new Date(value);
    const seconds = Math.round((Date.now() - date.getTime()) / 1000);
    if (seconds < 60) return "now";
    if (seconds < 3600) return `${Math.floor(seconds / 60)}m`;
    if (seconds < 86400) return `${Math.floor(seconds / 3600)}h`;
    const sameYear = date.getFullYear() === new Date().getFullYear();
    return date.toLocaleDateString(undefined, sameYear
        ? { month: "short", day: "numeric" }
        : { month: "short", day: "numeric", year: "numeric" });
}

function timeElement(value) {
    const time = el("time", { text: relativeTime(value) });
    time.dateTime = value;
    time.title = new Date(value).toLocaleString();
    return time;
}

// Grows a textarea with its content, like X's composer.
function autoGrow(textarea) {
    textarea.style.height = "auto";
    textarea.style.height = `${textarea.scrollHeight}px`;
}

async function readError(response) {
    const text = (await response.text()).trim();
    return text || "Something went wrong.";
}

function showToast(text) {
    const toast = $("toast");
    toast.textContent = text;
    toast.hidden = false;
    clearTimeout(toastTimeout);
    toastTimeout = setTimeout(() => { toast.hidden = true; }, 3500);
}

/* ==========================================================================
   Screens: signed out / signed in
   ========================================================================== */

function showAuth(mode = "login") {
    $("app-view").hidden = true;
    $("chat-drawer").hidden = true;
    $("chat-fab").hidden = true;
    $("auth-view").hidden = false;
    $("login-form").hidden = mode !== "login";
    $("register-form").hidden = mode !== "register";
    $("auth-heading").textContent = mode === "login" ? "Sign in to Real-Time Forum" : "Join today.";
    $(mode === "login" ? "login" : "first-name").focus();
}

async function enterApp() {
    $("auth-view").hidden = true;
    $("app-view").hidden = false;
    $("chat-fab").hidden = false;

    $("account-name").textContent = currentUser.nickname;
    $("account-handle").textContent = `@${currentUser.nickname}`;
    $("logout-handle").textContent = `@${currentUser.nickname}`;
    paintAvatar($("account-avatar"), currentUser.nickname);
    paintAvatar($("composer-avatar"), currentUser.nickname);

    connectChatSocket();
    startUserRefresh();
    showView("home");
    await Promise.all([loadCategories(), loadPosts(), loadChatUsers()]);
}

function leaveApp() {
    currentUser = null;
    selectedChatUser = null;
    unreadUserIds.clear();
    closeChatSocket();
    stopUserRefresh();
    showAuth("login");
}

function showView(view) {
    currentView = view;
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

/* ==========================================================================
   Posts
   ========================================================================== */

async function loadCategories() {
    try {
        const response = await fetch("/api/categories");
        if (!response.ok) return;
        categories = (await response.json()) || [];
    } catch (error) {
        console.error("Category loading error:", error);
    }
    renderCategories();
}

async function loadPosts() {
    if (!currentUser) return;

    const list = $("post-list");
    try {
        const allResponse = await fetch("/api/posts");
        if (!allResponse.ok) {
            list.replaceChildren(el("p", { className: "feed-empty", text: "Failed to load posts." }));
            return;
        }
        allPosts = (await allResponse.json()) || [];
    } catch (error) {
        console.error("Post loading error:", error);
        list.replaceChildren(el("p", { className: "feed-empty", text: "Failed to load posts." }));
        return;
    }

    renderFeed();
    renderCategories();
    if (currentView === "profile") renderProfile();
}

// Category tabs filter the posts already loaded, so switching tabs costs no request.
function selectCategory(name) {
    activeCategory = name;
    if (currentView !== "home") showView("home");
    renderFeed();
    renderCategories();
}

function renderCategories() {
    // Tabs across the top of the timeline (replaces the old <select>)
    const tabs = $("category-tabs");
    const tabNames = ["", ...categories.map(category => category.name)];
    tabs.replaceChildren(...tabNames.map(name => {
        const tab = el("button", {
            className: `tab${name === activeCategory ? " active" : ""}`,
            attrs: { type: "button", role: "tab", "aria-selected": String(name === activeCategory) }
        }, [el("span", { text: name || "All posts" })]);
        tab.addEventListener("click", () => selectCategory(name));
        return tab;
    }));

    // "Trending"-style card in the right column, with post counts
    const side = $("side-category-list");
    if (categories.length === 0) {
        side.replaceChildren(el("p", { className: "muted", text: "No categories yet.", attrs: { style: "margin:0;padding:4px 16px 8px" } }));
        return;
    }
    side.replaceChildren(...categories.map(category => {
        const count = allPosts.filter(post => (post.categories || []).includes(category.name)).length;
        const row = el("button", {
            className: `trend${category.name === activeCategory ? " active" : ""}`,
            attrs: { type: "button" }
        }, [
            el("span", { text: "Category" }),
            el("strong", { text: category.name }),
            el("span", { text: `${count} ${count === 1 ? "post" : "posts"}` })
        ]);
        row.addEventListener("click", () => selectCategory(category.name === activeCategory ? "" : category.name));
        return row;
    }));
}

function matchesSearch(post) {
    if (!searchQuery) return true;
    const haystack = `${post.title} ${post.content} ${post.author} ${(post.categories || []).join(" ")}`.toLowerCase();
    return haystack.includes(searchQuery);
}

function inActiveCategory(post) {
    return !activeCategory || (post.categories || []).includes(activeCategory);
}

function renderFeed() {
    const list = $("post-list");
    const posts = allPosts.filter(post => inActiveCategory(post) && matchesSearch(post));
    if (posts.length === 0) {
        const text = searchQuery ? "No posts match your search." :
            activeCategory ? "No posts in this category yet." : "No posts yet. Write the first one!";
        list.replaceChildren(el("p", { className: "feed-empty", text }));
        return;
    }
    list.replaceChildren(...posts.map(createPostCard));
}

function createPostCard(post) {
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

    // Clicking anywhere on the post opens its comments, like opening a post on X.
    card.addEventListener("click", event => {
        if (event.target.closest("button, a, input, textarea, form, .comments")) return;
        if (window.getSelection()?.toString()) return; // let people select text
        replyButton.click();
    });
    return card;
}

async function createPost(event) {
    event.preventDefault();

    const form = event.currentTarget; // currentTarget is null after an await
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
        const response = await fetch("/api/posts", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({
                title: $("post-title").value,
                content: $("post-content").value,
                categories: categoryNames
            })
        });

        if (!response.ok) {
            message.textContent = await readError(response);
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

function updateComposerButton() {
    const ready = $("post-title").value.trim() && $("post-content").value.trim() && $("post-categories").value.trim();
    $("post-submit").disabled = !ready;
}

/* ==========================================================================
   Comments
   ========================================================================== */

async function loadComments(post, section) {
    let list = section.querySelector(".comment-list");
    if (!list) {
        list = el("div", { className: "comment-list" });
        const textarea = el("textarea", { attrs: { placeholder: "Post your reply", rows: "1", required: "" } });
        const status = el("p", { className: "form-status comment-message", attrs: { role: "status" } });
        const form = el("form", { className: "comment-form" }, [
            avatar(currentUser.nickname, "avatar-sm"),
            textarea,
            el("button", { className: "btn btn-dark", text: "Reply", attrs: { type: "submit" } })
        ]);
        textarea.addEventListener("input", () => autoGrow(textarea));
        textarea.addEventListener("keydown", submitOnEnter);
        form.addEventListener("submit", event => addComment(event, post, section));
        section.append(list, form, status);
    }

    list.replaceChildren(el("p", { className: "comment-empty", text: "Loading comments..." }));
    try {
        const response = await fetch(`/api/comments?post_id=${encodeURIComponent(post.id)}`);
        if (!response.ok) {
            list.replaceChildren(el("p", { className: "comment-empty", text: "Failed to load comments." }));
            return;
        }

        const comments = (await response.json()) || [];
        if (comments.length === 0) {
            list.replaceChildren(el("p", { className: "comment-empty", text: "No comments yet. Start the conversation." }));
            return;
        }

        list.replaceChildren(...comments.map(comment => el("div", { className: "comment" }, [
            avatar(comment.author, "avatar-sm"),
            el("div", { className: "comment-body" }, [
                el("div", { className: "post-meta" }, [
                    el("strong", { text: comment.author }),
                    el("span", { text: "·" }),
                    timeElement(comment.created_at)
                ]),
                el("p", { text: comment.content })
            ])
        ])));
    } catch (error) {
        console.error("Comment loading error:", error);
        list.replaceChildren(el("p", { className: "comment-empty", text: "Failed to load comments." }));
    }
}

async function addComment(event, post, section) {
    event.preventDefault();

    const form = event.currentTarget;
    const textarea = form.querySelector("textarea");
    const status = section.querySelector(".comment-message");

    try {
        const response = await fetch("/api/comments", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ post_id: post.id, content: textarea.value })
        });

        if (!response.ok) {
            status.textContent = await readError(response);
            return;
        }

        form.reset();
        autoGrow(textarea);
        status.textContent = "";
        await loadComments(post, section);
    } catch (error) {
        console.error("Comment submission error:", error);
        status.textContent = "Something went wrong.";
    }
}

/* ==========================================================================
   Profile
   ========================================================================== */

function renderProfile() {
    if (!currentUser) return;
    const posts = allPosts.filter(post => post.user_id === currentUser.id);

    $("profile-title").textContent = currentUser.nickname;
    $("profile-count").textContent = `${posts.length} ${posts.length === 1 ? "post" : "posts"}`;
    $("profile-name").textContent = currentUser.nickname;
    $("profile-handle").textContent = `@${currentUser.nickname}`;
    paintAvatar($("profile-avatar"), currentUser.nickname);

    const categoryCount = new Set(posts.flatMap(post => post.categories || [])).size;
    $("profile-facts").replaceChildren(
        el("li", { text: `${posts.length} ${posts.length === 1 ? "post" : "posts"}` }),
        el("li", { text: `${categoryCount} ${categoryCount === 1 ? "category" : "categories"}` })
    );

    const list = $("profile-post-list");
    list.replaceChildren(...(posts.length
        ? posts.map(createPostCard)
        : [el("p", { className: "feed-empty", text: "You haven't posted yet." })]));
}

/* ==========================================================================
   Chat: users, drawer, messages
   ========================================================================== */

function startUserRefresh() {
    stopUserRefresh();
    // Presence isn't pushed over the socket, so refresh the list now and then.
    userRefreshTimer = setInterval(() => {
        if (document.visibilityState === "visible") loadChatUsers();
    }, USER_REFRESH_MS);
}

function stopUserRefresh() {
    clearInterval(userRefreshTimer);
    userRefreshTimer = null;
}

async function loadChatUsers() {
    try {
        const response = await fetch("/api/chat/users");
        if (!response.ok) {
            renderUserLists("Failed to load users.");
            return;
        }
        chatUsers = (await response.json()) || [];
    } catch (error) {
        console.error("Chat user loading error:", error);
        renderUserLists("Failed to load users.");
        return;
    }

    if (selectedChatUser) {
        const stillExists = chatUsers.find(user => user.id === selectedChatUser.id);
        if (stillExists) {
            selectedChatUser = stillExists;
            updateChatHeader();
        } else {
            closeConversation();
        }
    }
    renderUserLists();
}

function renderUserLists(errorText = "") {
    [$("side-user-list"), $("chat-user-list")].forEach(container => {
        if (errorText || chatUsers.length === 0) {
            container.replaceChildren(el("p", { text: errorText || "No other users yet." }));
            return;
        }
        container.replaceChildren(...chatUsers.map(createUserRow));
    });

    const onlineCount = chatUsers.filter(user => user.online).length;
    const onlineText = errorText || chatUsers.length === 0 ? "" : `${onlineCount} online`;
    $("online-count").textContent = onlineText;
    if (!selectedChatUser) $("chat-subtitle").textContent = onlineText;

    const hasUnread = unreadUserIds.size > 0;
    $("chat-badge").hidden = !hasUnread;
    $("chat-fab-dot").hidden = !hasUnread;
}

function createUserRow(user) {
    const face = avatar(user.nickname);
    const presence = el("span", {
        className: `presence-indicator ${user.online ? "online" : "offline"}`,
        attrs: { "aria-label": user.online ? "Online" : "Offline", role: "img" }
    });
    face.removeAttribute("aria-hidden");
    face.setAttribute("role", "presentation");
    face.append(presence);

    const row = el("button", {
        className: `user-row${selectedChatUser?.id === user.id ? " selected" : ""}`,
        attrs: { type: "button", "data-user-id": user.id }
    }, [
        face,
        el("span", { className: "user-row-text" }, [
            el("strong", { text: user.nickname }),
            el("span", { text: user.online ? "Online" : "Offline" })
        ]),
        unreadUserIds.has(user.id) ? el("span", { className: "unread-dot", attrs: { "aria-label": "Unread messages", role: "img" } }) : null
    ]);
    row.addEventListener("click", () => openConversation(user));
    return row;
}

function openChatDrawer() {
    $("chat-drawer").hidden = false;
    $("chat-fab").hidden = true;
    if (!selectedChatUser) showUserListInDrawer();
}

function closeChatDrawer() {
    closeConversation();
    $("chat-drawer").hidden = true;
    $("chat-fab").hidden = false;
}

function showUserListInDrawer() {
    $("chat-title").textContent = "Chats";
    $("chat-subtitle").textContent = $("online-count").textContent;
    $("chat-back").hidden = true;
    $("chat-user-list").hidden = false;
    $("chat-conversation").hidden = true;
}

function closeConversation() {
    selectedChatUser = null;
    clearTimeout(typingStatusTimeout);
    $("typing-status").textContent = "";
    showUserListInDrawer();
    renderUserLists();
}

function updateChatHeader() {
    if (!selectedChatUser) return;
    $("chat-title").textContent = selectedChatUser.nickname;
    $("chat-subtitle").textContent = selectedChatUser.online ? "Online" : "Offline";
}

async function openConversation(user) {
    selectedChatUser = user;
    unreadUserIds.delete(user.id);
    hasOlderMessages = false;
    clearTimeout(typingStatusTimeout);

    $("chat-drawer").hidden = false;
    $("chat-fab").hidden = true;
    $("chat-back").hidden = false;
    $("chat-user-list").hidden = true;
    $("chat-conversation").hidden = false;
    $("typing-status").textContent = "";
    $("message-status").textContent = "";
    updateChatHeader();
    renderUserLists();

    const list = $("chat-message-list");
    list.replaceChildren(el("p", { text: "Loading messages..." }));

    try {
        const response = await fetch(`/api/messages?with=${encodeURIComponent(user.id)}&limit=${MESSAGE_PAGE_SIZE}`);
        if (selectedChatUser?.id !== user.id) return;

        if (!response.ok) {
            list.replaceChildren(el("p", { text: "Failed to load messages." }));
            return;
        }

        const messages = (await response.json()) || [];
        if (messages.length === 0) {
            list.replaceChildren(el("p", { className: "chat-empty", text: `No messages yet. Say hi to ${user.nickname}!` }));
        } else {
            list.replaceChildren(...messages.map(createChatMessage));
            hasOlderMessages = messages.length === MESSAGE_PAGE_SIZE;
            updateOlderHint();
            list.scrollTop = list.scrollHeight;
        }
        $("message-content").focus();
    } catch (error) {
        console.error("Conversation loading error:", error);
        list.replaceChildren(el("p", { text: "Failed to load messages." }));
    }
}

function createChatMessage(message) {
    const mine = message.sender_id === currentUser?.id;
    const item = el("article", { className: `chat-message${mine ? " mine" : ""}` }, [
        el("p", { className: "chat-bubble", text: message.content }),
        el("div", { className: "chat-message-meta" }, [
            document.createTextNode(`${message.sender_name} · `),
            (() => {
                const time = el("time", { text: new Date(message.created_at).toLocaleString() });
                time.dateTime = message.created_at;
                return time;
            })()
        ])
    ]);
    item.dataset.messageId = message.id;
    return item;
}

function appendChatMessage(message) {
    const list = $("chat-message-list");
    list.querySelector(".chat-empty")?.remove();
    const wasAtBottom = list.scrollHeight - list.scrollTop - list.clientHeight < 40;
    list.append(createChatMessage(message));
    if (wasAtBottom || message.sender_id === currentUser?.id) list.scrollTop = list.scrollHeight;
}

// Older messages load when the list is scrolled to the top, so the list must be
// able to scroll. On a tall screen 10 short messages might all fit, so while
// older messages exist this hint at the top grows just enough to make it overflow.
function updateOlderHint() {
    const list = $("chat-message-list");
    let hint = list.querySelector(".older-hint");
    if (!hasOlderMessages || !selectedChatUser) {
        hint?.remove();
        return;
    }
    if (!hint) {
        hint = el("p", { className: "older-hint", text: "Scroll up for older messages" });
        list.prepend(hint);
    }
    hint.style.minHeight = "";
    // scrollHeight never drops below clientHeight, so measure the real content
    // height from the first and last children (the list is position: relative).
    const last = list.lastElementChild;
    const contentHeight = last.offsetTop + last.offsetHeight - hint.offsetTop;
    const style = getComputedStyle(list);
    const innerHeight = list.clientHeight - parseFloat(style.paddingTop) - parseFloat(style.paddingBottom);
    const spare = innerHeight - contentHeight;
    if (spare >= 0) hint.style.minHeight = `${hint.offsetHeight + spare + 40}px`;
}

// Throttle: the scroll event fires many times a second, but older messages
// load at most once per OLDER_MESSAGES_THROTTLE_MS, and never while a load is running.
function handleChatScroll(event) {
    const now = Date.now();
    if (
        event.currentTarget.scrollTop > 0 ||
        !hasOlderMessages ||
        loadingOlderMessages ||
        now - lastOlderMessagesLoadAt < OLDER_MESSAGES_THROTTLE_MS
    ) {
        return;
    }

    lastOlderMessagesLoadAt = now;
    loadOlderMessages();
}

async function loadOlderMessages() {
    const user = selectedChatUser;
    const list = $("chat-message-list");
    const oldestMessage = list.querySelector(".chat-message");
    if (!user || !oldestMessage) return;

    loadingOlderMessages = true;
    try {
        const response = await fetch(
            `/api/messages?with=${encodeURIComponent(user.id)}&limit=${MESSAGE_PAGE_SIZE}&before_id=${encodeURIComponent(oldestMessage.dataset.messageId)}`
        );
        if (selectedChatUser?.id !== user.id) return;

        if (!response.ok) {
            $("message-status").textContent = await readError(response);
            return;
        }

        const messages = (await response.json()) || [];
        if (messages.length === 0) {
            hasOlderMessages = false;
            updateOlderHint();
            return;
        }

        // Keep the view anchored on the message the user was reading.
        const previousHeight = list.scrollHeight;
        oldestMessage.before(...messages.map(createChatMessage));
        hasOlderMessages = messages.length === MESSAGE_PAGE_SIZE;
        updateOlderHint();
        list.scrollTop += list.scrollHeight - previousHeight;
    } catch (error) {
        console.error("Older message loading error:", error);
        $("message-status").textContent = "Failed to load older messages.";
    } finally {
        loadingOlderMessages = false;
    }
}

async function sendMessage(event) {
    event.preventDefault();

    const status = $("message-status");
    const textarea = $("message-content");
    if (!selectedChatUser) {
        status.textContent = "Choose a user first.";
        return;
    }
    if (!textarea.value.trim()) return;

    try {
        const response = await fetch("/api/messages", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ receiver_id: selectedChatUser.id, content: textarea.value })
        });

        if (!response.ok) {
            status.textContent = await readError(response);
            return;
        }

        const message = await response.json();
        textarea.value = "";
        autoGrow(textarea);
        status.textContent = "";
        appendChatMessage(message);
        await loadChatUsers(); // the list is ordered by latest message
    } catch (error) {
        console.error("Message sending error:", error);
        status.textContent = "Something went wrong.";
    }
}

// Throttled: at most one "typing" event per second, however fast the user types.
// The receiver shows the indicator for 1.5s, so it stays on while typing continues.
function updateTypingStatus() {
    const recipient = selectedChatUser;
    if (!recipient || chatSocket?.readyState !== WebSocket.OPEN) return;
    if (!$("message-content").value.trim()) return;

    const now = Date.now();
    if (now - lastTypingSentAt < TYPING_THROTTLE_MS) return;
    lastTypingSentAt = now;

    chatSocket.send(JSON.stringify({ type: "typing", receiver_id: recipient.id }));
}

// Enter sends, Shift+Enter adds a new line.
function submitOnEnter(event) {
    if (event.key === "Enter" && !event.shiftKey && !event.isComposing) {
        event.preventDefault();
        event.currentTarget.form.requestSubmit();
    }
}

/* ==========================================================================
   WebSocket
   ========================================================================== */

function connectChatSocket() {
    if (chatSocket?.readyState === WebSocket.OPEN || chatSocket?.readyState === WebSocket.CONNECTING) {
        return;
    }

    const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
    const socket = new WebSocket(`${protocol}//${window.location.host}/api/ws`);
    chatSocket = socket;
    socket.addEventListener("close", () => {
        if (chatSocket === socket) chatSocket = null;
    });
    socket.addEventListener("message", handleChatSocketMessage);
    socket.addEventListener("error", error => {
        console.error("Chat connection error:", error);
    });
}

function closeChatSocket() {
    chatSocket?.close();
    chatSocket = null;
}

function handleChatSocketMessage(event) {
    let data;
    try {
        data = JSON.parse(event.data);
    } catch (error) {
        console.error("Invalid chat event:", error);
        return;
    }

    if (data.type === "typing") {
        if (selectedChatUser?.id !== data.sender_id) return;
        const typingStatus = $("typing-status");
        typingStatus.textContent = `${selectedChatUser.nickname} is typing...`;
        clearTimeout(typingStatusTimeout);
        typingStatusTimeout = setTimeout(() => { typingStatus.textContent = ""; }, 1500);
        return;
    }
    if (data.type !== "message") return;

    const message = data.message;
    const conversationOpen = selectedChatUser?.id === message.sender_id && !$("chat-drawer").hidden;
    if (conversationOpen) {
        clearTimeout(typingStatusTimeout);
        $("typing-status").textContent = "";
        appendChatMessage(message);
    } else {
        unreadUserIds.add(message.sender_id);
        showToast(`New message from ${message.sender_name}`);
    }

    loadChatUsers();
}

/* ==========================================================================
   Auth
   ========================================================================== */

async function register(event) {
    event.preventDefault();

    const form = event.currentTarget; // currentTarget is null after an await
    const message = $("register-message");
    message.classList.remove("success");

    try {
        const response = await fetch("/api/register", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({
                nickname: $("nickname").value,
                age: Number($("age").value),
                gender: $("gender").value,
                first_name: $("first-name").value,
                last_name: $("last-name").value,
                email: $("email").value,
                password: $("register-password").value
            })
        });

        if (!response.ok) {
            message.textContent = await readError(response);
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

async function login(event) {
    event.preventDefault();

    const form = event.currentTarget; // currentTarget is null after an await
    const message = $("login-message");
    message.classList.remove("success");

    try {
        const response = await fetch("/api/login", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ login: $("login").value, password: $("password").value })
        });

        if (!response.ok) {
            message.textContent = await readError(response);
            return;
        }

        const data = await response.json();
        currentUser = data.user;
        form.reset();
        message.textContent = "";
        await enterApp();
    } catch (error) {
        console.error("Login error:", error);
        message.textContent = "Something went wrong.";
    }
}

async function logout() {
    closeAccountMenu();
    try {
        const response = await fetch("/api/logout", { method: "POST" });
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

async function restoreSession() {
    try {
        const response = await fetch("/api/check-auth");
        if (!response.ok) {
            leaveApp();
            return;
        }
        const user = await response.json();
        currentUser = { id: user.id, nickname: user.nickname };
        await enterApp();
    } catch (error) {
        console.error("Session check error:", error);
        leaveApp();
    }
}

/* ==========================================================================
   Account menu
   ========================================================================== */

function toggleAccountMenu() {
    const menu = $("account-menu");
    menu.hidden = !menu.hidden;
    $("account-button").setAttribute("aria-expanded", String(!menu.hidden));
}

function closeAccountMenu() {
    $("account-menu").hidden = true;
    $("account-button").setAttribute("aria-expanded", "false");
}

/* ==========================================================================
   Theme (light / X "Lights out" dark)
   ========================================================================== */

const systemDark = window.matchMedia("(prefers-color-scheme: dark)");

function savedTheme() {
    try { return localStorage.getItem("theme"); } catch { return null; }
}

function applyTheme(theme) {
    const dark = theme === "dark";
    document.documentElement.dataset.theme = theme;
    document.querySelector('meta[name="theme-color"]').content = dark ? "#000000" : "#ffffff";
    document.querySelectorAll(".theme-toggle").forEach(button => {
        button.setAttribute("aria-pressed", String(dark));
        button.querySelector("use").setAttribute("href", dark ? "#icon-sun" : "#icon-moon");
        const label = dark ? "Light mode" : "Dark mode";
        const text = button.querySelector("span");
        if (text) text.textContent = label;
        else button.setAttribute("aria-label", label);
    });
}

function toggleTheme() {
    const next = document.documentElement.dataset.theme === "dark" ? "light" : "dark";
    try { localStorage.setItem("theme", next); } catch { /* private mode: still switch for this visit */ }
    applyTheme(next);
}

// Until the user picks a theme, follow the OS setting live.
systemDark.addEventListener("change", event => {
    if (!savedTheme()) applyTheme(event.matches ? "dark" : "light");
});
applyTheme(document.documentElement.dataset.theme || "light");
document.querySelectorAll(".theme-toggle").forEach(button => button.addEventListener("click", toggleTheme));

/* ==========================================================================
   Wiring
   ========================================================================== */

$("login-form").addEventListener("submit", login);
$("register-form").addEventListener("submit", register);
document.querySelectorAll("[data-auth]").forEach(button => {
    button.addEventListener("click", () => {
        $("login-message").textContent = "";
        $("register-message").textContent = "";
        showAuth(button.dataset.auth);
    });
});

document.querySelectorAll("[data-view]").forEach(button => {
    button.addEventListener("click", () => {
        if (button.dataset.view === "home" && currentView === "home") {
            loadPosts();
        }
        showView(button.dataset.view);
    });
});
document.querySelector("[data-action='open-chat']").addEventListener("click", openChatDrawer);

$("sidebar-post-button").addEventListener("click", () => {
    showView("home");
    $("post-title").focus();
});

$("post-form").addEventListener("submit", createPost);
["post-title", "post-content", "post-categories"].forEach(id => {
    $(id).addEventListener("input", updateComposerButton);
});
$("post-content").addEventListener("input", event => autoGrow(event.currentTarget));

$("search-input").addEventListener("input", event => {
    searchQuery = event.currentTarget.value.trim().toLowerCase();
    if (currentView !== "home") showView("home");
    renderFeed();
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
    if (event.key !== "Escape") return;
    if (!$("account-menu").hidden) closeAccountMenu();
    else if (!$("chat-drawer").hidden) closeChatDrawer();
});

$("chat-fab").addEventListener("click", openChatDrawer);
$("chat-close").addEventListener("click", closeChatDrawer);
$("chat-back").addEventListener("click", closeConversation);
$("message-form").addEventListener("submit", sendMessage);
$("message-content").addEventListener("input", event => {
    autoGrow(event.currentTarget);
    updateTypingStatus();
});
$("message-content").addEventListener("keydown", submitOnEnter);
$("chat-message-list").addEventListener("scroll", handleChatScroll);
window.addEventListener("resize", () => {
    if (!$("chat-conversation").hidden) updateOlderHint();
});

restoreSession();
