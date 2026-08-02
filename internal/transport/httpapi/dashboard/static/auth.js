import {
  clearSession, elements, hideError, request, saveSession, setButtonBusy, showError, showToast, state
} from "./core.js";

export function setAuthMode(mode) {
  state.authMode = mode === "register" ? "register" : "login";
  const registering = state.authMode === "register";
  document.querySelectorAll("[data-auth-mode]").forEach((button) => {
    const active = button.dataset.authMode === state.authMode;
    button.classList.toggle("active", active);
    button.setAttribute("aria-selected", String(active));
  });
  elements["auth-title"].textContent = registering ? "Create your account" : "Sign in to your workspace";
  elements["auth-copy"].textContent = registering
    ? "Start with a viewer account. An admin can change access later."
    : "Use your account to continue to the issue board.";
  elements["auth-submit"].textContent = registering ? "Create account" : "Sign in";
  elements.password.autocomplete = registering ? "new-password" : "current-password";
  elements["password-confirmation-label"].hidden = !registering;
  elements["password-confirmation"].hidden = !registering;
  elements["password-confirmation"].required = registering;
  if (!registering) elements["password-confirmation"].value = "";
  hideError(elements["auth-error"]);
}

export async function submitAuth(event) {
  event.preventDefault();
  hideError(elements["auth-error"]);
  const registering = state.authMode === "register";
  if (registering && elements.password.value !== elements["password-confirmation"].value) {
    showError(elements["auth-error"], "Passwords do not match.");
    return;
  }
  setButtonBusy(elements["auth-submit"], true, state.authMode === "register" ? "Creating…" : "Signing in…");
  try {
    const body = { username: elements.username.value, password: elements.password.value };
    if (registering) body.passwordConfirmation = elements["password-confirmation"].value;
    const session = await request(state.authMode === "register" ? "/register" : "/login", {
      method: "POST",
      body: JSON.stringify(body),
      authenticated: false
    });
    saveSession(session);
    elements["auth-form"].reset();
    showDashboard();
    window.dispatchEvent(new Event("authenticated"));
    showToast(state.authMode === "register" ? "Account created. Welcome to Vialboard." : "Welcome back.");
  } catch (error) {
    showError(elements["auth-error"], error.message);
  } finally {
    setButtonBusy(elements["auth-submit"], false, state.authMode === "register" ? "Create account" : "Sign in");
  }
}

export async function logout() {
  try {
    await request("/logout", { method: "POST" });
  } catch (error) {
    if (state.session) showToast(error.message);
  } finally {
    clearSession();
    showAuth();
  }
}

export function showAuth() {
  elements["app-view"].hidden = true;
  elements["auth-view"].hidden = false;
  setAuthMode("login");
  window.setTimeout(() => elements.username.focus(), 0);
}

export function showDashboard() {
  elements["auth-view"].hidden = true;
  elements["app-view"].hidden = false;
  const user = state.session.user;
  elements["user-name"].textContent = user.username;
  elements["user-role"].textContent = user.role;
  elements["user-avatar"].textContent = user.username.slice(0, 1).toUpperCase();
  elements["users-nav"].hidden = user.role !== "admin";
  elements["teams-nav"].hidden = false;
  elements["new-team-button"].hidden = user.role === "viewer";
  elements["team-workspace-title"].textContent = user.role === "viewer" ? "Your team" : "Team management";
  elements["team-workspace-copy"].textContent = user.role === "viewer"
    ? "View your team and its members."
    : "Create teams, assign workers, and manage team membership.";
  elements["new-issue-button"].hidden = user.role === "viewer";
  document.querySelectorAll("[data-create-issue]").forEach((button) => { button.hidden = user.role === "viewer"; });
  elements["board-copy"].textContent = user.role === "admin"
    ? "All workspace issues, ordered and visible."
    : user.role === "manager" ? "Issues across the teams you manage." : "Your assigned work, ordered and visible.";
}
