import * as api from "./api.js";
import { el, avatar, timeElement, autoGrow } from "./dom.js";
import { state } from "./state.js";

// loadComments creates each card's reply form once and reloads replies whenever its comments expand.
export async function loadComments(post, section) {
    let list = section.querySelector(".comment-list");
    if (!list) {
        list = el("div", { className: "comment-list" });
        const textarea = el("textarea", { attrs: { placeholder: "Post your reply", rows: "1", required: "" } });
        const status = el("p", { className: "form-status comment-message", attrs: { role: "status" } });
        const form = el("form", { className: "comment-form" }, [
            avatar(state.currentUser.nickname, "avatar-sm"),
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
        const response = await api.getComments(post.id);
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

// addComment preserves reply text and server feedback until a successful submission resets the form.
async function addComment(event, post, section) {
    event.preventDefault();

    const form = event.currentTarget; // Capture before awaiting so the persistent reply form can be reset.
    const textarea = form.querySelector("textarea");
    const status = section.querySelector(".comment-message");

    try {
        const response = await api.createComment({ post_id: post.id, content: textarea.value });
        if (!response.ok) {
            status.textContent = await api.readError(response);
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

// submitOnEnter submits replies on Enter while preserving Shift+Enter and input-method composition.
function submitOnEnter(event) {
    if (event.key === "Enter" && !event.shiftKey && !event.isComposing) {
        event.preventDefault();
        event.currentTarget.form.requestSubmit();
    }
}
