import { apiJSON, setError } from "./api.js";

const auditRows = document.getElementById("audit-rows");
const auditMeta = document.getElementById("audit-meta");
const auditPrev = document.getElementById("audit-prev");
const auditNext = document.getElementById("audit-next");
const auditFilterEl = document.getElementById("audit-filter");

let auditPage = 1;
let auditFilter = "all";
const auditPageSize = 25;

function formatAuditTime(iso) {
  try {
    const d = new Date(iso);
    if (Number.isNaN(d.getTime())) return String(iso || "");
    return d.toLocaleString();
  } catch {
    return String(iso || "");
  }
}

function auditEmptyRow(text) {
  const tr = document.createElement("tr");
  tr.innerHTML = '<td colspan="4" class="meta"></td>';
  tr.querySelector("td").textContent = text;
  return tr;
}

function renderAuditItems(items) {
  auditRows.innerHTML = "";
  if (!items.length) {
    auditRows.appendChild(auditEmptyRow("No matching entries"));
    return;
  }
  items.forEach((item) => {
    const sourceText = String(item.source || "");
    const blocked = sourceText.toLowerCase().startsWith("blocked");
    const tr = document.createElement("tr");
    if (blocked) tr.className = "blocked";

    const time = document.createElement("td");
    time.className = "meta";
    time.textContent = formatAuditTime(item.time);

    const label = document.createElement("td");
    label.className = "label";
    label.textContent = item.label || "";

    const url = document.createElement("td");
    url.className = "url";
    url.textContent = item.url || "";

    const source = document.createElement("td");
    source.className = blocked ? "source-blocked" : "meta";
    source.textContent = sourceText;

    tr.append(time, label, url, source);
    auditRows.appendChild(tr);
  });
}

export async function loadAudit(page = auditPage) {
  try {
    const data = await apiJSON(
      "/api/audit?page=" + page + "&page_size=" + auditPageSize + "&filter=" + encodeURIComponent(auditFilter)
    );
    auditPage = data.page || 1;
    if (data.filter) {
      auditFilter = data.filter;
      auditFilterEl.value = auditFilter;
    }

    renderAuditItems(Array.isArray(data.items) ? data.items : []);

    const total = data.total || 0;
    const totalPages = data.total_pages || 0;
    const countLabel =
      auditFilter === "blocked" ? total + " blocked" :
      auditFilter === "allowed" ? total + " allowed" :
      total + " total";
    auditMeta.textContent = total
      ? "Page " + auditPage + " of " + totalPages + " · " + countLabel
      : "No entries";
    auditPrev.disabled = auditPage <= 1;
    auditNext.disabled = !totalPages || auditPage >= totalPages;
  } catch (e) {
    auditRows.innerHTML = "";
    auditRows.appendChild(auditEmptyRow("Failed to load audit log"));
    auditMeta.textContent = "";
    setError(e);
  }
}

export function wireAudit() {
  auditPrev.onclick = () => {
    if (auditPage > 1) loadAudit(auditPage - 1);
  };
  auditNext.onclick = () => loadAudit(auditPage + 1);
  document.getElementById("audit-refresh").onclick = () => loadAudit(auditPage);
  auditFilterEl.onchange = (e) => {
    auditFilter = e.target.value || "all";
    loadAudit(1);
  };
}
