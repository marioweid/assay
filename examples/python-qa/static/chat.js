const form = document.querySelector("#chat-form");
const input = document.querySelector("#question");
const send = document.querySelector("#send");
const clear = document.querySelector("#clear");
const messages = document.querySelector("#messages");
const welcome = document.querySelector("#welcome");
const error = document.querySelector("#error");
const conversation = document.querySelector("#conversation");
let history = [];
let busy = false;

function message(role, content) {
  const element = document.createElement("article");
  element.className = `message ${role}`;
  const label = document.createElement("div");
  label.className = "speaker";
  label.textContent = role === "user" ? "You" : "Assay assistant";
  const text = document.createElement("p");
  text.textContent = content;
  element.append(label, text);
  messages.append(element);
  return element;
}

function scroll() {
  conversation.scrollTop = conversation.scrollHeight;
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
  const nextHistory = [...history, { role: "user", content: question }].slice(-19);
  try {
    const response = await fetch("/api/chat", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ messages: nextHistory }),
    });
    const data = await response.json();
    if (!response.ok) {
      throw new Error(
        typeof data.detail === "string" ? data.detail : "Unable to send this message.",
      );
    }
    const reply = message("assistant", data.answer);
    const link = document.createElement("a");
    link.className = "trace-link";
    link.href = data.trace_url;
    link.target = "_blank";
    link.rel = "noopener";
    link.textContent = data.trace_exported
      ? "View trace in Assay ↗"
      : "Trace export failed · Open Assay ↗";
    reply.append(link);
    history = [...nextHistory, { role: "assistant", content: data.answer }];
  } catch (reason) {
    userMessage.remove();
    input.value = question;
    error.textContent =
      reason instanceof Error ? reason.message : "Request failed. Please try again.";
    error.hidden = false;
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
  history = [];
  messages.replaceChildren();
  welcome.hidden = false;
  error.hidden = true;
  input.focus();
});

try {
  const response = await fetch("/api/config");
  if (!response.ok) throw new Error("Chat server is not ready. Refresh in a moment.");
  const config = await response.json();
  document.querySelector("#model").textContent = config.model;
  document.querySelector("#assay-link").href = config.traces_url;
  send.disabled = false;
} catch (reason) {
  error.textContent = reason instanceof Error ? reason.message : "Unable to connect.";
  error.hidden = false;
}
