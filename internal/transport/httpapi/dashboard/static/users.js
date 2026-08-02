import {
  clearSession, elements, escapeHTML, formatDate, hideError, pageSize, request,
  setButtonBusy, showError, showToast, state
} from "./core.js";

export async function loadUsers() {
  document.querySelector(".users-card").classList.add("loading");
  elements["user-result-copy"].textContent = "Loading users…";
  try {
    const params = new URLSearchParams({ page: String(state.userPage), pageSize: String(pageSize) });
    const page = await request(`/users?${params}`);
    state.users = page.items;
    state.userTotalPages = Math.max(1, page.pagination.totalPages);
    renderUsers(page.pagination);
  } catch (error) {
    if (state.session) {
      elements["user-list"].replaceChildren();
      elements["user-result-copy"].textContent = error.message;
      showToast(error.message);
    }
  } finally {
    document.querySelector(".users-card").classList.remove("loading");
  }
}

export function renderUsers(pagination) {
  elements["user-nav-count"].textContent = String(pagination.total);
  elements["user-result-copy"].textContent = `${pagination.total} ${pagination.total === 1 ? "account" : "accounts"} in this workspace`;
  elements["user-page-copy"].textContent = `Page ${pagination.page} of ${Math.max(1, pagination.totalPages)}`;
  elements["previous-user-page"].disabled = pagination.page <= 1;
  elements["next-user-page"].disabled = pagination.totalPages === 0 || pagination.page >= pagination.totalPages;
  elements["user-list"].innerHTML = state.users.map((user) => {
    const current = user.id === state.session.user.id;
    const team = state.teams.find((item) => item.id === user.teamId);
    return `
      <article class="user-row">
        <div class="user-account">
          <span class="avatar" aria-hidden="true">${escapeHTML(user.username.slice(0, 1).toUpperCase())}</span>
          <span>
            <strong>${escapeHTML(user.username)}${current ? '<i class="you-badge">You</i>' : ""}</strong>
            <small>${escapeHTML(team ? `${user.id} · ${team.name}` : user.id)}</small>
          </span>
        </div>
        <select class="user-role-select" data-user-role data-id="${escapeHTML(user.id)}" aria-label="Role for ${escapeHTML(user.username)}" ${current ? "disabled" : ""}>
          <option value="viewer" ${user.role === "viewer" ? "selected" : ""}>Viewer / worker</option>
          <option value="manager" ${user.role === "manager" ? "selected" : ""}>Manager</option>
          <option value="admin" ${user.role === "admin" ? "selected" : ""}>Admin</option>
        </select>
        <button class="user-state-button ${user.active ? "" : "inactive"}" type="button" data-user-active data-id="${escapeHTML(user.id)}" ${current ? "disabled" : ""}>
          ${user.active ? "Active" : "Inactive"}
        </button>
        <time class="user-created" datetime="${escapeHTML(user.createdAt)}">${formatDate(user.createdAt)}</time>
        <button class="row-action" type="button" data-user-reset data-id="${escapeHTML(user.id)}" ${current ? "disabled" : ""}>Reset password</button>
      </article>`;
  }).join("");
}

export function changeUserPage(direction) {
  const next = state.userPage + direction;
  if (next < 1 || next > state.userTotalPages) return;
  state.userPage = next;
  loadUsers();
}

export async function changeUserRole(event) {
  const select = event.target.closest("[data-user-role]");
  if (!select) return;
  try {
    await request(`/users/${select.dataset.id}`, {
      method: "PATCH",
      body: JSON.stringify({ role: select.value })
    });
    await loadUsers();
    showToast("User role updated.");
  } catch (error) {
    await loadUsers();
    showToast(error.message);
  }
}

export async function toggleUserAccess(event) {
  const reset = event.target.closest("[data-user-reset]");
  if (reset) {
    const user = state.users.find((item) => item.id === reset.dataset.id);
    if (user && user.id !== state.session.user.id) openPasswordDialog(user);
    return;
  }
  const button = event.target.closest("[data-user-active]");
  if (!button) return;
  const user = state.users.find((item) => item.id === button.dataset.id);
  if (!user || user.id === state.session.user.id) return;
  if (user.active && !window.confirm(`Deactivate ${user.username}? Their current session will be revoked.`)) return;
  button.disabled = true;
  try {
    await request(`/users/${user.id}`, {
      method: "PATCH",
      body: JSON.stringify({ active: !user.active })
    });
    await loadUsers();
    showToast(user.active ? "User deactivated." : "User activated.");
  } catch (error) {
    button.disabled = false;
    showToast(error.message);
  }
}

export function openUserDialog() {
  elements["user-form"].reset();
  hideError(elements["user-error"]);
  elements["user-dialog"].showModal();
  elements["new-username"].focus();
}

export function closeUserDialog() {
  elements["user-dialog"].close();
  hideError(elements["user-error"]);
}

export async function createUser(event) {
  event.preventDefault();
  hideError(elements["user-error"]);
  setButtonBusy(elements["save-user"], true, "Creating…");
  try {
    await request("/users", {
      method: "POST",
      body: JSON.stringify({
        username: elements["new-username"].value,
        password: elements["new-password"].value,
        role: elements["new-user-role"].value
      })
    });
    closeUserDialog();
    state.userPage = 1;
    await loadUsers();
    showToast("User created.");
  } catch (error) {
    showError(elements["user-error"], error.message);
  } finally {
    setButtonBusy(elements["save-user"], false, "Create user");
  }
}

export function openPasswordDialog(user = null) {
  state.passwordUserID = user?.id || null;
  const resetting = state.passwordUserID !== null;
  elements["password-form"].reset();
  hideError(elements["password-error"]);
  elements["password-kicker"].textContent = resetting ? "User security" : "Account security";
  elements["password-title"].textContent = resetting ? `Reset ${user.username}'s password` : "Change password";
  elements["current-password-label"].hidden = resetting;
  elements["current-password"].hidden = resetting;
  elements["current-password"].required = !resetting;
  elements["save-password"].textContent = resetting ? "Reset password" : "Change password";
  elements["password-dialog"].showModal();
  (resetting ? elements["replacement-password"] : elements["current-password"]).focus();
}

export function closePasswordDialog() {
  elements["password-dialog"].close();
  state.passwordUserID = null;
  hideError(elements["password-error"]);
}

export async function savePassword(event) {
  event.preventDefault();
  hideError(elements["password-error"]);
  if (elements["replacement-password"].value !== elements["replacement-password-confirmation"].value) {
    showError(elements["password-error"], "Passwords do not match.");
    return;
  }
  const resetting = state.passwordUserID !== null;
  const userID = state.passwordUserID;
  setButtonBusy(elements["save-password"], true, resetting ? "Resetting…" : "Changing…");
  try {
    await request(resetting ? `/users/${userID}/password` : "/account/password", {
      method: "PATCH",
      body: JSON.stringify(resetting
        ? { newPassword: elements["replacement-password"].value, passwordConfirmation: elements["replacement-password-confirmation"].value }
        : { currentPassword: elements["current-password"].value, newPassword: elements["replacement-password"].value, passwordConfirmation: elements["replacement-password-confirmation"].value })
    });
    closePasswordDialog();
    if (resetting) {
      showToast("Password reset. Existing sessions were revoked.");
      } else {
        clearSession();
        window.dispatchEvent(new Event("session-expired"));
        showToast("Password changed. Sign in again.");
    }
  } catch (error) {
    showError(elements["password-error"], error.message);
  } finally {
    setButtonBusy(elements["save-password"], false, resetting ? "Reset password" : "Change password");
  }
}
