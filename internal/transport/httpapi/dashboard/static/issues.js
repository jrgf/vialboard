import {
  elements, escapeHTML, formatDateOnly, formatDateTime, hideError, pageSize, request,
  setButtonBusy, showError, showToast, state
} from "./core.js";

export async function loadMembers() {
  try {
    state.members = await request("/members");
  } catch (error) {
    state.members = [];
    if (state.session) showToast(error.message);
  }
  renderMembers("", elements["issue-team"].value);
}

export function renderMembers(selected = "", teamID = "") {
  const members = state.members.filter((member) => (member.teamId || "") === teamID);
  elements["issue-assignee"].innerHTML = '<option value="">Unassigned</option>' + members.map((member) =>
    `<option value="${escapeHTML(member.id)}">${escapeHTML(member.username)}</option>`
  ).join("");
  elements["issue-assignee"].value = selected || "";
}

export function renderTeams(selected = "") {
  elements["issue-team"].innerHTML = '<option value="">No team</option>' + state.teams.map((team) =>
    `<option value="${escapeHTML(team.id)}">${escapeHTML(team.name)}</option>`
  ).join("");
  elements["issue-team"].value = selected || "";
}

export function changeIssueTeam() {
  renderMembers("", elements["issue-team"].value);
}

export async function loadBoard() {
  document.querySelector(".board-card").classList.add("loading");
  elements["result-copy"].textContent = "Loading workspace…";
  try {
    const listParams = new URLSearchParams({ page: String(state.page), pageSize: String(pageSize) });
    if (state.status) listParams.set("status", state.status);
    if (state.priority) listParams.set("priority", state.priority);
    if (state.search) listParams.set("search", state.search);
    listParams.set("sort", state.sort);
    listParams.set("order", state.order);
    const totalsParams = (status) => new URLSearchParams({ status, page: "1", pageSize: "1" });
    const [page, open, closed] = await Promise.all([
      request(`/issues?${listParams}`),
      request(`/issues?${totalsParams("open")}`),
      request(`/issues?${totalsParams("closed")}`)
    ]);
    state.issues = page.items;
    state.totalPages = Math.max(1, page.pagination.totalPages);
    renderBoard(page.pagination, open.pagination.total, closed.pagination.total);
  } catch (error) {
    if (state.session) {
      elements["issue-list"].replaceChildren();
      elements["result-copy"].textContent = error.message;
      showToast(error.message);
    }
  } finally {
    document.querySelector(".board-card").classList.remove("loading");
  }
}

export function renderBoard(pagination, openCount, closedCount) {
  const total = openCount + closedCount;
  elements["total-count"].textContent = String(total);
  elements["open-count"].textContent = String(openCount);
  elements["closed-count"].textContent = String(closedCount);
  elements["nav-count"].textContent = String(total);
  elements["result-copy"].textContent = `${pagination.total} ${pagination.total === 1 ? "issue" : "issues"} in this view`;
  elements["sidebar-pulse"].textContent = openCount ? `${openCount} open ${openCount === 1 ? "issue" : "issues"}` : "Everything is clear";
  elements["sidebar-pulse-copy"].textContent = openCount ? "Review what needs attention next." : "No open issues need attention.";
  elements["page-copy"].textContent = `Page ${pagination.page} of ${Math.max(1, pagination.totalPages)}`;
  elements["previous-page"].disabled = pagination.page <= 1;
  elements["next-page"].disabled = pagination.totalPages === 0 || pagination.page >= pagination.totalPages;
  renderIssues();
}

export function renderIssues() {
  if (!state.issues.length) {
    elements["issue-list"].replaceChildren();
    elements["empty-state"].hidden = false;
    return;
  }
  elements["empty-state"].hidden = true;
  elements["issue-list"].innerHTML = state.issues.map((issue) => {
    const assignee = state.members.find((member) => member.id === issue.assigneeId);
    const team = state.teams.find((item) => item.id === issue.teamId);
    const planning = [
      team ? team.name : "No team",
      assignee ? `Assigned to ${assignee.username}` : "Unassigned",
      issue.dueDate ? `Due ${formatDateOnly(issue.dueDate)}` : "No due date"
    ].join(" · ");
    const canManage = state.session.user.role === "admin" || state.session.user.role === "manager";
    const worker = state.session.user.role === "viewer";
    return `
    <article class="issue-row">
      <div class="issue-summary">
        <strong>${escapeHTML(issue.title)}</strong>
        <span>${escapeHTML(issue.description || "No description")}</span>
        <small>${escapeHTML(planning)}</small>
      </div>
      <span class="priority-badge ${escapeHTML(issue.priority)}">${escapeHTML(issue.priority)}</span>
      <span class="status-badge ${issue.status === "closed" ? "closed" : ""}">${escapeHTML(issue.status)}</span>
      <div class="issue-actions">
        <button class="row-action" type="button" data-action="toggle" data-id="${Number(issue.id)}">${issue.status === "open" ? "Close" : "Reopen"}</button>
        <button class="row-action" type="button" data-action="edit" data-id="${Number(issue.id)}">${worker ? "View" : "Edit"}</button>
        ${canManage ? `<button class="row-action danger" type="button" data-action="delete" data-id="${Number(issue.id)}">Delete</button>` : ""}
      </div>
    </article>`;
  }).join("");
}

export function changePage(direction) {
  const next = state.page + direction;
  if (next < 1 || next > state.totalPages) return;
  state.page = next;
  loadBoard();
}

export function openCreateDialog() {
  if (state.session.user.role === "viewer") {
    showToast("Only managers and admins can create issues.");
    return;
  }
  if (state.session.user.role === "manager" && !state.teams.length) {
    showToast("Create a team before adding issues.");
    return;
  }
  state.editingID = null;
  elements["issue-form"].reset();
  elements["issue-priority"].value = "medium";
  ["issue-title", "issue-description", "issue-status", "issue-priority", "issue-due-date"].forEach((id) => {
    elements[id].disabled = false;
  });
  const defaultTeamID = state.session.user.role === "admin" ? "" : state.teams[0]?.id || "";
  renderTeams(defaultTeamID);
  renderMembers("", defaultTeamID);
  elements["issue-team"].disabled = state.session.user.role === "viewer";
  elements["issue-assignee"].disabled = state.session.user.role === "viewer";
  elements["activity-panel"].hidden = true;
  elements["activity-list"].replaceChildren();
  elements["dialog-kicker"].textContent = "New issue";
  elements["dialog-title"].textContent = "Capture the work";
  elements["save-issue"].textContent = "Create issue";
  hideError(elements["issue-error"]);
  elements["issue-dialog"].showModal();
  elements["issue-title"].focus();
}

export function openEditDialog(issue) {
  const worker = state.session.user.role === "viewer";
  state.editingID = issue.id;
  elements["issue-title"].value = issue.title;
  elements["issue-description"].value = issue.description;
  elements["issue-status"].value = issue.status;
  elements["issue-priority"].value = issue.priority;
  elements["issue-due-date"].value = issue.dueDate || "";
  ["issue-title", "issue-description", "issue-priority", "issue-due-date"].forEach((id) => {
    elements[id].disabled = worker;
  });
  elements["issue-status"].disabled = false;
  renderTeams(issue.teamId);
  renderMembers(issue.assigneeId, issue.teamId || "");
  elements["issue-team"].disabled = state.session.user.role === "viewer";
  elements["issue-assignee"].disabled = state.session.user.role === "viewer";
  elements["activity-panel"].hidden = false;
  elements["comment-form"].reset();
  elements["activity-list"].textContent = "Loading activity…";
  elements["dialog-kicker"].textContent = `Issue #${issue.id}`;
  elements["dialog-title"].textContent = "Update the issue";
  elements["save-issue"].textContent = worker ? "Update status" : "Save changes";
  hideError(elements["issue-error"]);
  elements["issue-dialog"].showModal();
  (worker ? elements["issue-status"] : elements["issue-title"]).focus();
  loadActivity(issue.id);
}

export async function openIssueByID(issueID) {
  try {
    openEditDialog(await request(`/issues/${issueID}`));
  } catch (error) {
    showToast(error.message);
  }
}

export function closeIssueDialog() {
  elements["issue-dialog"].close();
  state.editingID = null;
  hideError(elements["issue-error"]);
}

export async function saveIssue(event) {
  event.preventDefault();
  hideError(elements["issue-error"]);
  const editing = state.editingID !== null;
  setButtonBusy(elements["save-issue"], true, editing ? "Saving…" : "Creating…");
  try {
    const worker = state.session.user.role === "viewer";
    const payload = editing && worker ? { status: elements["issue-status"].value } : {
      title: elements["issue-title"].value,
      description: elements["issue-description"].value,
      status: elements["issue-status"].value,
      priority: elements["issue-priority"].value,
      dueDate: elements["issue-due-date"].value
    };
    if (!worker && !elements["issue-team"].disabled) payload.teamId = elements["issue-team"].value;
    if (!worker && !elements["issue-assignee"].disabled) payload.assigneeId = elements["issue-assignee"].value;
    await request(editing ? `/issues/${state.editingID}` : "/issues", {
      method: editing ? "PATCH" : "POST",
      body: JSON.stringify(payload)
    });
    closeIssueDialog();
    state.page = 1;
    await loadBoard();
    showToast(editing ? "Issue updated." : "Issue created.");
  } catch (error) {
    showError(elements["issue-error"], error.message);
  } finally {
    setButtonBusy(elements["save-issue"], false, editing && state.session.user.role === "viewer" ? "Update status" : editing ? "Save changes" : "Create issue");
  }
}

export async function loadActivity(issueID) {
  try {
    const page = await request(`/issues/${issueID}/activity?page=1&pageSize=50`);
    if (state.editingID !== issueID) return;
    elements["activity-list"].innerHTML = page.items.length ? page.items.map((activity) => `
      <article class="activity-item">
        <p><strong>${escapeHTML(activity.actorUsername)}</strong> ${escapeHTML(activityLabel(activity.kind))}</p>
        ${activity.body ? `<div>${escapeHTML(activity.body)}</div>` : ""}
        <time datetime="${escapeHTML(activity.createdAt)}">${formatDateTime(activity.createdAt)}</time>
      </article>`).join("") : '<p class="muted">No activity yet.</p>';
  } catch (error) {
    if (state.editingID === issueID) elements["activity-list"].textContent = error.message;
  }
}

export async function addComment(event) {
  event.preventDefault();
  if (state.editingID === null) return;
  const issueID = state.editingID;
  setButtonBusy(elements["save-comment"], true, "Posting…");
  try {
    await request(`/issues/${issueID}/comments`, {
      method: "POST",
      body: JSON.stringify({ body: elements["comment-body"].value })
    });
    elements["comment-form"].reset();
    await loadActivity(issueID);
  } catch (error) {
    showToast(error.message);
  } finally {
    setButtonBusy(elements["save-comment"], false, "Comment");
  }
}

export async function handleIssueAction(event) {
  const button = event.target.closest("[data-action]");
  if (!button) return;
  const issue = state.issues.find((item) => item.id === Number(button.dataset.id));
  if (!issue) return;
  if (button.dataset.action === "edit") {
    openEditDialog(issue);
    return;
  }
  if (button.dataset.action === "delete") {
    if (!window.confirm(`Delete “${issue.title}”? This cannot be undone.`)) return;
    try {
      await request(`/issues/${issue.id}`, { method: "DELETE" });
      if (state.issues.length === 1 && state.page > 1) state.page -= 1;
      await loadBoard();
      showToast("Issue deleted.");
    } catch (error) {
      showToast(error.message);
    }
    return;
  }
  try {
    await request(`/issues/${issue.id}`, {
      method: "PATCH",
      body: JSON.stringify({ status: issue.status === "open" ? "closed" : "open" })
    });
    await loadBoard();
    showToast(issue.status === "open" ? "Issue closed." : "Issue reopened.");
  } catch (error) {
    showToast(error.message);
  }
}

export function activityLabel(kind) {
  return ({
    created: "created the issue",
    updated: "updated the issue",
    statusChanged: "changed the status",
    priorityChanged: "changed the priority",
    dueDateChanged: "changed the due date",
    assignmentChanged: "changed the assignee",
    teamChanged: "changed the team",
    comment: "commented"
  })[kind] || "updated the issue";
}
