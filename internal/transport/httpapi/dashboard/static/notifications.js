import { clearSession, elements, escapeHTML, formatDateTime, request, showToast, state } from "./core.js";

export function startNotifications() {
  stopNotifications();
  if (state.session) connectNotifications();
}

export function stopNotifications() {
  state.notificationAbort?.abort();
  state.notificationAbort = null;
  window.clearTimeout(state.notificationReconnectTimer);
}

export async function loadNotifications() {
  const inbox = await request("/notifications");
  state.notifications = inbox.items;
  state.notificationUnread = inbox.unread;
  renderNotifications();
}

export async function markAllNotificationsRead() {
  if (!state.notificationUnread) return;
  try {
    await request("/notifications/readAll", { method: "POST" });
    state.notifications.forEach((item) => { item.readAt ||= new Date().toISOString(); });
    state.notificationUnread = 0;
    renderNotifications();
  } catch (error) {
    showToast(error.message);
  }
}

export async function handleNotificationClick(event) {
  const button = event.target.closest("[data-notification-id]");
  if (!button) return;
  const notification = state.notifications.find((item) => item.id === Number(button.dataset.notificationId));
  if (!notification) return;
  if (!notification.readAt) {
    try {
      await request(`/notifications/${notification.id}/read`, { method: "PATCH" });
      notification.readAt = new Date().toISOString();
      state.notificationUnread = Math.max(0, state.notificationUnread - 1);
      renderNotifications();
    } catch (error) {
      showToast(error.message);
    }
  }
  window.dispatchEvent(new CustomEvent("notification-navigation", {
    detail: { issueId: notification.issueId, teamId: notification.teamId }
  }));
}

function renderNotifications() {
  elements["notification-nav-count"].textContent = state.notificationUnread > 99 ? "99+" : String(state.notificationUnread);
  elements["notification-nav-count"].hidden = state.notificationUnread === 0;
  elements["mark-all-notifications"].disabled = state.notificationUnread === 0;
  elements["notification-empty"].hidden = state.notifications.length !== 0;
  elements["notification-list"].innerHTML = state.notifications.map((notification) => `
    <button class="notification-item${notification.readAt ? "" : " unread"}" type="button" data-notification-id="${notification.id}">
      <span class="notification-dot" aria-hidden="true"></span>
      <span class="notification-copy">
        <strong>${escapeHTML(notification.message)}</strong>
        <small>${escapeHTML(formatDateTime(notification.createdAt))}</small>
      </span>
    </button>
  `).join("");
}

async function connectNotifications() {
  const controller = new AbortController();
  state.notificationAbort = controller;
  try {
    const response = await fetch("/notifications/stream", {
      headers: { Accept: "text/event-stream", Authorization: `Bearer ${state.session.token}` },
      signal: controller.signal
    });
    if (response.status === 401) {
	  clearSession();
      window.dispatchEvent(new Event("session-expired"));
      return;
    }
    if (!response.ok || !response.body) throw new Error("Live notifications are unavailable.");
    await loadNotifications();
    const reader = response.body.getReader();
    const decoder = new TextDecoder();
    let buffer = "";
    while (true) {
      const { value, done } = await reader.read();
      if (done) break;
      buffer += decoder.decode(value, { stream: true });
      const frames = buffer.split("\n\n");
      buffer = frames.pop() || "";
      frames.forEach(receiveFrame);
    }
  } catch (error) {
    if (error.name !== "AbortError") {
      console.warn(error.message);
      if (state.notifications.length === 0 && state.session) {
        loadNotifications().catch((loadError) => showToast(loadError.message));
      }
    }
  } finally {
    if (state.notificationAbort === controller) state.notificationAbort = null;
    if (!controller.signal.aborted && state.session) {
      state.notificationReconnectTimer = window.setTimeout(connectNotifications, 2000);
    }
  }
}

function receiveFrame(frame) {
  const data = frame.split("\n").find((line) => line.startsWith("data: "))?.slice(6);
  if (!data) return;
  try {
    const notification = JSON.parse(data);
    if (state.notifications.some((item) => item.id === notification.id)) return;
    state.notifications.unshift(notification);
    state.notifications = state.notifications.slice(0, 50);
    state.notificationUnread += 1;
    renderNotifications();
    showToast(notification.message);
  } catch {}
}
