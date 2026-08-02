import { loadAudit } from "./audit.js";

export function tabFromPath() {
  return location.pathname === "/audit" || location.pathname === "/audit/" ? "audit" : "control";
}

export function setTab(name, push) {
  document.querySelectorAll(".tabs button").forEach((btn) => {
    btn.classList.toggle("active", btn.dataset.tab === name);
  });
  document.getElementById("panel-control").hidden = name !== "control";
  document.getElementById("panel-audit").hidden = name !== "audit";
  const path = name === "audit" ? "/audit" : "/";
  if (push !== false && location.pathname !== path) {
    history.pushState({ tab: name }, "", path);
  }
  if (name === "audit") loadAudit();
}

export function wireTabs() {
  document.getElementById("tab-control").onclick = () => setTab("control");
  document.getElementById("tab-audit").onclick = () => setTab("audit");
  window.addEventListener("popstate", () => setTab(tabFromPath(), false));
}
