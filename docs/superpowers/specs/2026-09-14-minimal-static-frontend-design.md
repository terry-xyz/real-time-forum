# Minimal Static Frontend Design

## Scope

Create only a plain `static/index.html` file. It is a visual HTML scaffold for the backend functionality that already exists: registration, login, logout, posts, categories, and comments.

## Deliberate non-goals

- No JavaScript or API wiring.
- No CSS or visual styling beyond semantic HTML.
- No chat, private messages, WebSockets, online presence, or pagination.
- No backend changes.

## Page structure

The page will contain:

1. A header with the forum title and a logout form.
2. A registration form containing nickname, age, gender, first name, last name, email, and password inputs.
3. A login form accepting nickname or email plus password.
4. A post-creation form containing title, content, and category inputs.
5. A category display/filter area.
6. A posts feed area with representative post markup and a comment form for each displayed post.
7. A comments display area showing author, date, and content fields.

Forms will use descriptive labels, native input types, `required` attributes for required fields, and clear section headings. Since the existing handlers expect JSON rather than normal form submissions, the forms are intentionally display/input scaffolding and will not submit successfully until JavaScript is added later.

## Acceptance criteria

- `static/index.html` exists and is served by the existing file server.
- A user can see all current auth, post, category, and comment fields without browser developer tools.
- The markup remains dependency-free and uses one HTML document.
- No chat or unimplemented functionality is presented as available.
