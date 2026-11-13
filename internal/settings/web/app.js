// SPDX-License-Identifier: GPL-3.0-or-later
// Settings page for Unison Cue. All values are inserted with textContent
// or .value, never as HTML.
"use strict";

const $ = (id) => document.getElementById(id);
let state = null;   // last state from the app
let saved = "";     // serialized form at load/save, to detect changes

// ---- tabs ---------------------------------------------------------------

const tabs = [...document.querySelectorAll('[role="tab"]')];
function selectTab(tab) {
  for (const t of tabs) {
    const on = t === tab;
    t.setAttribute("aria-selected", String(on));
    t.tabIndex = on ? 0 : -1;
    $(t.getAttribute("aria-controls")).hidden = !on;
  }
  try { sessionStorage.setItem("tab", tab.id); } catch (_) {}
}
tabs.forEach((t, i) => {
  t.addEventListener("click", () => selectTab(t));
  t.addEventListener("keydown", (e) => {
    const d = e.key === "ArrowRight" ? 1 : e.key === "ArrowLeft" ? -1 : 0;
    if (d) { const n = tabs[(i + d + tabs.length) % tabs.length]; selectTab(n); n.focus(); }
  });
});
try { const t = $(sessionStorage.getItem("tab") || ""); if (t) selectTab(t); } catch (_) {}

// ---- helpers ------------------------------------------------------------

function el(tag, props = {}, ...children) {
  const e = document.createElement(tag);
  Object.assign(e, props);
  for (const c of children) e.append(c);
  return e;
}

function setDuration(nId, unitId, hours) {
  const unit = [168, 24, 1].find((u) => hours >= u && hours % u === 0) || 1;
  $(unitId).value = String(unit);
  $(nId).value = String(hours / unit);
}

function num(id) {
  const v = $(id).value.trim();
  return v === "" ? NaN : Number(v);
}

function status(msg, isError = false) {
  const s = $("status");
  s.textContent = msg;
  s.classList.toggle("error", isError);
}

// ---- render -------------------------------------------------------------

function render(st) {
  state = st;
  const c = st.config, info = st.info;

  $("unisonPath").value = c.unisonPath || "";
  $("unisonPath").placeholder = info.detectedUnison || "unison";
  $("unisonPath-hint").textContent = c.unisonPath
    ? (info.unisonVersion ? "Using " + info.unisonVersion + "." : "")
    : info.detectedUnison
      ? "Leave empty to find it automatically. Found: " + info.detectedUnison + (info.unisonVersion ? " (" + info.unisonVersion + ")" : "")
      : "unison was not found automatically. Enter its full path.";
  $("unisonDir").value = c.unisonDir || "";
  $("unisonDir").placeholder = info.defaultProfileDir;
  $("unisonDir-hint").textContent = "Leave empty to use unison's own folder (" + info.defaultProfileDir + ").";
  $("extraArgs").value = (c.extraArgs || []).join("\n");

  setDuration("reminder-n", "reminder-unit", c.reminderHours || 24);
  $("renotifyHours").value = String(c.renotifyHours || 4);
  $("timeoutMinutes").value = String(c.timeoutMinutes || 0);
  $("notifySuccess").checked = !c.quietSuccess;
  $("sshAskpass").checked = !c.noSshAskpass;
  $("autostart").checked = st.autostart;

  // Profiles
  $("profiles-dir").textContent = info.profileDir;
  const pt = $("profiles-table").querySelector("tbody");
  pt.replaceChildren();
  for (const p of st.profiles) {
    const s = p.settings || {};
    const name = el("td", {},
      el("div", { textContent: p.label ? p.label + " (" + p.name + ")" : p.name }),
      el("div", { className: "roots", textContent: (p.roots || []).join("  ⇄  ") }));
    const box = (key, checked) => {
      const i = el("input", { type: "checkbox", checked });
      i.dataset.profile = p.name; i.dataset.key = key;
      i.setAttribute("aria-label", key + " for " + p.name);
      return el("td", { className: "c" }, i);
    };
    const hours = el("input", { type: "number", min: 1, step: 1, placeholder: "default",
      value: s.reminderHours ? String(s.reminderHours) : "" });
    hours.dataset.profile = p.name; hours.dataset.key = "reminderHours";
    hours.setAttribute("aria-label", "Remind after, in hours, for " + p.name);
    const err = el("div", { className: "err" }); err.dataset.for = "profiles." + p.name + ".reminderHours";
    pt.append(el("tr", {}, name, box("show", !s.hidden), box("syncAll", !s.excludeFromSyncAll),
      box("remind", !s.noReminder), el("td", {}, hours, err)));
  }
  $("no-profiles").hidden = st.profiles.length > 0;
  $("profiles-table").hidden = st.profiles.length === 0;

  // Environment
  const et = $("env-table").querySelector("tbody");
  et.replaceChildren();
  for (const [k, v] of Object.entries(c.environment || {}).sort()) addEnvRow(k, v);
  $("shell-import-block").hidden = info.platform === "windows";
  $("shellImport").checked = !c.noShellEnvironment;
  const ft = $("effective-table").querySelector("tbody");
  ft.replaceChildren();
  const label = { settings: "These settings", shell: "Login shell", system: "System" };
  for (const v of st.env) {
    ft.append(el("tr", {}, el("td", {}, el("code", { textContent: v.name })),
      el("td", {}, el("code", { textContent: v.value })),
      el("td", {}, el("span", { className: "source", textContent: label[v.source] || v.source }))));
  }
  $("no-effective").hidden = st.env.length > 0;
  $("effective-table").hidden = st.env.length === 0;

  // About
  $("about-version").textContent = "Version " + info.version;
  $("about-unison").textContent = info.unisonVersion || "unison not found";
  $("path-config").textContent = info.configPath;
  $("path-logs").textContent = info.logDir;
  $("path-profiles").textContent = info.profileDir;

  clearErrors();
  saved = JSON.stringify(collect());
  updateButtons();
}

function addEnvRow(name = "", value = "") {
  const n = el("input", { type: "text", value: name, placeholder: "UNISONLOCALHOSTNAME", spellcheck: false });
  const v = el("input", { type: "text", value, spellcheck: false });
  n.setAttribute("aria-label", "Variable name");
  v.setAttribute("aria-label", "Value");
  n.className = "env-name"; v.className = "env-value";
  const rm = el("button", { type: "button", className: "remove", textContent: "×", title: "Remove" });
  rm.setAttribute("aria-label", "Remove variable");
  const tr = el("tr", {}, el("td", {}, n), el("td", {}, v), el("td", { className: "c" }, rm));
  rm.addEventListener("click", () => { tr.remove(); updateButtons(); });
  $("env-table").querySelector("tbody").append(tr);
  return n;
}

// ---- collect ------------------------------------------------------------

function collect() {
  const c = structuredClone(state.config);
  c.unisonPath = $("unisonPath").value.trim();
  c.unisonDir = $("unisonDir").value.trim();
  c.extraArgs = $("extraArgs").value.split("\n").map((s) => s.trim()).filter(Boolean);
  c.reminderHours = num("reminder-n") * Number($("reminder-unit").value);
  c.renotifyHours = num("renotifyHours");
  c.timeoutMinutes = num("timeoutMinutes");
  c.quietSuccess = !$("notifySuccess").checked;
  c.noSshAskpass = !$("sshAskpass").checked;
  c.noShellEnvironment = !$("shellImport").checked;

  const profiles = structuredClone(c.profiles || {});
  for (const i of document.querySelectorAll("#profiles-table input")) {
    const s = profiles[i.dataset.profile] || (profiles[i.dataset.profile] = {});
    switch (i.dataset.key) {
      case "show": s.hidden = !i.checked; break;
      case "syncAll": s.excludeFromSyncAll = !i.checked; break;
      case "remind": s.noReminder = !i.checked; break;
      case "reminderHours": s.reminderHours = i.value.trim() === "" ? 0 : Number(i.value); break;
    }
  }
  c.profiles = profiles;

  const env = {};
  for (const tr of $("env-table").querySelectorAll("tbody tr")) {
    const k = tr.querySelector(".env-name").value.trim();
    if (k !== "") env[k] = tr.querySelector(".env-value").value;
  }
  c.environment = env;
  return { config: c, autostart: $("autostart").checked };
}

function dirty() { return state && JSON.stringify(collect()) !== saved; }

function updateButtons() {
  const d = dirty();
  $("save").disabled = !d;
  $("revert").disabled = !d;
  if (d) status("Unsaved changes");
  else if ($("status").textContent === "Unsaved changes") status("");
}

function clearErrors() {
  for (const e of document.querySelectorAll(".err")) e.textContent = "";
  for (const e of document.querySelectorAll(".invalid")) e.classList.remove("invalid");
}

function showErrors(fields) {
  clearErrors();
  let firstTab = null;
  for (const [key, msg] of Object.entries(fields)) {
    const e = document.querySelector('.err[data-for="' + CSS.escape(key) + '"]');
    if (!e) continue;
    e.textContent = msg;
    const pane = e.closest('[role="tabpanel"]');
    if (!firstTab && pane) firstTab = pane.getAttribute("aria-labelledby");
    const input = e.previousElementSibling && e.previousElementSibling.matches("input") ? e.previousElementSibling
      : $(key) || null;
    if (input) input.classList.add("invalid");
  }
  if (firstTab && $(firstTab).getAttribute("aria-selected") !== "true") selectTab($(firstTab));
}

// ---- server -------------------------------------------------------------

async function api(method, path, body) {
  const res = await fetch(path, {
    method,
    headers: body ? { "Content-Type": "application/json" } : {},
    body: body ? JSON.stringify(body) : undefined,
    credentials: "same-origin",
  });
  let data = null;
  try { data = await res.json(); } catch (_) {}
  if (!res.ok) {
    const err = new Error((data && data.error) || res.statusText);
    err.fields = data && data.fields;
    throw err;
  }
  return data;
}

async function load() {
  try {
    render(await api("GET", "/api/state"));
    $("load-error").hidden = true;
  } catch (e) {
    $("load-error").textContent = "Could not load the settings: " + e.message;
    $("load-error").hidden = false;
  }
}

async function save() {
  $("save").disabled = true;
  status("Saving…");
  try {
    render(await api("POST", "/api/state", collect()));
    status("Saved");
  } catch (e) {
    if (e.fields) showErrors(e.fields);
    status(e.message, true);
    updateButtons();
  }
}

document.addEventListener("input", updateButtons);
document.addEventListener("change", updateButtons);
$("save").addEventListener("click", save);
$("revert").addEventListener("click", () => { render(state); status(""); });
$("env-add").addEventListener("click", () => { addEnvRow().focus(); updateButtons(); });
for (const b of document.querySelectorAll("[data-open]")) {
  b.addEventListener("click", () => api("POST", "/api/open", { what: b.dataset.open }).catch((e) => status(e.message, true)));
}
document.addEventListener("keydown", (e) => {
  if ((e.metaKey || e.ctrlKey) && e.key === "s") { e.preventDefault(); if (!$("save").disabled) save(); }
});
window.addEventListener("focus", () => { if (!dirty()) load(); });

load();
