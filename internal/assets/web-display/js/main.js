(async () => {
  const el = document.getElementById("links");
  try {
    const res = await fetch("/api/links", { cache: "no-store" });
    if (!res.ok) throw new Error("bad status");
    const items = await res.json();
    const list = Array.isArray(items) ? items : [];
    if (!list.length) {
      el.innerHTML = '<p class="empty">No links configured</p>';
      return;
    }
    el.innerHTML = "";
    for (const item of list) {
      const label = (item.label || "").trim();
      const url = (item.url || "").trim();
      if (!label || !url) continue;
      const a = document.createElement("a");
      a.href = url;
      a.textContent = label;
      el.appendChild(a);
    }
    if (!el.children.length) {
      el.innerHTML = '<p class="empty">No links configured</p>';
    }
  } catch (_) {
    el.innerHTML = '<p class="empty">Unable to load links</p>';
  }
})();
