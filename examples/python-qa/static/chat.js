const form = document.querySelector("#chat-form");
const input = document.querySelector("#question");
const send = document.querySelector("#send");
const clear = document.querySelector("#clear");
const messages = document.querySelector("#messages");
const welcome = document.querySelector("#welcome");
const error = document.querySelector("#error");
const conversation = document.querySelector("#conversation");
let busy = true;
let sessionReady = false;
clear.disabled = true;

function message(role, content, traceUrl, exported = true) {
  const element = document.createElement("article");
  element.className = `message ${role}`;
  const label = document.createElement("div");
  label.className = "speaker";
  label.textContent = role === "user" ? "You" : "Assay assistant";
  const text = document.createElement("p");
  text.textContent = content;
  element.append(label, text);
  if (traceUrl) {
    const link = document.createElement("a");
    link.className = "trace-link";
    link.href = traceUrl;
    link.target = "_blank";
    link.rel = "noopener";
    link.textContent = exported ? "View trace in Assay ↗" : "Trace export failed · Open Assay ↗";
    element.append(link);
  }
  messages.append(element);
  return element;
}

function showError(reason) {
  error.textContent =
    reason instanceof Error ? reason.message : "Request failed. Please try again.";
  error.hidden = false;
}

function scroll() {
  conversation.scrollTop = conversation.scrollHeight;
}

async function loadSession() {
  let cursor = null;
  const seen = new Set();
  do {
    const url =
      cursor === null ? "/api/session" : `/api/session?cursor=${encodeURIComponent(cursor)}`;
    const response = await fetch(url);
    const data = await response.json();
    if (!response.ok) throw new Error("Unable to restore the chat. Refresh to retry.");
    for (const turn of data.turns) {
      if (turn.question) message("user", turn.question, turn.trace_url);
      if (turn.answer) message("assistant", turn.answer, turn.trace_url);
      if (!turn.question && !turn.answer) {
        message(
          "assistant",
          "This turn has no captured messages. Open its trace for details.",
          turn.trace_url,
        );
      }
    }
    cursor = data.next_cursor;
    if (cursor !== null && seen.has(cursor))
      throw new Error("Chat history returned a repeated page.");
    if (cursor !== null) seen.add(cursor);
  } while (cursor !== null);
  welcome.hidden = messages.childElementCount > 0;
  scroll();
}

async function submit() {
  const question = input.value.trim();
  if (!question || busy || send.disabled) return;
  busy = true;
  send.disabled = true;
  clear.disabled = true;
  error.hidden = true;
  welcome.hidden = true;
  input.value = "";
  const userMessage = message("user", question);
  const pending = document.createElement("div");
  pending.className = "pending";
  pending.setAttribute("role", "status");
  pending.textContent = "Thinking, then recording the trace…";
  messages.append(pending);
  scroll();
  try {
    const response = await fetch("/api/chat", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ message: question }),
    });
    const data = await response.json();
    if (!response.ok) {
      throw new Error(
        typeof data.detail === "string" ? data.detail : "Unable to send this message.",
      );
    }
    message("assistant", data.answer, data.trace_url, data.trace_exported);
  } catch (reason) {
    userMessage.remove();
    input.value = question;
    showError(reason);
  } finally {
    pending.remove();
    busy = false;
    send.disabled = false;
    clear.disabled = false;
    scroll();
    input.focus();
  }
}

form.addEventListener("submit", (event) => {
  event.preventDefault();
  void submit();
});
input.addEventListener("keydown", (event) => {
  if (event.key === "Enter" && !event.shiftKey) {
    event.preventDefault();
    void submit();
  }
});
for (const button of document.querySelectorAll(".suggestions button")) {
  button.addEventListener("click", () => {
    input.value = button.firstChild.textContent.trim();
    void submit();
  });
}
clear.addEventListener("click", () => {
  if (busy) return;
  busy = true;
  clear.disabled = true;
  send.disabled = true;
  error.hidden = true;
  void fetch("/api/session", { method: "POST" })
    .then((response) => {
      if (!response.ok) throw new Error("Unable to start a new chat. Try again.");
      messages.replaceChildren();
      sessionReady = true;
      welcome.hidden = false;
      input.focus();
    })
    .catch(showError)
    .finally(() => {
      busy = false;
      clear.disabled = false;
      send.disabled = !sessionReady;
    });
});

try {
  const response = await fetch("/api/config");
  if (!response.ok) throw new Error("Chat server is not ready. Refresh in a moment.");
  const config = await response.json();
  document.querySelector("#model").textContent = config.model;
  document.querySelector("#assay-link").href = config.traces_url;
  await loadSession();
  sessionReady = true;
  send.disabled = false;
} catch (reason) {
  showError(reason);
} finally {
  busy = false;
  clear.disabled = false;
}
