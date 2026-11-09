import { state } from "./state.js";
import { getChatUsers, getMessages, sendMessage as postMessage, getChatSocketURL, readError } from "./api.js";
import { $, el, avatar, autoGrow, showToast } from "./dom.js";

const MESSAGE_PAGE_SIZE = 10;
const USER_REFRESH_MS = 20000;
const OLDER_MESSAGES_THROTTLE_MS = 300;
const TYPING_THROTTLE_MS = 1000;

let initialized = false;
let active = false;
let sessionVersion = 0;
let conversationVersion = 0;
let userRefreshVersion = 0;
let chatUsers = [];
let selectedChatUser = null;
let loadingInitialMessages = false;
const conversationMessages = new Map();
let historyCursorId = null;
let pendingSend = null;
let chatSocket = null;
let typingStatusTimeout = null;
let userRefreshTimer = null;
let hasOlderMessages = false;
let loadingOlderMessages = false;
let lastOlderMessagesLoadAt = 0;
let lastTypingSentAt = 0;

// initChat binds persistent controls once, including delegated clicks for refreshed user rows.
export function initChat() {
    if (initialized) return;
    initialized = true;

    document.querySelector("[data-action='open-chat']").addEventListener("click", openChatDrawer);
    $("chat-fab").addEventListener("click", openChatDrawer);
    $("chat-close").addEventListener("click", closeChatDrawer);
    $("chat-back").addEventListener("click", closeConversation);
    $("side-user-list").addEventListener("click", handleUserListClick);
    $("chat-user-list").addEventListener("click", handleUserListClick);
    $("message-form").addEventListener("submit", sendMessage);
    $("message-content").addEventListener("input", handleMessageInput);
    $("message-content").addEventListener("keydown", submitOnEnter);
    $("chat-message-list").addEventListener("scroll", handleChatScroll);
    document.addEventListener("keydown", handleEscape);
    window.addEventListener("resize", handleResize);
}

// startChat opens the signed-in socket and starts presence polling before loading both user lists.
export function startChat() {
    if (!state.currentUser) return;
    if (!active) {
        active = true;
        sessionVersion += 1;
    }
    connectChatSocket();
    startUserRefresh();
    return loadChatUsers();
}

// stopChat invalidates pending work and releases the signed-in chat resources and unread state.
export function stopChat() {
    active = false;
    sessionVersion += 1;
    closeChatSocket();
    stopUserRefresh();
    chatUsers = [];
    state.unreadUserIds.clear();
    lastTypingSentAt = 0;
    pendingSend = null;
    closeConversation();
    $("message-content").value = "";
    $("message-content").style.height = "";
    $("chat-message-list").replaceChildren();
    $("chat-message-list").scrollTop = 0;
    $("message-status").textContent = "";
    // Incoming-message feedback can name the previous account's contacts, so clear it on logout.
    $("toast").hidden = true;
    $("toast").textContent = "";
    $("chat-drawer").hidden = true;
    $("chat-fab").hidden = true;
}

// startUserRefresh polls presence because the existing socket protocol does not push it.
function startUserRefresh() {
    stopUserRefresh();
    userRefreshTimer = setInterval(() => {
        if (document.visibilityState === "visible") loadChatUsers();
    }, USER_REFRESH_MS);
}

// stopUserRefresh clears the previous interval so repeated app entry cannot duplicate polling.
function stopUserRefresh() {
    clearInterval(userRefreshTimer);
    userRefreshTimer = null;
}

// loadChatUsers refreshes the server's conversation ordering and the selected user's presence.
async function loadChatUsers() {
    const session = sessionVersion;
    if (!active) return;
    const refresh = ++userRefreshVersion;
    try {
        const response = await getChatUsers();
        if (!active || session !== sessionVersion || refresh !== userRefreshVersion) return;
        if (!response.ok) {
            renderUserLists("Failed to load users.");
            return;
        }
        const users = (await response.json()) || [];
        // Logout or a newer refresh can finish during body parsing, so check ownership again.
        if (!active || session !== sessionVersion || refresh !== userRefreshVersion) return;
        chatUsers = users;
    } catch (error) {
        if (!active || session !== sessionVersion || refresh !== userRefreshVersion) return;
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

// renderUserLists keeps the sidebar, drawer, presence count, and unread indicators in sync.
function renderUserLists(errorText = "") {
    for (const container of [$("side-user-list"), $("chat-user-list")]) {
        if (errorText || chatUsers.length === 0) {
            container.replaceChildren(el("p", { text: errorText || "No other users yet." }));
        } else {
            container.replaceChildren(...chatUsers.map(createUserRow));
        }
    }

    const onlineCount = chatUsers.filter(user => user.online).length;
    const onlineText = errorText || chatUsers.length === 0 ? "" : `${onlineCount} online`;
    $("online-count").textContent = onlineText;
    if (!selectedChatUser) $("chat-subtitle").textContent = onlineText;

    const hasUnread = state.unreadUserIds.size > 0;
    $("chat-badge").hidden = !hasUnread;
    $("chat-fab-dot").hidden = !hasUnread;
}

// createUserRow renders the UI's avatar, accessible presence indicator, and optional unread dot.
function createUserRow(user) {
    const face = avatar(user.nickname);
    const presence = el("span", {
        className: `presence-indicator ${user.online ? "online" : "offline"}`,
        attrs: { "aria-label": user.online ? "Online" : "Offline", role: "img" }
    });
    face.removeAttribute("aria-hidden");
    face.setAttribute("role", "presentation");
    face.append(presence);

    return el("button", {
        className: `user-row${selectedChatUser?.id === user.id ? " selected" : ""}`,
        attrs: { type: "button", "data-user-id": user.id }
    }, [
        face,
        el("span", { className: "user-row-text" }, [
            el("strong", { text: user.nickname }),
            el("span", { text: user.online ? "Online" : "Offline" })
        ]),
        state.unreadUserIds.has(user.id)
            ? el("span", { className: "unread-dot", attrs: { "aria-label": "Unread messages", role: "img" } })
            : null
    ]);
}

// handleUserListClick resolves the latest user data without attaching handlers to replaced rows.
function handleUserListClick(event) {
    if (!active) return;
    const row = event.target.closest(".user-row[data-user-id]");
    if (!row || !event.currentTarget.contains(row)) return;
    const user = chatUsers.find(candidate => String(candidate.id) === row.dataset.userId);
    if (user) openConversation(user);
}

// openChatDrawer reveals the existing selection or the drawer's user list.
function openChatDrawer() {
    if (!active) return;
    $("chat-drawer").hidden = false;
    $("chat-fab").hidden = true;
    if (!selectedChatUser) showUserListInDrawer();
}

// closeChatDrawer clears the selection before restoring the floating chat control.
function closeChatDrawer() {
    closeConversation();
    $("chat-drawer").hidden = true;
    $("chat-fab").hidden = false;
}

// showUserListInDrawer restores the drawer's list heading and online count.
function showUserListInDrawer() {
    $("chat-title").textContent = "Chats";
    $("chat-subtitle").textContent = $("online-count").textContent;
    $("chat-back").hidden = true;
    $("chat-user-list").hidden = false;
    $("chat-conversation").hidden = true;
}

// closeConversation invalidates history requests and clears typing before returning to the list.
function closeConversation() {
    selectedChatUser = null;
    conversationVersion += 1;
    loadingInitialMessages = false;
    conversationMessages.clear();
    historyCursorId = null;
    hasOlderMessages = false;
    loadingOlderMessages = false;
    lastOlderMessagesLoadAt = 0;
    clearTimeout(typingStatusTimeout);
    typingStatusTimeout = null;
    $("typing-status").textContent = "";
    showUserListInDrawer();
    renderUserLists();
}

// updateChatHeader uses the selected user's latest nickname and presence from the server.
function updateChatHeader() {
    if (!selectedChatUser) return;
    $("chat-title").textContent = selectedChatUser.nickname;
    $("chat-subtitle").textContent = selectedChatUser.online ? "Online" : "Offline";
}

// conversationIsCurrent prevents responses from a previous selection or session changing the drawer.
function conversationIsCurrent(session, conversation) {
    return active && session === sessionVersion && conversation === conversationVersion;
}

// openConversation marks the user read and loads the latest page into the drawer.
async function openConversation(user) {
    selectedChatUser = user;
    const session = sessionVersion;
    const conversation = ++conversationVersion;
    state.unreadUserIds.delete(user.id);
    loadingInitialMessages = true;
    conversationMessages.clear();
    historyCursorId = null;
    hasOlderMessages = false;
    loadingOlderMessages = false;
    lastOlderMessagesLoadAt = 0;
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
        const response = await getMessages(user.id, { limit: MESSAGE_PAGE_SIZE });
        if (!conversationIsCurrent(session, conversation)) return;
        if (!response.ok) {
            finishInitialHistory([], user, "Failed to load messages.");
            return;
        }

        const messages = (await response.json()) || [];
        if (!conversationIsCurrent(session, conversation)) return;
        finishInitialHistory(messages, user);
        $("message-content").focus();
    } catch (error) {
        if (!conversationIsCurrent(session, conversation)) return;
        console.error("Conversation loading error:", error);
        finishInitialHistory([], user, "Failed to load messages.");
    }
}

// finishInitialHistory merges live events with the initial snapshot so neither can erase the other.
function finishInitialHistory(history, user, errorText = "") {
    mergeConversationMessages(history);
    // Only server pages advance pagination; a buffered event can sort before unloaded server history.
    historyCursorId = history[0]?.id ?? null;
    loadingInitialMessages = false;
    hasOlderMessages = !errorText && history.length === MESSAGE_PAGE_SIZE;

    const list = $("chat-message-list");
    if (conversationMessages.size === 0) {
        list.replaceChildren(errorText
            ? el("p", { text: errorText })
            : el("p", { className: "chat-empty", text: `No messages yet. Say hi to ${user.nickname}!` }));
        return;
    }
    renderConversationMessages();
    if (errorText) $("message-status").textContent = errorText;
    list.scrollTop = list.scrollHeight;
}

// mergeConversationMessages deduplicates server pages and buffered live events by their message IDs.
function mergeConversationMessages(messages) {
    for (const message of messages) conversationMessages.set(String(message.id), message);
}

// renderConversationMessages orders the full merged history like the backend before restoring its hint.
function renderConversationMessages() {
    const messages = [...conversationMessages.values()].sort((left, right) => {
        const timeDifference = Date.parse(left.created_at) - Date.parse(right.created_at);
        if (timeDifference !== 0) return timeDifference;
        return left.id < right.id ? -1 : left.id > right.id ? 1 : 0;
    });
    $("chat-message-list").replaceChildren(...messages.map(createChatMessage));
    updateOlderHint();
}

// captureMessageAnchor records the first visible message's position within the scrolling viewport.
function captureMessageAnchor(list) {
    const viewportTop = list.getBoundingClientRect().top + list.clientTop;
    const viewportBottom = viewportTop + list.clientHeight;
    for (const message of list.querySelectorAll(".chat-message")) {
        const bounds = message.getBoundingClientRect();
        if (bounds.bottom > viewportTop && bounds.top < viewportBottom) {
            return { messageId: message.dataset.messageId, offset: bounds.top - viewportTop };
        }
    }
    return null;
}

// restoreMessageAnchor corrects the same message's displacement, even when new messages sort after it.
function restoreMessageAnchor(list, anchor, fallbackScrollTop) {
    if (anchor) {
        const message = [...list.querySelectorAll(".chat-message")]
            .find(candidate => candidate.dataset.messageId === anchor.messageId);
        if (message) {
            const viewportTop = list.getBoundingClientRect().top + list.clientTop;
            list.scrollTop += message.getBoundingClientRect().top - viewportTop - anchor.offset;
            return;
        }
    }
    // The viewport can contain only the older-history hint, so retain the caller's safe scroll fallback.
    list.scrollTop = fallbackScrollTop;
}

// createChatMessage preserves full local timestamps and safe text for sender names and content.
function createChatMessage(message) {
    const mine = message.sender_id === state.currentUser?.id;
    const time = el("time", { text: new Date(message.created_at).toLocaleString() });
    time.dateTime = message.created_at;
    const item = el("article", { className: `chat-message${mine ? " mine" : ""}` }, [
        el("p", { className: "chat-bubble", text: message.content }),
        el("div", { className: "chat-message-meta" }, [
            document.createTextNode(`${message.sender_name} · `),
            time
        ])
    ]);
    item.dataset.messageId = message.id;
    return item;
}

// appendChatMessage follows incoming messages only near the bottom and always follows sent messages.
function appendChatMessage(message) {
    const id = String(message.id);
    if (conversationMessages.has(id)) return;
    conversationMessages.set(id, message);
    if (loadingInitialMessages) return;
    const list = $("chat-message-list");
    const previousScrollTop = list.scrollTop;
    const wasAtBottom = list.scrollHeight - list.scrollTop - list.clientHeight < 40;
    const anchor = wasAtBottom ? null : captureMessageAnchor(list);
    renderConversationMessages();
    if (wasAtBottom || message.sender_id === state.currentUser?.id) list.scrollTop = list.scrollHeight;
    else restoreMessageAnchor(list, anchor, previousScrollTop);
}

// updateOlderHint ensures short histories overflow so scrolling to the top can request another page.
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
    // scrollHeight cannot fall below clientHeight, so child offsets reveal the real content height.
    const last = list.lastElementChild;
    const contentHeight = last.offsetTop + last.offsetHeight - hint.offsetTop;
    const style = getComputedStyle(list);
    const innerHeight = list.clientHeight - parseFloat(style.paddingTop) - parseFloat(style.paddingBottom);
    const spare = innerHeight - contentHeight;
    if (spare >= 0) hint.style.minHeight = `${hint.offsetHeight + spare + 40}px`;
}

// handleChatScroll throttles top-of-list requests and prevents concurrent older-page loads.
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

// loadOlderMessages advances the server-page cursor and merges older history without shifting the view.
async function loadOlderMessages() {
    const user = selectedChatUser;
    const session = sessionVersion;
    const conversation = conversationVersion;
    const list = $("chat-message-list");
    if (!user || !historyCursorId) return;

    loadingOlderMessages = true;
    try {
        const response = await getMessages(user.id, {
            limit: MESSAGE_PAGE_SIZE,
            beforeId: historyCursorId
        });
        if (!conversationIsCurrent(session, conversation)) return;
        if (!response.ok) {
            const errorText = await readError(response);
            if (conversationIsCurrent(session, conversation)) $("message-status").textContent = errorText;
            return;
        }

        const messages = (await response.json()) || [];
        if (!conversationIsCurrent(session, conversation)) return;
        // Sorted inserts can land on either side of the reader, so anchor to the visible message itself.
        const anchor = captureMessageAnchor(list);
        const previousHeight = list.scrollHeight;
        const previousScrollTop = list.scrollTop;
        mergeConversationMessages(messages);
        historyCursorId = messages[0]?.id ?? null;
        hasOlderMessages = messages.length === MESSAGE_PAGE_SIZE;
        renderConversationMessages();
        restoreMessageAnchor(list, anchor, previousScrollTop + list.scrollHeight - previousHeight);
    } catch (error) {
        if (!conversationIsCurrent(session, conversation)) return;
        console.error("Older message loading error:", error);
        $("message-status").textContent = "Failed to load older messages.";
    } finally {
        // A previous conversation's completion must not unlock the current conversation's request.
        if (conversationIsCurrent(session, conversation)) loadingOlderMessages = false;
    }
}

// sendMessage posts the original textarea text and refreshes the server's latest-message ordering.
async function sendMessage(event) {
    event.preventDefault();
    if (!active || pendingSend) return;
    const session = sessionVersion;
    const conversation = conversationVersion;
    const status = $("message-status");
    const textarea = $("message-content");
    const recipient = selectedChatUser;
    if (!recipient) {
        status.textContent = "Choose a user first.";
        return;
    }
    const content = textarea.value;
    if (!content.trim()) return;
    const request = { session, conversation, receiverId: recipient.id, content };
    pendingSend = request;

    try {
        const response = await postMessage({ receiver_id: request.receiverId, content: request.content });
        if (!active || session !== sessionVersion) return;
        if (!response.ok) {
            const errorText = await readError(response);
            if (conversationIsCurrent(session, conversation)) status.textContent = errorText;
            return;
        }

        const message = await response.json();
        if (!active || session !== sessionVersion) return;
        if (conversationIsCurrent(session, conversation)) {
            if (textarea.value === request.content) {
                textarea.value = "";
                autoGrow(textarea);
            }
            status.textContent = "";
        }
        if (selectedChatUser?.id === request.receiverId) {
            appendChatMessage(message);
        }
        // Presence refresh has its own error handling and should not keep the send lock held.
        loadChatUsers();
    } catch (error) {
        if (!conversationIsCurrent(session, conversation)) return;
        console.error("Message sending error:", error);
        status.textContent = "Something went wrong.";
    } finally {
        // Logout may have started a new account's send, so release only this request's lock.
        if (pendingSend === request) pendingSend = null;
    }
}

// updateTypingStatus sends at most once per second so a continuing draft renews the 1.5-second indicator.
function updateTypingStatus() {
    const recipient = selectedChatUser;
    if (!recipient || chatSocket?.readyState !== WebSocket.OPEN) return;
    if (!$("message-content").value.trim()) return;
    const now = Date.now();
    if (now - lastTypingSentAt < TYPING_THROTTLE_MS) return;
    lastTypingSentAt = now;
    chatSocket.send(JSON.stringify({ type: "typing", receiver_id: recipient.id }));
}

// handleMessageInput keeps the draft height and outgoing typing status aligned with user input.
function handleMessageInput(event) {
    autoGrow(event.currentTarget);
    updateTypingStatus();
}

// submitOnEnter sends on Enter while preserving newlines and input-method composition.
function submitOnEnter(event) {
    if (event.key === "Enter" && !event.shiftKey && !event.isComposing) {
        event.preventDefault();
        event.currentTarget.form.requestSubmit();
    }
}

// handleEscape defers to auth's account-menu handler so one Escape closes only one surface.
function handleEscape(event) {
    if (event.key !== "Escape" || event.defaultPrevented || !$("account-menu").hidden) return;
    if (!$("chat-drawer").hidden) closeChatDrawer();
}

// handleResize recalculates the older-history affordance when the conversation height changes.
function handleResize() {
    if (!$("chat-conversation").hidden) updateOlderHint();
}

// connectChatSocket reuses an open connection and allows a later app entry to replace a closed one.
function connectChatSocket() {
    if (chatSocket?.readyState === WebSocket.OPEN || chatSocket?.readyState === WebSocket.CONNECTING) return;
    const socket = new WebSocket(getChatSocketURL());
    chatSocket = socket;
    socket.addEventListener("close", () => {
        if (chatSocket === socket) chatSocket = null;
    });
    socket.addEventListener("message", handleChatSocketMessage);
    socket.addEventListener("error", error => {
        console.error("Chat connection error:", error);
    });
}

// closeChatSocket releases ownership immediately so queued events from the old socket are ignored.
function closeChatSocket() {
    const socket = chatSocket;
    chatSocket = null;
    socket?.close();
}

// handleChatSocketMessage preserves typing and message payloads, unread dots, and incoming-message toasts.
function handleChatSocketMessage(event) {
    if (!active || event.currentTarget !== chatSocket) return;
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
        state.unreadUserIds.add(message.sender_id);
        showToast(`New message from ${message.sender_name}`);
    }
    loadChatUsers();
}
