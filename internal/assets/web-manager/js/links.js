import { apiJSON, apiText, clearError, postJSON, setError } from "./api.js";

const rowsEl = document.getElementById("rows");
const editLinksBtn = document.getElementById("edit-links");
const addLinkBtn = document.getElementById("add");
const saveLinksBtn = document.getElementById("save");
const cancelLinksBtn = document.getElementById("cancel-links");

let linksEditing = false;
let linksEditable = false;
let savedLinks = [];

function applyLinksEditable() {
  editLinksBtn.hidden = !linksEditable || linksEditing;
  if (!linksEditable && linksEditing) setLinksEditing(false);
}

export function setLinksEditable(on) {
  linksEditable = !!on;
  applyLinksEditable();
}

function setLinksEditing(on) {
  if (on && !linksEditable) return;
  linksEditing = !!on;
  editLinksBtn.hidden = !linksEditable || linksEditing;
  addLinkBtn.hidden = !linksEditing;
  saveLinksBtn.hidden = !linksEditing;
  cancelLinksBtn.hidden = !linksEditing;
  renderLinks(savedLinks);
}

function readRowsDraft() {
  return [...rowsEl.querySelectorAll(".row")].map((el) => ({
    label: el.querySelector('[data-k="label"]').value.trim(),
    url: el.querySelector('[data-k="url"]').value.trim(),
  }));
}

function readRows() {
  return readRowsDraft().filter((x) => x.label && x.url);
}

async function openOnDisplay(url) {
  await postJSON("/api/remote/navigate", { url });
  clearError();
}

function row(item) {
  const wrap = document.createElement("div");
  wrap.className = "row";

  const label = document.createElement("input");
  label.dataset.k = "label";
  label.placeholder = "Label";
  label.value = item.label || "";
  label.readOnly = !linksEditing;

  const url = document.createElement("input");
  url.dataset.k = "url";
  url.placeholder = "https://";
  url.value = item.url || "";
  url.readOnly = !linksEditing;

  wrap.append(label, url);

  if (linksEditing) {
    const remove = document.createElement("button");
    remove.type = "button";
    remove.className = "danger";
    remove.textContent = "Remove";
    remove.onclick = () => wrap.remove();
    wrap.appendChild(remove);
  } else {
    const open = document.createElement("button");
    open.type = "button";
    open.textContent = "Open";
    open.onclick = () => {
      const href = url.value.trim();
      if (href) openOnDisplay(href).catch(setError);
    };
    wrap.appendChild(open);
  }
  return wrap;
}

function renderLinks(items) {
  rowsEl.innerHTML = "";
  const list = items && items.length ? items : (linksEditing ? [{ label: "", url: "" }] : []);
  list.forEach((item) => rowsEl.appendChild(row(item)));
}

export async function loadLinks() {
  const items = await apiJSON("/api/links");
  savedLinks = Array.isArray(items) ? items : [];
  renderLinks(savedLinks);
}

async function saveLinks() {
  if (!linksEditable) return;
  try {
    await apiText("/api/links", {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(readRows()),
    });
    clearError();
    await loadLinks();
    setLinksEditing(false);
  } catch (e) {
    setError(e);
  }
}

export function wireLinks() {
  editLinksBtn.onclick = () => {
    if (linksEditable) setLinksEditing(true);
  };
  cancelLinksBtn.onclick = () => setLinksEditing(false);
  addLinkBtn.onclick = () => {
    if (linksEditable && linksEditing) rowsEl.appendChild(row({ label: "", url: "" }));
  };
  saveLinksBtn.onclick = () => saveLinks();
  document.getElementById("nav-go").onclick = () => {
    const url = document.getElementById("nav-url").value.trim();
    if (url) openOnDisplay(url).catch(setError);
  };
  document.getElementById("homepage").onclick = () => openOnDisplay("").catch(setError);
}
