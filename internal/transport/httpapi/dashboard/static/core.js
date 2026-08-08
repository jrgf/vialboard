const sessionKey = "vialboard.session";
export const pageSize = 8;
export const elements = Object.fromEntries([
  "auth-view", "app-view", "auth-form", "auth-title", "auth-copy", "auth-error", "auth-submit",
  "username", "password", "password-confirmation-label", "password-confirmation", "logout-button", "change-password-button", "user-name", "user-role", "user-avatar", "board-copy",
  "new-issue-button", "export-issues-button", "export-status", "export-copy", "export-progress", "cancel-export-button",
  "search-input", "status-filter", "priority-filter", "sort-filter", "issue-list", "empty-state", "result-copy", "total-count",
  "open-count", "closed-count", "nav-count", "sidebar-pulse", "sidebar-pulse-copy", "page-copy",
  "previous-page", "next-page", "issue-dialog", "issue-form", "dialog-kicker", "dialog-title",
  "close-dialog", "cancel-dialog", "issue-title", "issue-description", "issue-status", "issue-priority",
  "issue-due-date", "issue-assignee", "activity-panel", "comment-form", "comment-body", "save-comment",
  "activity-list", "issue-error",
  "save-issue", "issues-nav", "teams-nav", "users-nav", "issues-workspace", "teams-workspace", "users-workspace",
  "team-nav-count", "new-team-button", "team-workspace-title", "team-workspace-copy", "team-result-copy", "team-picker", "team-empty", "team-controls",
  "available-user-search", "available-user-options", "add-team-member-button", "new-team-user-button", "team-member-list",
  "new-user-button", "user-list", "user-nav-count", "user-result-copy", "user-page-copy", "previous-user-page",
  "next-user-page", "user-dialog", "user-form", "close-user-dialog", "cancel-user-dialog",
  "new-username", "new-password", "new-user-role", "user-error", "save-user", "password-dialog",
  "password-form", "password-kicker", "password-title", "current-password-label", "current-password",
  "replacement-password", "replacement-password-confirmation", "password-error", "close-password-dialog", "cancel-password-dialog",
  "save-password", "team-dialog", "team-form", "close-team-dialog", "cancel-team-dialog", "team-name",
  "team-manager-label", "team-manager", "manager-options", "team-manager-actions", "team-manager-search", "assign-team-manager-button",
  "team-worker-actions", "team-member-actions-head",
  "team-error", "save-team", "team-user-dialog", "team-user-form", "close-team-user-dialog",
  "cancel-team-user-dialog", "team-username", "team-password", "team-user-error", "save-team-user",
  "issue-team", "notifications-nav", "notification-nav-count", "notifications-workspace",
  "notification-list", "notification-empty", "mark-all-notifications", "toast"
].map((id) => [id, document.getElementById(id)]));

export const state = {
  authMode: "login",
  session: readSession(),
  issues: [],
  users: [],
  members: [],
  teams: [],
  managers: [],
  teamMembers: [],
  availableUsers: [],
  notifications: [],
  notificationUnread: 0,
  notificationAbort: null,
  notificationReconnectTimer: null,
  issueExport: null,
  issueExportAbort: null,
  issueExportPollTimer: null,
  issueExportIdempotencyKey: null,
  issueExportCompletionHandled: false,
  selectedTeamID: "",
  editingID: null,
  passwordUserID: null,
  page: 1,
  totalPages: 1,
  userPage: 1,
  userTotalPages: 1,
  status: "",
  priority: "",
  search: "",
  sort: "createdAt",
  order: "desc"
};

export function readSession() {
  try {
    const stored = localStorage.getItem(sessionKey) || sessionStorage.getItem(sessionKey);
    const session = JSON.parse(stored);
    if (!session?.token || !session?.user?.username) return null;
    if (session.expiresAt && Date.parse(session.expiresAt) <= Date.now()) {
      removeStoredSession();
      return null;
    }
    localStorage.setItem(sessionKey, JSON.stringify(session));
    sessionStorage.removeItem(sessionKey);
    return session;
  } catch {
    removeStoredSession();
    return null;
  }
}

export function saveSession(session) {
  state.session = session;
  localStorage.setItem(sessionKey, JSON.stringify(session));
  sessionStorage.removeItem(sessionKey);
}

export function clearSession() {
	state.notificationAbort?.abort();
	window.clearTimeout(state.notificationReconnectTimer);
	state.issueExportAbort?.abort();
	window.clearTimeout(state.issueExportPollTimer);
	state.session = null;
  state.issues = [];
  state.users = [];
  state.members = [];
  state.teams = [];
  state.managers = [];
  state.teamMembers = [];
  state.availableUsers = [];
  state.notifications = [];
  state.notificationUnread = 0;
  state.notificationAbort = null;
  state.notificationReconnectTimer = null;
  state.issueExport = null;
  state.issueExportAbort = null;
  state.issueExportPollTimer = null;
  state.issueExportIdempotencyKey = null;
  state.issueExportCompletionHandled = false;
  elements["export-status"].hidden = true;
  elements["export-issues-button"].disabled = false;
  elements["export-issues-button"].textContent = "Export CSV";
  state.selectedTeamID = "";
  removeStoredSession();
}

export function removeStoredSession() {
  try { localStorage.removeItem(sessionKey); } catch {}
  try { sessionStorage.removeItem(sessionKey); } catch {}
}

export async function request(path, options = {}) {
  const headers = { Accept: "application/json", ...(options.headers || {}) };
  if (options.body) headers["Content-Type"] = "application/json";
  if (options.authenticated !== false && state.session?.token) {
    headers.Authorization = `Bearer ${state.session.token}`;
  }
  const response = await fetch(path, { ...options, headers });
  if (response.status === 204) return null;
  const payload = await response.json().catch(() => null);
  if (!response.ok) {
    if (response.status === 401 && options.authenticated !== false) {
      clearSession();
      window.dispatchEvent(new Event("session-expired"));
    }
    throw new Error(payload?.message || payload?.error?.message || payload?.error || "The request could not be completed.");
  }
  return payload;
}

export function setButtonBusy(button, busy, label) {
  button.disabled = busy;
  button.textContent = label;
}

export function showError(element, message) {
  element.textContent = message;
  element.hidden = false;
}

export function hideError(element) {
  element.textContent = "";
  element.hidden = true;
}

export function showToast(message) {
  elements.toast.textContent = message;
  elements.toast.hidden = false;
  window.clearTimeout(showToast.timer);
  showToast.timer = window.setTimeout(() => { elements.toast.hidden = true; }, 3200);
}

export function formatDate(value) {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? "—" : new Intl.DateTimeFormat(undefined, { month: "short", day: "numeric" }).format(date);
}

export function formatDateOnly(value) {
  return formatDate(`${value}T00:00:00`);
}

export function formatDateTime(value) {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? "—" : new Intl.DateTimeFormat(undefined, {
    month: "short", day: "numeric", hour: "numeric", minute: "2-digit"
  }).format(date);
}

export function escapeHTML(value) {
  const span = document.createElement("span");
  span.textContent = String(value ?? "");
  return span.innerHTML;
}
