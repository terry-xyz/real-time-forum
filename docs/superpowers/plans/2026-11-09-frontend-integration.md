# Frontend Integration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Integrate the new UI into the existing `static/` frontend and split both frontends' JavaScript into readable browser modules while preserving the current `ui/` appearance and behavior.

**Architecture:** Keep `static/index.html` and `static/style.css` as the served files, update the page markup for the new UI, and combine the styles. Use native ES modules under `static/js/`, with a small startup module, shared API/DOM/app-state modules, and feature modules for auth, posts, comments, profile, chat, and theme. Leave Go and the backend API unchanged.

**Tech Stack:** Go `net/http` static file server, HTML, CSS, browser JavaScript ES modules, existing `/api/*` endpoints and WebSocket.

---

## File map

| File | Responsibility |
| --- | --- |
| `static/index.html` | New UI markup at the existing path; preserve theme pre-paint and load the module entry point. |
| `static/style.css` | Existing styles plus UI styles, reconciled to preserve the UI appearance. |
| `static/main.js` | Remove the old single-file runtime after porting its behavior. |
| `static/js/main.js` | Startup, session restoration, and coordination of feature modules. |
| `static/js/api.js` | Shared HTTP helpers and existing endpoint requests/error handling. |
| `static/js/state.js` | App-wide user, view, feed, and filter state. |
| `static/js/dom.js` | Safe element construction, icons, avatars, date formatting, textarea sizing, shared toast feedback. |
| `static/js/auth.js` | Login, registration, logout, session restore, auth view switching. |
| `static/js/posts.js` | Posts, categories, search, feed rendering, composer. |
| `static/js/comments.js` | Comment loading, rendering, and submission. |
| `static/js/profile.js` | Profile rendering and navigation. |
| `static/js/chat.js` | Chat users, conversations, messages, WebSocket, typing, presence, pagination. |
| `static/js/theme.js` | Saved/system theme selection and toggles. |

Modules use named exports. Feature modules depend on shared modules; `main.js` coordinates startup. Keep socket, timers, selection, throttles, and pagination state private to `chat.js`. Avoid cyclic imports. Where the old static frontend and the current UI differ, the UI behavior is authoritative; bring over old implementation details only when they preserve that behavior and backend compatibility.

## Task 1: Inventory current behavior

**Files:** Read `static/main.js`, `ui/main.js`, `static/index.html`, and `ui/index.html`.

- [ ] Record common and unique flows from both scripts: auth, posts, category filtering, comments, chat, typing, presence, pagination, error/empty feedback, and navigation. Map each function to a module in the file map, marking the current UI behavior as authoritative in conflicts.
- [ ] Record every ID, class, and `data-*` attribute queried by the UI script; use it as the markup contract for Task 2.

## Task 2: Update the page markup

**Files:** Modify `static/index.html`; use `ui/index.html` as reference.

- [ ] Update the contents of the existing static page to support the new UI. Preserve the file path, document metadata as appropriate, theme pre-paint script, SVG icon sprite, auth forms, app shell, profile, chat drawer, and toast.
- [ ] Load only `/js/main.js` with `type="module"` at the end of body.
- [ ] Compare page IDs and data attributes against Task 1's inventory. Ensure auth, posts, comments, profile, chat, theme, status, and loading/empty states have the elements their modules need.

## Task 3: Combine CSS

**Files:** Modify `static/style.css`; reference `ui/style.css`.

- [ ] Incorporate the UI rules into the existing static stylesheet, retaining rules still used by the integrated page and reconciling conflicts in favor of the new UI's appearance.
- [ ] Check auth, feed, app shell, profile, chat drawer, light/dark theme, responsive breakpoints, and hidden states. Confirm the stylesheet keeps `[hidden]` elements hidden.

## Task 4: Create shared modules

**Files:** Create `static/js/api.js`, `static/js/state.js`, and `static/js/dom.js`.

- [ ] Put only cross-feature state in `state.js`: current user/view, posts, categories, active category, search query, and any shared unread state. Keep chat-only state in `chat.js`.
- [ ] Centralize repeated fetch setup and response error text handling in `api.js`. Preserve paths and request shapes for register, login, logout, check-auth, categories, posts, comments, chat users, messages, and `/api/ws`.
- [ ] Move safe element creation, icon/avatar helpers, relative time, time elements, textarea auto-grow, and the shared toast helper into `dom.js`. Keep API/user text rendered via `textContent`; keep the toast timeout private to the helper.

## Task 5: Extract auth and theme

**Files:** Create `static/js/auth.js` and `static/js/theme.js`; modify `static/js/dom.js`; use `state.js`.

- [ ] Port login, registration, logout, session restoration, account menu, and signed-in/out transitions from `ui/main.js`. Preserve the current UI's success/failure feedback and fixed form flow. Reuse compatible backend response handling from the old script. Export explicit initialization and session restore functions.
- [ ] Port theme storage, system preference handling, application, and toggles. Keep the inline pre-paint script and CSS theme attributes consistent with the stored `"theme"` value.
- [ ] Manually inspect signed-out view, login, registration feedback, logout, reload/session restore, theme toggle, and theme persistence using `go run .` at `http://localhost:8080`.

## Task 6: Extract posts, comments, and profile

**Files:** Create `static/js/posts.js`, `static/js/comments.js`, and `static/js/profile.js`; use `state.js`.

- [ ] Port category loading, post loading/creation, feed/category rendering, search, composer state, and post interactions into `posts.js`. Preserve the UI's client-side category tabs and filtering behavior. Reuse compatible API/error handling from the old script without changing the UI interaction.
- [ ] Port comment load/render/submit behavior from both scripts into `comments.js`, preserving endpoint shapes and user-visible feedback.
- [ ] Port profile rendering and navigation into `profile.js`. Avoid cyclic imports; use a callback supplied by `main.js` if posts and profile need to call each other's rendering behavior.
- [ ] Manually inspect post creation, category filtering, search, comments, profile, empty states, and request failure feedback against the UI preview.

## Task 7: Extract chat

**Files:** Create `static/js/chat.js`; use `state.js` only for app-wide values.

- [ ] Port user refresh, online presence, drawer controls, conversation selection, message load/send/receive, typing, unread/new-message feedback, older-message pagination, and socket lifecycle from both scripts.
- [ ] Keep socket/timer/pagination state private to `chat.js`; bind chat listeners once during initialization so list refreshes do not duplicate handlers. Preserve websocket event names and payload fields.
- [ ] With two authenticated browser sessions, manually inspect online/offline status, conversation selection, send/receive, typing, incoming message feedback while another conversation is selected, logout socket closure, and older-message loading with scroll position preserved.

## Task 8: Wire modules and remove old entry point

**Files:** Create `static/js/main.js`; modify `static/index.html`; delete `static/main.js`.

- [ ] Import public module functions in `main.js`, initialize handlers once, restore the session, and coordinate app entry/exit. Keep feature-to-feature dependencies flowing through shared modules or explicit callbacks.
- [ ] Confirm `static/index.html` has exactly one runtime script entry: `<script type="module" src="/js/main.js"></script>`.
- [ ] After accounting for all old static behavior, remove `static/main.js` so there is one frontend runtime.
- [ ] Run `go run .`, open `http://localhost:8080`, and confirm module requests succeed, there are no missing DOM references or console errors, and API calls remain same-origin.

## Task 9: Final parity review

**Files:** Review `static/index.html`, `static/style.css`, and `static/js/*.js`; compare with `ui/`.

- [ ] Compare signed-out, signed-in home, profile, chat drawer, light/dark theme, and narrow viewport states with the UI preview; resolve integration-caused differences.
- [ ] Walk the full feature checklist: auth/session, posts, categories, search, comments, profile, theme persistence, chat users/presence, conversation, send/receive, typing, unread feedback, older messages, and empty/error states.
- [ ] Review the final diff to ensure backend, API, database, and dependency files were not changed, and unrelated pre-existing worktree changes remain untouched.

## Plan self-review

- **Spec coverage:** Tasks 2–3 cover page and CSS integration; Tasks 4–8 cover the ES module structure and all features; Tasks 1, 5–9 cover UI-authoritative behavior preservation and manual parity review.
- **Placeholders:** No TODO/TBD or unspecified test-writing instructions.
- **Interfaces:** App-wide state is limited; chat lifecycle state stays private; startup coordinates features and avoids cyclic imports.
