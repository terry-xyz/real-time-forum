# Changelog

This changelog is an agent handoff document. It records what agents changed, what worked, what failed or remains uncertain, and what the next agent should know before making more changes.

## 2026-06-24

### Completed By Codex

- Built a new Go 1.24 real-time forum application at the repository root, using the existing `forum/` project only as reference context.
- Implemented SQLite persistence, schema initialization, default categories, cookie sessions, bcrypt password hashing, JSON APIs, and WebSocket private messaging in `app.go`.
- Added registration and login flows with nickname or email login, full registration fields, dotted-domain email validation, and Male/Female-only gender normalization.
- Added post, category, comment, user list, message history, older-message pagination, and authenticated WebSocket chat behavior.
- Added a static single-page interface in `static/index.html`, `static/app.js`, and `static/styles.css`.
- Fixed frontend form access bugs by reading named controls through stable helpers instead of ambiguous form properties.
- Fixed post creation and private messaging from the logged-in UI, including WebSocket send guarding until the socket is open.
- Added native browser email validation for `x@x` through a dotted-domain pattern and cleared stale auth errors while editing auth forms.
- Added a visible post-form error for empty category selection: `Please select at least one category`.
- Added tests in `app_test.go` covering auth, validation, frontend guardrails, posts, comments, empty array JSON responses, chat user sorting, message pagination, and WebSocket delivery.
- Added root `.gitignore` and `LICENSE`.
- Ignored `audit/`, `forum/`, and `specs/` as requested; these context directories are not part of the committed root project.
- Split the initial project into five commits and pushed branch `dev-lpapanthy` to `origin/dev-lpapanthy`.

### Commit State

- `306032b chore(.gitignore): ignore project context directories`
- `2655e96 build(go.mod): add forum runtime dependencies`
- `ac5b477 feat(app.go): implement forum API and realtime messaging`
- `f07b59b feat(static): add realtime forum interface`
- `fc93be7 test(app_test.go): cover forum workflows and validations`

### What Worked

- A compact standard-library HTTP server with `gorilla/websocket` is enough for the required API and real-time chat workflow.
- SQLite works as local persistence; `SetMaxOpenConns(1)` keeps the cgo SQLite driver predictable for this app and in-memory tests.
- Cookie-backed sessions keep HTTP API authentication and WebSocket authentication aligned.
- Empty API collections are initialized as arrays so the SPA receives `[]` instead of `null`.
- Message pagination returns the latest ten messages, displays them oldest-to-newest, and supports `before_id` for older pages.
- `httptest.Server` plus `cookiejar` covers authenticated API flows, and WebSocket tests authenticate by forwarding the session cookie.
- Browser verification confirmed the no-category post flow now shows the form error and does not create a post.

### Issues Encountered

- Git writes initially failed with `.git/index.lock: Permission denied` under the earlier sandbox. Commits succeeded after running git with the required elevated permissions.
- Browser verification initially loaded stale `/static/app.js`; `static/index.html` now references `/static/app.js?v=post-error-1` so the current script is fetched.
- Go verification needed `CC=C:\w64devkit\bin\gcc.exe` and workspace-local `GOCACHE`/`GOTMPDIR` during sandboxed runs because the SQLite driver uses cgo.
- The first push printed a remote hook warning about an ambiguous zero hash, but Git still created `origin/dev-lpapanthy`. A follow-up `git push -u origin dev-lpapanthy` set upstream tracking and reported everything up to date.
- Git status still prints a warning about `C:\Users\pleft/.config/git/ignore` permission; it has not blocked local commits or pushes.

### Known Risks For Next Agent

- `handleWebSocket` currently accepts every origin through `CheckOrigin`; this is fine for local/demo use only if deployment is constrained elsewhere.
- The API has no CSRF protection for cookie-authenticated POST requests.
- There is no rate limiting, production migration strategy, or deployment configuration yet.
- Comment submission in `static/app.js` still uses `event.target.content.value`; prefer the existing named-control helper if that path is edited.
- `go.mod` targets Go 1.24.0; verify the local and deployment toolchains match before assuming builds are portable.
- No root `README.md` exists, so no README update was made.
- This `CHANGELOG.md` was created after the pushed commits and is currently a local handoff document unless a later agent commits it.

### Ready For Next Agent

- Continue from branch `dev-lpapanthy`, which tracks `origin/dev-lpapanthy`.
- Run verification with:
  - `CC=C:\w64devkit\bin\gcc.exe`
  - workspace-local `GOCACHE` and `GOTMPDIR` if needed
  - `go test -count=1 ./...`
  - `go vet ./...`
  - `go build ./...`
- Use this file for cross-agent continuity: add a new dated section when completing work, finding a failed approach, choosing an architecture direction, or leaving a risk for the next agent.

### Verification

- `go test -count=1 ./...` passed before the push.
- `go vet ./...` passed before the push.
- `go build ./...` passed before the push.
- Manual browser verification passed for the empty-category post error after reloading with the versioned script URL.
