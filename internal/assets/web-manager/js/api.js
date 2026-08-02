const status = document.getElementById("status");
const loginStatus = document.getElementById("login-status");

let unauthorizedHandler = () => {};
let csrfToken = "";

export function onUnauthorized(fn) {
  unauthorizedHandler = fn;
}

export function setCSRF(token) {
  csrfToken = token || "";
}

export function errMsg(err) {
  return String(err && err.message ? err.message : err);
}

function setBanner(el, msg, isError) {
  el.textContent = msg || "";
  el.classList.toggle("error", !!isError && !!msg);
}

export function setLoginStatus(msg, isError) {
  setBanner(loginStatus, msg, isError);
}

export function setStatus(msg, isError) {
  setBanner(status, msg, isError);
}

export function clearError() {
  setStatus("");
}

export function setError(err) {
  if (/unauthorized/i.test(errMsg(err))) {
    unauthorizedHandler();
    return;
  }
  setStatus(errMsg(err), true);
}

function withCSRF(opts = {}) {
  const method = (opts.method || "GET").toUpperCase();
  const headers = { ...(opts.headers || {}) };
  if (csrfToken && method !== "GET" && method !== "HEAD") {
    headers["X-CSRF-Token"] = csrfToken;
  }
  return { ...opts, headers };
}

export async function api(path, opts = {}) {
  const res = await fetch(path, { cache: "no-store", ...withCSRF(opts) });
  if (res.status === 401) throw new Error("unauthorized");
  return res;
}

export async function apiText(path, opts) {
  const res = await api(path, opts);
  const text = await res.text();
  if (!res.ok) throw new Error(text || res.statusText);
  return text;
}

export async function apiJSON(path, opts) {
  const text = await apiText(path, opts);
  if (!text) return {};
  try {
    return JSON.parse(text);
  } catch {
    return {};
  }
}

export async function postJSON(path, body) {
  return apiJSON(path, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body || {}),
  });
}
