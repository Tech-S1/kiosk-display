import { apiJSON, errMsg, setCSRF, setLoginStatus } from "./api.js";

const loginEl = document.getElementById("login");
const appEl = document.getElementById("app");
const loginPassword = document.getElementById("login-password");
const passwordForm = document.getElementById("login-password-form");
const oidcForm = document.getElementById("login-oidc-form");
const skipRedirectKey = "oidc_skip_redirect";

let started = false;
let onReady = () => {};
let onStart = () => {};
let authMethods = { password: false, oidc: false, autoRedirect: false };

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
  if (!passwordForm.hidden) loginPassword.focus();
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

function applyAuthMethods(data) {
  authMethods = {
    password: !!(data && data.password),
    oidc: !!(data && data.oidc),
    autoRedirect: !!(data && data.auto_redirect),
  };
  passwordForm.hidden = !authMethods.password;
  oidcForm.hidden = !authMethods.oidc;
  if (authMethods.password && authMethods.oidc) oidcForm.classList.add("with-divider");
  else oidcForm.classList.remove("with-divider");
}

function shouldAutoRedirect() {
  if (!authMethods.oidc || !authMethods.autoRedirect) return false;
  if (sessionStorage.getItem(skipRedirectKey) === "1") {
    sessionStorage.removeItem(skipRedirectKey);
    return false;
  }
  return true;
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
    applyAuthMethods(data);
    if (data && data.ok) {
      setCSRF(data.csrf || "");
      showApp();
      return true;
    }
    if (shouldAutoRedirect()) {
      window.location.href = "/api/oidc/login";
      return false;
    }
  } catch {
    passwordForm.hidden = false;
    oidcForm.hidden = true;
  }
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
  document.getElementById("login-oidc").onclick = () => {
    window.location.href = "/api/oidc/login";
  };
  document.getElementById("logout").onclick = async () => {
    let logoutURL = "";
    try {
      const res = await fetch("/api/logout", { method: "POST" });
      if (res.ok) {
        try {
          const data = await res.json();
          logoutURL = data.logout_url || "";
        } catch {}
      }
    } catch {}
    setCSRF("");
    started = false;
    if (authMethods.autoRedirect || logoutURL) {
      sessionStorage.setItem(skipRedirectKey, "1");
    }
    if (logoutURL) {
      window.location.href = logoutURL;
      return;
    }
    showLogin();
  };
}
