let initialized = false;

// savedTheme tolerates browsers that block storage so system preference still applies.
function savedTheme() {
    try { return localStorage.getItem("theme"); } catch { return null; }
}

// applyTheme synchronizes the CSS theme, browser chrome, and both visible theme controls.
export function applyTheme(theme) {
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

// toggleTheme saves an explicit preference while allowing theme changes when storage is unavailable.
export function toggleTheme() {
    const next = document.documentElement.dataset.theme === "dark" ? "light" : "dark";
    try { localStorage.setItem("theme", next); } catch { /* Storage may be blocked; apply this visit's choice anyway. */ }
    applyTheme(next);
}

// initTheme adopts the pre-paint theme and follows system changes until a saved preference exists.
export function initTheme() {
    if (initialized) return;
    initialized = true;
    const systemDark = window.matchMedia("(prefers-color-scheme: dark)");
    systemDark.addEventListener("change", event => {
        if (!savedTheme()) applyTheme(event.matches ? "dark" : "light");
    });
    applyTheme(document.documentElement.dataset.theme || "light");
    document.querySelectorAll(".theme-toggle").forEach(button => button.addEventListener("click", toggleTheme));
}
