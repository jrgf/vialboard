import { elements, state } from "./core.js";
import { logout, setAuthMode, showAuth, showDashboard, submitAuth } from "./auth.js";
import {
  addComment, changeIssueTeam, changePage, closeIssueDialog, handleIssueAction,
  loadBoard, loadMembers, openCreateDialog, openIssueByID, saveIssue
} from "./issues.js";
import { cancelIssueExport, startIssueExport, stopIssueExport } from "./exports.js";
import {
  changeUserPage, changeUserRole, closePasswordDialog, closeUserDialog, createUser, loadUsers,
  openPasswordDialog, openUserDialog, savePassword, toggleUserAccess
} from "./users.js";
import {
  addTeamMember, assignTeamManager, changeAvailableUserSearch, changeSelectedTeam, changeTeamManagerSearch,
  closeTeamDialog, closeTeamUserDialog, createTeam, createTeamUser, handleTeamMemberAction, loadTeams,
  loadTeamWorkspace, openTeamDialog, openTeamUserDialog
} from "./teams.js";
import {
  handleNotificationClick, markAllNotificationsRead, startNotifications, stopNotifications
} from "./notifications.js";

document.querySelectorAll("[data-auth-mode]").forEach((button) => {
  button.addEventListener("click", () => setAuthMode(button.dataset.authMode));
});
document.querySelectorAll("[data-create-issue]").forEach((button) => button.addEventListener("click", openCreateDialog));
elements["auth-form"].addEventListener("submit", submitAuth);
elements["logout-button"].addEventListener("click", logout);
elements["change-password-button"].addEventListener("click", () => openPasswordDialog());
elements["issues-nav"].addEventListener("click", () => showWorkspace("issues"));
elements["teams-nav"].addEventListener("click", () => showWorkspace("teams"));
elements["users-nav"].addEventListener("click", () => showWorkspace("users"));
elements["notifications-nav"].addEventListener("click", () => showWorkspace("notifications"));
elements["new-issue-button"].addEventListener("click", openCreateDialog);
elements["export-issues-button"].addEventListener("click", startIssueExport);
elements["cancel-export-button"].addEventListener("click", cancelIssueExport);
elements["new-user-button"].addEventListener("click", openUserDialog);
elements["new-team-button"].addEventListener("click", openTeamDialog);
elements["new-team-user-button"].addEventListener("click", openTeamUserDialog);
elements["issue-team"].addEventListener("change", changeIssueTeam);
elements["team-picker"].addEventListener("change", changeSelectedTeam);
elements["add-team-member-button"].addEventListener("click", addTeamMember);
elements["available-user-search"].addEventListener("input", changeAvailableUserSearch);
elements["team-manager-search"].addEventListener("input", changeTeamManagerSearch);
elements["assign-team-manager-button"].addEventListener("click", assignTeamManager);
elements["team-member-list"].addEventListener("click", handleTeamMemberAction);
elements["notification-list"].addEventListener("click", handleNotificationClick);
elements["mark-all-notifications"].addEventListener("click", markAllNotificationsRead);
elements["status-filter"].addEventListener("change", () => {
  state.status = elements["status-filter"].value;
  state.page = 1;
  loadBoard();
});
elements["priority-filter"].addEventListener("change", () => {
  state.priority = elements["priority-filter"].value;
  state.page = 1;
  loadBoard();
});
elements["sort-filter"].addEventListener("change", () => {
  [state.sort, state.order] = elements["sort-filter"].value.split(":");
  state.page = 1;
  loadBoard();
});
elements["search-input"].addEventListener("input", () => {
  window.clearTimeout(loadBoard.searchTimer);
  loadBoard.searchTimer = window.setTimeout(() => {
    state.search = elements["search-input"].value.trim();
    state.page = 1;
    loadBoard();
  }, 250);
});
elements["previous-page"].addEventListener("click", () => changePage(-1));
elements["next-page"].addEventListener("click", () => changePage(1));
elements["previous-user-page"].addEventListener("click", () => changeUserPage(-1));
elements["next-user-page"].addEventListener("click", () => changeUserPage(1));
elements["close-dialog"].addEventListener("click", closeIssueDialog);
elements["cancel-dialog"].addEventListener("click", closeIssueDialog);
elements["issue-form"].addEventListener("submit", saveIssue);
elements["comment-form"].addEventListener("submit", addComment);
elements["issue-list"].addEventListener("click", handleIssueAction);
elements["user-list"].addEventListener("change", changeUserRole);
elements["user-list"].addEventListener("click", toggleUserAccess);
elements["user-form"].addEventListener("submit", createUser);
elements["close-user-dialog"].addEventListener("click", closeUserDialog);
elements["cancel-user-dialog"].addEventListener("click", closeUserDialog);
elements["team-form"].addEventListener("submit", createTeam);
elements["close-team-dialog"].addEventListener("click", closeTeamDialog);
elements["cancel-team-dialog"].addEventListener("click", closeTeamDialog);
elements["team-user-form"].addEventListener("submit", createTeamUser);
elements["close-team-user-dialog"].addEventListener("click", closeTeamUserDialog);
elements["cancel-team-user-dialog"].addEventListener("click", closeTeamUserDialog);
elements["password-form"].addEventListener("submit", savePassword);
elements["close-password-dialog"].addEventListener("click", closePasswordDialog);
elements["cancel-password-dialog"].addEventListener("click", closePasswordDialog);
elements["issue-dialog"].addEventListener("click", (event) => {
  if (event.target === elements["issue-dialog"]) closeIssueDialog();
});
elements["user-dialog"].addEventListener("click", (event) => {
  if (event.target === elements["user-dialog"]) closeUserDialog();
});
elements["password-dialog"].addEventListener("click", (event) => {
  if (event.target === elements["password-dialog"]) closePasswordDialog();
});
elements["team-dialog"].addEventListener("click", (event) => {
  if (event.target === elements["team-dialog"]) closeTeamDialog();
});
elements["team-user-dialog"].addEventListener("click", (event) => {
  if (event.target === elements["team-user-dialog"]) closeTeamUserDialog();
});

function initializeDashboard() {
  startNotifications();
  showWorkspace("issues");
  if (state.session.user.role === "admin") loadUsers();
}

window.addEventListener("authenticated", initializeDashboard);
window.addEventListener("session-expired", showAuth);
window.addEventListener("members-changed", loadMembers);
window.addEventListener("notification-navigation", async (event) => {
  if (event.detail.issueId) {
    await showWorkspace("issues");
    await openIssueByID(event.detail.issueId);
    return;
  }
  if (event.detail.teamId) {
    state.selectedTeamID = event.detail.teamId;
    await showWorkspace("teams");
  }
});
window.addEventListener("pagehide", stopNotifications);
window.addEventListener("pagehide", stopIssueExport);

if (state.session) {
  showDashboard();
  initializeDashboard();
} else {
  showAuth();
}

export async function showWorkspace(view) {
  const users = view === "users" && state.session.user.role === "admin";
  const teams = view === "teams";
  const notifications = view === "notifications";
  const issues = !users && !teams && !notifications;
  elements["issues-workspace"].hidden = !issues;
  elements["teams-workspace"].hidden = !teams;
  elements["users-workspace"].hidden = !users;
  elements["notifications-workspace"].hidden = !notifications;
  elements["issues-nav"].classList.toggle("active", issues);
  elements["teams-nav"].classList.toggle("active", teams);
  elements["users-nav"].classList.toggle("active", users);
  elements["notifications-nav"].classList.toggle("active", notifications);
  elements["issues-nav"].toggleAttribute("aria-current", issues);
  elements["teams-nav"].toggleAttribute("aria-current", teams);
  elements["users-nav"].toggleAttribute("aria-current", users);
  elements["notifications-nav"].toggleAttribute("aria-current", notifications);
  if (notifications) return;
  if (users) return loadUsers();
  if (teams) return loadTeamWorkspace();
  await loadTeams();
  await loadMembers();
  return loadBoard();
}
