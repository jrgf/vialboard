import {
  elements, escapeHTML, hideError, request, setButtonBusy, showError, showToast, state
} from "./core.js";

export async function loadTeams() {
  state.teams = await request("/teams");
  elements["team-nav-count"].textContent = String(state.teams.length);
  if (!state.teams.some((team) => team.id === state.selectedTeamID)) {
    state.selectedTeamID = state.teams[0]?.id || "";
  }
  renderTeamPicker();
}

export async function loadTeamWorkspace() {
  elements["team-result-copy"].textContent = "Loading teams…";
  try {
    await loadTeams();
    const hasTeams = state.teams.length > 0;
    const canManage = state.session.user.role !== "viewer";
    elements["team-empty"].hidden = hasTeams;
    elements["team-controls"].hidden = !hasTeams;
    elements["team-worker-actions"].hidden = !canManage;
    elements["team-member-actions-head"].hidden = !canManage;
    elements["team-result-copy"].textContent = hasTeams
      ? `${state.teams.length} ${state.teams.length === 1 ? "team" : "teams"} available`
      : "No teams yet";
    if (hasTeams) await loadTeamDetails();
  } catch (error) {
    showToast(error.message);
  }
}

function renderTeamPicker() {
  elements["team-picker"].innerHTML = state.teams.length
    ? state.teams.map((team) => `<option value="${escapeHTML(team.id)}">${escapeHTML(team.name)}</option>`).join("")
    : '<option value="">No teams</option>';
  elements["team-picker"].value = state.selectedTeamID;
}

export async function changeSelectedTeam() {
  state.selectedTeamID = elements["team-picker"].value;
  await loadTeamDetails();
}

async function loadTeamDetails() {
  if (!state.selectedTeamID) return;
  const admin = state.session.user.role === "admin";
  const canManage = state.session.user.role !== "viewer";
  const [members, available, managers] = await Promise.all([
    request(`/teams/${state.selectedTeamID}/members`),
    canManage ? request("/teams/available-users") : Promise.resolve([]),
    admin ? request("/teams/availableManagers") : Promise.resolve([])
  ]);
  state.teamMembers = members;
  state.availableUsers = available;
  state.managers = managers;
  renderTeamMembers();
  renderAvailableUsers();
  renderManagerOptions();
  renderTeamManager();
}

function renderTeamMembers() {
  const canManage = state.session.user.role !== "viewer";
  elements["team-member-list"].innerHTML = state.teamMembers.length
    ? state.teamMembers.map((member) => `
      <article class="team-member-row">
        <div class="user-account">
          <span class="avatar" aria-hidden="true">${escapeHTML(member.username.slice(0, 1).toUpperCase())}</span>
          <span><strong>${escapeHTML(member.username)}</strong><small>${escapeHTML(member.id)}</small></span>
        </div>
        <span class="team-role">${member.role === "manager" ? "Manager" : "Viewer / worker"}</span>
        ${canManage && member.role === "viewer" ? `<button class="row-action danger" type="button" data-remove-team-member="${escapeHTML(member.id)}">Remove</button>` : ""}
      </article>`).join("")
    : '<p class="team-empty-copy">No members belong to this team.</p>';
}

function renderAvailableUsers() {
  // ponytail: client-side matching reuses the loaded directory; move search server-side when its payload becomes measurable.
  elements["available-user-options"].innerHTML = state.availableUsers.map((user) =>
    `<option value="${escapeHTML(user.username)}"></option>`
  ).join("");
  elements["available-user-search"].value = "";
  elements["available-user-search"].disabled = state.availableUsers.length === 0;
  elements["available-user-search"].placeholder = state.availableUsers.length ? "Search worker username" : "No available workers";
  elements["add-team-member-button"].disabled = true;
}

function renderManagerOptions() {
  elements["manager-options"].innerHTML = state.managers.map((manager) =>
    `<option value="${escapeHTML(manager.username)}"></option>`
  ).join("");
}

function renderTeamManager() {
  const admin = state.session.user.role === "admin";
  elements["team-manager-actions"].hidden = !admin;
  if (!admin) return;
  const team = state.teams.find((item) => item.id === state.selectedTeamID);
  const manager = state.managers.find((item) => item.id === team?.managerId);
  elements["team-manager-search"].value = manager?.username || "";
  elements["assign-team-manager-button"].disabled = true;
}

function userByUsername(users, value) {
  const username = value.trim().toLowerCase();
  return users.find((user) => user.username.toLowerCase() === username);
}

export function changeAvailableUserSearch() {
  elements["add-team-member-button"].disabled = !userByUsername(state.availableUsers, elements["available-user-search"].value);
}

export function changeTeamManagerSearch() {
  const manager = userByUsername(state.managers, elements["team-manager-search"].value);
  const team = state.teams.find((item) => item.id === state.selectedTeamID);
  elements["assign-team-manager-button"].disabled = !manager || manager.id === team?.managerId;
}

export async function assignTeamManager() {
  const manager = userByUsername(state.managers, elements["team-manager-search"].value);
  if (!state.selectedTeamID || !manager) return;
  setButtonBusy(elements["assign-team-manager-button"], true, "Assigning…");
  try {
    await request(`/teams/${state.selectedTeamID}`, {
      method: "PATCH",
      body: JSON.stringify({ managerId: manager.id })
    });
    await loadTeamWorkspace();
    showToast(`${manager.username} now manages this team.`);
  } catch (error) {
    showToast(error.message);
  } finally {
    setButtonBusy(elements["assign-team-manager-button"], false, "Assign manager");
    changeTeamManagerSearch();
  }
}

export async function openTeamDialog() {
  elements["team-form"].reset();
  hideError(elements["team-error"]);
  const admin = state.session.user.role === "admin";
  elements["team-manager-label"].hidden = !admin;
  elements["team-manager"].hidden = !admin;
  elements["team-manager"].required = admin;
  elements["team-manager"].disabled = admin;
  elements["team-manager"].value = "";
  elements["team-manager"].placeholder = admin ? "Loading managers…" : "";
  elements["team-dialog"].showModal();
  elements["team-name"].focus();
  if (!admin) return;
  try {
    state.managers = await request("/teams/availableManagers");
    renderManagerOptions();
    elements["team-manager"].disabled = state.managers.length === 0;
    elements["team-manager"].placeholder = state.managers.length ? "Search manager username" : "No active managers";
    if (!state.managers.length) showError(elements["team-error"], "Create an active manager before creating a team.");
  } catch (error) {
    showError(elements["team-error"], error.message);
  }
}

export function closeTeamDialog() {
  elements["team-dialog"].close();
  hideError(elements["team-error"]);
}

export async function createTeam(event) {
  event.preventDefault();
  hideError(elements["team-error"]);
  const manager = state.session.user.role === "admin" ? userByUsername(state.managers, elements["team-manager"].value) : null;
  if (state.session.user.role === "admin" && !manager) {
    showError(elements["team-error"], "Choose an active manager by username.");
    return;
  }
  setButtonBusy(elements["save-team"], true, "Creating…");
  try {
    const team = await request("/teams", {
      method: "POST",
      body: JSON.stringify({
        name: elements["team-name"].value,
        managerId: manager?.id || ""
      })
    });
    state.selectedTeamID = team.id;
    closeTeamDialog();
    await loadTeamWorkspace();
    showToast("Team created.");
  } catch (error) {
    showError(elements["team-error"], error.message);
  } finally {
    setButtonBusy(elements["save-team"], false, "Create team");
  }
}

export function openTeamUserDialog() {
  if (!state.selectedTeamID) return;
  elements["team-user-form"].reset();
  hideError(elements["team-user-error"]);
  elements["team-user-dialog"].showModal();
  elements["team-username"].focus();
}

export function closeTeamUserDialog() {
  elements["team-user-dialog"].close();
  hideError(elements["team-user-error"]);
}

export async function createTeamUser(event) {
  event.preventDefault();
  if (!state.selectedTeamID) return;
  hideError(elements["team-user-error"]);
  setButtonBusy(elements["save-team-user"], true, "Creating…");
  try {
    await request(`/teams/${state.selectedTeamID}/users`, {
      method: "POST",
      body: JSON.stringify({
        username: elements["team-username"].value,
        password: elements["team-password"].value
      })
    });
    closeTeamUserDialog();
    await loadTeamDetails();
    window.dispatchEvent(new Event("members-changed"));
    showToast("Worker created and added to the team.");
  } catch (error) {
    showError(elements["team-user-error"], error.message);
  } finally {
    setButtonBusy(elements["save-team-user"], false, "Create worker");
  }
}

export async function addTeamMember() {
  const user = userByUsername(state.availableUsers, elements["available-user-search"].value);
  if (!state.selectedTeamID || !user) return;
  setButtonBusy(elements["add-team-member-button"], true, "Adding…");
  try {
    await request(`/teams/${state.selectedTeamID}/members/${user.id}`, { method: "PUT" });
    await loadTeamDetails();
    window.dispatchEvent(new Event("members-changed"));
    showToast("Worker added to the team.");
  } catch (error) {
    showToast(error.message);
  } finally {
    setButtonBusy(elements["add-team-member-button"], false, "Add worker");
  }
}

export async function handleTeamMemberAction(event) {
  const button = event.target.closest("[data-remove-team-member]");
  if (!button || !state.selectedTeamID) return;
  const member = state.teamMembers.find((item) => item.id === button.dataset.removeTeamMember);
  if (!member || !window.confirm(`Remove ${member.username} from this team?`)) return;
  button.disabled = true;
  try {
    await request(`/teams/${state.selectedTeamID}/members/${member.id}`, { method: "DELETE" });
    await loadTeamDetails();
    window.dispatchEvent(new Event("members-changed"));
    showToast("Worker removed from the team.");
  } catch (error) {
    button.disabled = false;
    showToast(error.message);
  }
}
