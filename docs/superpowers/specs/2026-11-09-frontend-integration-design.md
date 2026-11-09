# Frontend Integration and JavaScript Architecture

## Goal

Integrate the frontend currently in `ui/` into the existing `static/` frontend location and organize the JavaScript from both frontend implementations into focused browser modules. Preserve the website's current appearance, interactions, and feature set. This is a structural refactor, not a redesign or behavior change.

## Current context

- `main.go` serves `./static` and the existing `/api/*` handlers from the same origin.
- `static/` contains the existing `index.html`, `style.css`, and an older single-file frontend in `main.js`.
- `ui/` contains the newer UI markup, stylesheet, and a larger `main.js` with auth, posts, comments, profile, theme, chat, presence, typing, and message history behavior.
- The UI preview server proxies `/api/*` to the backend, so the integrated frontend can continue using the same API paths and WebSocket endpoint without a build step.

## Target files

Keep `static/index.html` as the page path and update its markup to support the new UI. Keep `static/style.css` and incorporate the UI stylesheet rules into it, resolving selector conflicts so the integrated page retains the new UI's appearance. Place the modular JavaScript under `static/js/` and load the entry module from `static/index.html` with a browser ES module script.

The Go server, API routes, request and response shapes, and database remain unchanged.

## JavaScript modules

Use native browser ES modules; do not add a bundler or package dependencies for this refactor.

- `static/js/main.js`: application startup and coordination of feature initialization.
- `static/js/api.js`: shared HTTP requests and response error handling for existing API routes.
- `static/js/state.js`: only app-wide state shared across features, such as the current user and current view. Chat-specific timers, socket state, selection, and pagination remain private to `chat.js`.
- `static/js/dom.js`: shared DOM construction and formatting helpers used by multiple features.
- `static/js/auth.js`: login, registration, logout, session restore, and signed-in/signed-out transitions.
- `static/js/posts.js`: post loading/creation, category filtering, search, and feed rendering.
- `static/js/comments.js`: comment loading, rendering, and submission.
- `static/js/profile.js`: profile view and profile post rendering.
- `static/js/chat.js`: chat user lists, conversations, message history, pagination, typing status, presence, and WebSocket events.
- `static/js/theme.js`: saved/system theme selection and toggle behavior.

Modules expose small initialization or action functions. `main.js` wires them together. Avoid a large shared global namespace and avoid cyclic imports; shared dependencies flow from feature modules toward `api.js`, `dom.js`, and `state.js`.

## Behavior preservation

Treat the current `ui/` frontend as the visual and interaction baseline. Preserve all user-visible behavior currently provided by either JavaScript file, including behavior that exists only in the older `static/main.js`. Reconcile duplicate auth, posts, comments, and chat logic as one implementation per feature, preserving its user-visible outcomes and compatibility with the existing backend.

Keep element IDs, classes, and data attributes aligned with the feature modules that use them. Preserve the theme pre-paint behavior in the page head. Keep API paths and WebSocket URL behavior unchanged.

## Migration sequence

1. Inventory the behavior and event wiring in both current `main.js` files, recording overlaps and behaviors unique to either implementation.
2. Update `static/index.html` markup to support the new UI while retaining the existing file path and theme pre-paint behavior.
3. Combine the existing and UI stylesheet rules in `static/style.css`; resolve conflicting selectors against the new UI appearance.
4. Extract shared API, state, and DOM helpers, then move each feature into its focused module.
5. Add the `type="module"` entry in `static/index.html` and initialize the features through `static/js/main.js`.
6. Compare the integrated site with the UI preview for visual and interaction parity, and confirm that behaviors unique to the older static frontend remain available.

## Acceptance criteria

- The production site is served from `static/` at the existing URL and uses the existing backend.
- The integrated page retains the new UI's appearance and interactions.
- Features from both current frontends remain available, including auth, posts, categories, comments, profile, theme, chat, typing, online presence, and older-message loading.
- JavaScript responsibilities are separated into the modules described above, with a small startup entry point.
- No backend, API, schema, or dependency changes are needed.

## Verification

After implementation, run the app and manually compare the integrated frontend against the current UI preview, covering signed-out and signed-in views, post and comment flows, profile and theme controls, and chat including presence, typing, and older-message loading. Also verify that any behavior unique to the old static frontend still works. This design does not call for adding automated tests as part of the architecture-only change.
