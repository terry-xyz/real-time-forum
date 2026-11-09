// Feed, navigation, profile, and auth share these values; chat lifecycle stays in chat.js.
export const state = {
    currentUser: null,
    currentView: "home",
    allPosts: [],
    categories: [],
    activeCategory: "",
    searchQuery: "",
    unreadUserIds: new Set()
};
