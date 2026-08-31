import { elements, request, setButtonBusy, showToast, state } from "./core.js";

export async function startIssueExport() {
  if (state.issueExport && !terminalOperation(state.issueExport.status)) return;
  setButtonBusy(elements["export-issues-button"], true, "Starting…");
  state.issueExportIdempotencyKey ||= crypto.randomUUID();
  try {
    const operation = await request("/exports/issues", {
      method: "POST",
      headers: {
        "Idempotency-Key": state.issueExportIdempotencyKey,
        Prefer: "respond-async"
      }
    });
    state.issueExportIdempotencyKey = null;
    state.issueExportCompletionHandled = false;
    state.issueExport = operation;
    await updateIssueExport(operation);
    if (!terminalOperation(operation.status)) {
      pollIssueExport();
      watchIssueExportCompletion();
    }
  } catch (error) {
    showToast(error.message);
  } finally {
    const running = state.issueExport && !terminalOperation(state.issueExport.status);
    setButtonBusy(elements["export-issues-button"], running, running ? "Exporting…" : "Export CSV");
  }
}

export async function cancelIssueExport() {
  if (!state.issueExport?.status_url || terminalOperation(state.issueExport.status)) return;
  setButtonBusy(elements["cancel-export-button"], true, "Cancelling…");
  try {
    await request(state.issueExport.status_url, { method: "DELETE" });
    await pollIssueExport();
  } catch (error) {
    showToast(error.message);
  } finally {
    setButtonBusy(elements["cancel-export-button"], false, "Cancel");
  }
}

export function stopIssueExport() {
  window.clearTimeout(state.issueExportPollTimer);
  state.issueExportPollTimer = null;
  state.issueExportAbort?.abort();
  state.issueExportAbort = null;
}

async function pollIssueExport() {
  window.clearTimeout(state.issueExportPollTimer);
  state.issueExportPollTimer = null;
  if (!state.issueExport?.status_url || terminalOperation(state.issueExport.status)) return;
  try {
    await updateIssueExport(await request(state.issueExport.status_url));
  } catch (error) {
    showToast(error.message);
  }
  if (state.issueExport && !terminalOperation(state.issueExport.status)) {
    state.issueExportPollTimer = window.setTimeout(pollIssueExport, 1000);
  }
}

async function watchIssueExportCompletion() {
  state.issueExportAbort?.abort();
  const statusURL = state.issueExport?.status_url;
  if (!statusURL || terminalOperation(state.issueExport.status)) return;
  const controller = new AbortController();
  state.issueExportAbort = controller;
  try {
    const response = await fetch(`${statusURL}/events`, {
      headers: { Accept: "text/event-stream", Authorization: `Bearer ${state.session.token}` },
      signal: controller.signal
    });
    if (!response.ok || !response.body) throw new Error("Live export status is unavailable.");
    const reader = response.body.getReader();
    const decoder = new TextDecoder();
    let buffer = "";
    while (true) {
      const { value, done } = await reader.read();
      if (done) break;
      buffer += decoder.decode(value, { stream: true });
      const frames = buffer.split("\n\n");
      buffer = frames.pop() || "";
      for (const frame of frames) {
        const data = frame.split("\n").find((line) => line.startsWith("data: "))?.slice(6);
        if (data) await updateIssueExport(JSON.parse(data));
      }
    }
  } catch (error) {
    if (error.name !== "AbortError") console.warn(error.message);
  } finally {
    if (state.issueExportAbort === controller) state.issueExportAbort = null;
  }
}

async function updateIssueExport(operation) {
  operation.status_url ||= state.issueExport?.status_url;
  state.issueExport = operation;
  renderIssueExport();
  if (!terminalOperation(operation.status) || state.issueExportCompletionHandled) return;
  state.issueExportCompletionHandled = true;
  stopIssueExport();
  setButtonBusy(elements["export-issues-button"], false, "Export CSV");
  if (operation.status === "succeeded") {
    await downloadIssueExport(operation.result);
    showToast(`Exported ${operation.result.rows} issues.`);
  } else if (operation.status === "cancelled") {
    showToast("Issue export cancelled.");
  } else {
    showToast(operation.error?.message || "Issue export failed.");
  }
}

function renderIssueExport() {
  const operation = state.issueExport;
  elements["export-status"].hidden = !operation;
  if (!operation) return;
  const progress = Number(operation.progress) || 0;
  elements["export-progress"].value = progress;
  elements["export-progress"].textContent = `${progress}%`;
  const labels = {
    pending: "Queued",
    running: `Exporting… ${progress}%`,
    retrying: `Retrying (attempt ${operation.attempt + 1} of ${operation.max_attempts})…`,
    succeeded: `Ready · ${operation.result?.rows || 0} issues`,
    failed: operation.error?.message || "Export failed",
    cancelled: "Export cancelled"
  };
  elements["export-copy"].textContent = labels[operation.status] || operation.status;
  elements["cancel-export-button"].hidden = terminalOperation(operation.status);
}

async function downloadIssueExport(result) {
  const response = await fetch(result.download_url, {
    headers: { Authorization: `Bearer ${state.session.token}` }
  });
  if (!response.ok) {
    const payload = await response.json().catch(() => null);
    throw new Error(payload?.message || "The export could not be downloaded.");
  }
  const href = URL.createObjectURL(await response.blob());
  const link = document.createElement("a");
  link.href = href;
  link.download = result.filename;
  link.click();
  URL.revokeObjectURL(href);
}

function terminalOperation(status) {
  return ["succeeded", "failed", "cancelled"].includes(status);
}
