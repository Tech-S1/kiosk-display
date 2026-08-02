import { apiJSON, errMsg, setCSRF, setLoginStatus } from "./api.js";

const loginEl = document.getElementById("login");
const appEl = document.getElementById("app");
const loginPassword = document.getElementById("login-password");

let started = false;
let onReady = () => {};
let onStart = () => {};

export function isAppVisible() {
  return !appEl.hidden;
}

export function resetStarted() {
  started = false;
}

export function showLogin() {
  appEl.hidden = true;
  loginEl.hidden = false;
  setCSRF("");
  loginPassword.focus();
}

export function showApp() {
  loginEl.hidden = true;
  appEl.hidden = false;
  setLoginStatus("");
  onReady();
  if (started) return;
  started = true;
  onStart();
}

async function doLogin() {
  const password = loginPassword.value;
  if (!password) return;
  setLoginStatus("");
  try {
    const res = await fetch("/api/login", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ password }),
    });
    if (!res.ok) {
      setLoginStatus((await res.text()) || "invalid password", true);
      return;
    }
    let data = {};
    try {
      data = await res.json();
    } catch {}
    setCSRF(data.csrf || "");
    loginPassword.value = "";
    showApp();
  } catch (e) {
    setLoginStatus(errMsg(e), true);
  }
}

export async function ensureAuth() {
  try {
    const data = await apiJSON("/api/auth");
    if (data && data.ok) {
      setCSRF(data.csrf || "");
      showApp();
      return true;
    }
  } catch {}
  showLogin();
  return false;
}

export function wireAuth(ready, start) {
  onReady = ready;
  onStart = start;
  document.getElementById("login-submit").onclick = () => doLogin();
  loginPassword.addEventListener("keydown", (e) => {
    if (e.key === "Enter") {
      e.preventDefault();
      doLogin();
    }
  });
  document.getElementById("logout").onclick = async () => {
    try {
      await fetch("/api/logout", { method: "POST" });
    } catch {}
    setCSRF("");
    started = false;
    showLogin();
  };
}
