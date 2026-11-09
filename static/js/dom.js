const AVATAR_COLORS = ["#1d9bf0", "#7856ff", "#f91880", "#00ba7c", "#ff7a00", "#536471", "#e0245e", "#17bf63"];

// $ resolves the existing page IDs shared by the feature modules.
export function $(id) {
    return document.getElementById(id);
}

// el builds DOM nodes with textContent so API and user text cannot become HTML markup.
export function el(tag, options = {}, children = []) {
    const node = document.createElement(tag);
    if (options.className) node.className = options.className;
    if (options.text !== undefined) node.textContent = options.text;
    if (options.attrs) {
        for (const [name, value] of Object.entries(options.attrs)) {
            node.setAttribute(name, value);
        }
    }
    for (const child of children) {
        if (child) node.append(child);
    }
    return node;
}

// icon references the page's SVG sprite so all features use the existing UI icons.
export function icon(name) {
    const svg = document.createElementNS("http://www.w3.org/2000/svg", "svg");
    svg.setAttribute("aria-hidden", "true");
    const use = document.createElementNS("http://www.w3.org/2000/svg", "use");
    use.setAttribute("href", `#icon-${name}`);
    svg.append(use);
    return svg;
}

// paintAvatar gives a name the same initial and color wherever the user appears.
export function paintAvatar(node, name) {
    const label = (name || "?").trim();
    let hash = 0;
    // Match the UI's unsigned hash so extracting the helper keeps each person's color.
    for (const char of label) hash = (hash * 31 + char.codePointAt(0)) >>> 0;
    node.textContent = [...label][0] || "?";
    node.style.setProperty("--avatar-color", AVATAR_COLORS[hash % AVATAR_COLORS.length]);
    node.setAttribute("aria-hidden", "true");
    return node;
}

// avatar creates a colored initial with optional size classes used by posts and replies.
export function avatar(name, extraClass = "") {
    return paintAvatar(el("span", { className: `avatar ${extraClass}`.trim() }), name);
}

// relativeTime retains the UI's compact timestamps and locale-aware dates for older posts.
export function relativeTime(value) {
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

// timeElement pairs the compact visible timestamp with its full date and native tooltip.
export function timeElement(value) {
    const time = el("time", { text: relativeTime(value) });
    time.dateTime = value;
    time.title = new Date(value).toLocaleString();
    return time;
}

// autoGrow resets the textarea height before measuring so deleting text also shrinks the composer.
export function autoGrow(textarea) {
    textarea.style.height = "auto";
    textarea.style.height = `${textarea.scrollHeight}px`;
}

let toastTimeout = null;

// showToast replaces the shared feedback and gives each new message its full display duration.
export function showToast(text) {
    const toast = $("toast");
    toast.textContent = text;
    toast.hidden = false;
    clearTimeout(toastTimeout);
    toastTimeout = setTimeout(() => { toast.hidden = true; }, 3500);
}
