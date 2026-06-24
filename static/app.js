const state = {
  user: null,
  users: [],
  posts: [],
  categories: [],
  activeChatUser: null,
  unreadUserIDs: new Set(),
  oldestMessageId: null,
  loadingOlder: false,
  socket: null,
};

const $ = (selector) => document.querySelector(selector);

async function api(path, options = {}) {
  const response = await fetch(path, {
    credentials: 'same-origin',
    headers: { 'Content-Type': 'application/json', ...(options.headers || {}) },
    ...options,
  });
  const data = await response.json();
  if (!response.ok) throw new Error(data.error || 'Request failed');
  return data;
}

function formJSON(form) {
  return Object.fromEntries(new FormData(form).entries());
}

function controlValue(form, name) {
  return form.elements.namedItem(name).value;
}

function messageInput() {
  return $('#message-form').elements.namedItem('content');
}

function showAuth(showRegister) {
  $('#login-form').classList.toggle('hidden', showRegister);
  $('#register-form').classList.toggle('hidden', !showRegister);
  $('#show-login').classList.toggle('active', !showRegister);
  $('#show-register').classList.toggle('active', showRegister);
  clearAuthError();
}

function clearAuthError() {
  $('#auth-error').textContent = '';
}

function clearPostError() {
  $('#post-error').textContent = '';
}

function renderShell() {
  $('#auth-view').classList.toggle('hidden', Boolean(state.user));
  $('#app-view').classList.toggle('hidden', !state.user);
  $('#current-user').textContent = state.user ? state.user.nickname : '';
}

async function loadInitialData() {
  const [categories, posts, users] = await Promise.all([
    api('/api/categories'),
    api('/api/posts'),
    api('/api/users'),
  ]);
  state.categories = categories.categories || [];
  state.posts = posts.posts || [];
  state.users = users.users || [];
  renderCategories();
  renderPosts();
  renderUsers();
}

function renderCategories() {
  $('#category-list').innerHTML = state.categories.map((category) => `
    <label><input type="checkbox" name="category_ids" value="${category.id}"> ${escapeHTML(category.name)}</label>
  `).join('');
}

function renderPosts() {
  $('#posts').innerHTML = state.posts.map((post) => `
    <article class="post" data-post-id="${post.id}">
      <header>
        <h2>${escapeHTML(post.title)}</h2>
        <span>${escapeHTML(post.author.nickname)} · ${formatDate(post.created_at)}</span>
      </header>
      <p>${escapeHTML(post.content)}</p>
      <small>${post.categories.map(escapeHTML).join(', ')}</small>
      <button type="button" data-action="toggle-comments">Comments</button>
      <section class="comments hidden"></section>
      <form class="comment-form hidden">
        <input name="content" placeholder="Write a comment" required>
        <button type="submit">Comment</button>
      </form>
    </article>
  `).join('');
}

function renderUsers() {
  $('#user-list').innerHTML = state.users.map((user) => `
    <li>
      <button type="button" data-user-id="${user.id}" class="${state.activeChatUser?.id === user.id ? 'active' : ''}">
        <span>${escapeHTML(user.nickname)}</span>
        <span class="${user.online ? 'online' : 'offline'}">${state.unreadUserIDs.has(user.id) ? 'new' : (user.online ? 'online' : 'offline')}</span>
      </button>
    </li>
  `).join('');
}

async function openChat(userID) {
  state.activeChatUser = state.users.find((user) => user.id === userID);
  state.unreadUserIDs.delete(userID);
  state.oldestMessageId = null;
  $('#chat-title').textContent = state.activeChatUser.nickname;
  messageInput().disabled = false;
  $('#message-form button').disabled = state.socket?.readyState !== WebSocket.OPEN;
  const data = await api(`/api/messages?with=${userID}`);
  renderMessages(data.messages || [], false);
  renderUsers();
}

function renderMessages(messages, prepend) {
  if (messages.length > 0) state.oldestMessageId = messages[0].id;
  const html = messages.map(messageHTML).join('');
  if (prepend) {
    $('#messages').insertAdjacentHTML('afterbegin', html);
  } else {
    $('#messages').innerHTML = html;
    $('#messages').scrollTop = $('#messages').scrollHeight;
  }
}

function messageHTML(message) {
  return `
    <div class="message">
      <strong>${escapeHTML(message.from.nickname)}</strong>
      <time>${formatDate(message.created_at)}</time>
      <p>${escapeHTML(message.content)}</p>
    </div>
  `;
}

async function loadOlderMessages() {
  if (!state.activeChatUser || !state.oldestMessageId || state.loadingOlder) return;
  state.loadingOlder = true;
  try {
    const data = await api(`/api/messages?with=${state.activeChatUser.id}&before_id=${state.oldestMessageId}`);
    renderMessages(data.messages || [], true);
  } finally {
    state.loadingOlder = false;
  }
}

function connectSocket() {
  if (state.socket) state.socket.close();
  const protocol = location.protocol === 'https:' ? 'wss' : 'ws';
  state.socket = new WebSocket(`${protocol}://${location.host}/ws`);
  state.socket.onopen = () => {
    if (state.activeChatUser) $('#message-form button').disabled = false;
  };
  state.socket.onmessage = async (event) => {
    const payload = JSON.parse(event.data);
    if (payload.type !== 'private_message') return;
    if (state.activeChatUser && (payload.from.id === state.activeChatUser.id || payload.to.id === state.activeChatUser.id)) {
      $('#messages').insertAdjacentHTML('beforeend', messageHTML(payload.message));
      $('#messages').scrollTop = $('#messages').scrollHeight;
    } else if (payload.from.id !== state.user.id) {
      state.unreadUserIDs.add(payload.from.id);
    }
    const users = await api('/api/users');
    state.users = users.users;
    renderUsers();
  };
}

function throttle(fn, wait) {
  let last = 0;
  return (...args) => {
    const now = Date.now();
    if (now - last >= wait) {
      last = now;
      fn(...args);
    }
  };
}

function escapeHTML(value) {
  return String(value).replace(/[&<>"']/g, (char) => ({
    '&': '&amp;',
    '<': '&lt;',
    '>': '&gt;',
    '"': '&quot;',
    "'": '&#39;',
  }[char]));
}

function formatDate(value) {
  return new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value));
}

async function boot() {
  try {
    const me = await api('/api/me');
    state.user = me.user;
    renderShell();
    await loadInitialData();
    connectSocket();
  } catch {
    state.user = null;
    renderShell();
  }
}

$('#show-login').addEventListener('click', () => showAuth(false));
$('#show-register').addEventListener('click', () => showAuth(true));
$('#auth-view').addEventListener('input', clearAuthError);
$('#auth-view').addEventListener('change', clearAuthError);
$('#post-form').addEventListener('input', clearPostError);
$('#post-form').addEventListener('change', clearPostError);

$('#register-form').addEventListener('submit', async (event) => {
  event.preventDefault();
  clearAuthError();
  const data = formJSON(event.currentTarget);
  data.age = Number(data.age);
  try {
    await api('/api/register', { method: 'POST', body: JSON.stringify(data) });
    showAuth(false);
  } catch (error) {
    $('#auth-error').textContent = error.message;
  }
});

$('#login-form').addEventListener('submit', async (event) => {
  event.preventDefault();
  clearAuthError();
  try {
    const data = await api('/api/login', { method: 'POST', body: JSON.stringify(formJSON(event.currentTarget)) });
    state.user = data.user;
    renderShell();
    await loadInitialData();
    connectSocket();
  } catch (error) {
    $('#auth-error').textContent = error.message;
  }
});

$('#logout').addEventListener('click', async () => {
  await api('/api/logout', { method: 'POST', body: '{}' });
  state.user = null;
  renderShell();
});

$('#post-form').addEventListener('submit', async (event) => {
  event.preventDefault();
  clearPostError();
  const form = event.currentTarget;
  const categoryIDs = [...form.querySelectorAll('input[name="category_ids"]:checked')].map((input) => Number(input.value));
  if (categoryIDs.length === 0) {
    $('#post-error').textContent = 'Please select at least one category';
    return;
  }
  try {
    const data = await api('/api/posts', {
      method: 'POST',
      body: JSON.stringify({ title: controlValue(form, 'title'), content: controlValue(form, 'content'), category_ids: categoryIDs }),
    });
    state.posts.unshift(data.post);
    form.reset();
    renderPosts();
  } catch (error) {
    $('#post-error').textContent = error.message;
  }
});

$('#posts').addEventListener('click', async (event) => {
  if (event.target.dataset.action !== 'toggle-comments') return;
  const postEl = event.target.closest('.post');
  const commentsEl = postEl.querySelector('.comments');
  const form = postEl.querySelector('.comment-form');
  commentsEl.classList.toggle('hidden');
  form.classList.toggle('hidden');
  if (!commentsEl.dataset.loaded) {
    const data = await api(`/api/posts/${postEl.dataset.postId}/comments`);
    commentsEl.innerHTML = data.comments.map((comment) => `<p><strong>${escapeHTML(comment.author.nickname)}</strong> ${escapeHTML(comment.content)}</p>`).join('');
    commentsEl.dataset.loaded = 'true';
  }
});

$('#posts').addEventListener('submit', async (event) => {
  if (!event.target.classList.contains('comment-form')) return;
  event.preventDefault();
  const postEl = event.target.closest('.post');
  const data = await api(`/api/posts/${postEl.dataset.postId}/comments`, {
    method: 'POST',
    body: JSON.stringify({ content: event.target.content.value }),
  });
  postEl.querySelector('.comments').insertAdjacentHTML('beforeend', `<p><strong>${escapeHTML(data.comment.author.nickname)}</strong> ${escapeHTML(data.comment.content)}</p>`);
  event.target.reset();
});

$('#user-list').addEventListener('click', (event) => {
  const button = event.target.closest('button[data-user-id]');
  if (button) openChat(Number(button.dataset.userId));
});

$('#messages').addEventListener('scroll', throttle(() => {
  if ($('#messages').scrollTop < 30) loadOlderMessages();
}, 700));

$('#message-form').addEventListener('submit', (event) => {
  event.preventDefault();
  const input = messageInput();
  if (!state.activeChatUser || !input.value.trim() || state.socket?.readyState !== WebSocket.OPEN) return;
  state.socket.send(JSON.stringify({ type: 'private_message', to_id: state.activeChatUser.id, content: input.value }));
  input.value = '';
});

boot();
