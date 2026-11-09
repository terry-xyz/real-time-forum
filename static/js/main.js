import { initAuth, restoreSession } from "./auth.js";
import { initChat, startChat, stopChat } from "./chat.js";
import { $, paintAvatar } from "./dom.js";
import { initPosts, loadCategories, loadPosts } from "./posts.js";
import { initProfile, showView, renderProfile } from "./profile.js";
import { initTheme } from "./theme.js";

// onAuthenticated paints the auth-owned user and starts the signed-in features from the home view.
async function onAuthenticated(user) {
    // Auth supplies the confirmed user so account actions and the composer show the same identity.
    $("account-name").textContent = user.nickname;
    $("account-handle").textContent = `@${user.nickname}`;
    $("logout-handle").textContent = `@${user.nickname}`;
    paintAvatar($("account-avatar"), user.nickname);
    paintAvatar($("composer-avatar"), user.nickname);

    showView("home");
    // Chat owns its initial user request; load independent feed data alongside it without duplicating it.
    await Promise.all([startChat(), loadCategories(), loadPosts()]);
}

// onLoggedOut releases chat resources while auth clears session state and shows the login form.
function onLoggedOut() {
    stopChat();
}

initTheme();
// Auth must register Escape first so an open account menu consumes it before chat handles the key.
initAuth({ onAuthenticated, onLoggedOut });
initPosts({ showView, renderProfile });
initProfile();
initChat();

// Restore only after every handler is ready; auth invokes onAuthenticated for a restored session itself.
restoreSession();
