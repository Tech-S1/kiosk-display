(() => {
  const ROOT_ID = "kiosk-hud-root";
  if (window.top !== window) return;
  if (document.getElementById(ROOT_ID)) return;

  const DISPLAY_HOME = "https://127.0.0.1:8080/";
  const PANELS = {
    links: { cls: "links-open", other: "sleep-open" },
    sleep: { cls: "sleep-open", other: "links-open" },
  };

  function kioskApi(path, opts = {}) {
    return new Promise((resolve, reject) => {
      try {
        chrome.runtime.sendMessage(
          { type: "kiosk-api", path, method: opts.method || "GET", body: opts.body },
          (res) => {
            if (chrome.runtime.lastError) {
              reject(new Error(chrome.runtime.lastError.message));
              return;
            }
            resolve(res || { ok: false });
          }
        );
      } catch (err) {
        reject(err);
      }
    });
  }

  const host = document.createElement("div");
  host.id = ROOT_ID;
  const shadow = host.attachShadow({ mode: "closed" });

  async function mount() {
    let css = "";
    try {
      css = await (await fetch(chrome.runtime.getURL("content.css"))).text();
    } catch (_) {}

    const style = document.createElement("style");
    style.textContent = css;
    shadow.appendChild(style);

    const wrap = document.createElement("div");
    wrap.innerHTML = `
      <div id="kiosk-hud-denied">
        <div>Link blocked</div>
        <div id="kiosk-hud-denied-host"></div>
      </div>
      <div id="kiosk-hud-edge-bottom"><div class="kiosk-hud-edge-grip h"></div></div>
      <div id="kiosk-hud-edge-right"><div class="kiosk-hud-edge-grip v"></div></div>
      <div id="kiosk-hud-bar" class="kiosk-hud-panel">
        <div class="kiosk-hud-bar-handle h"></div>
        <div class="kiosk-hud-bar-head"><span>Quick links</span></div>
        <div id="kiosk-hud-links"></div>
      </div>
      <div id="kiosk-hud-sleep-bar" class="kiosk-hud-panel kiosk-hud-panel-side">
        <div id="kiosk-hud-sleep-body">
          <div class="kiosk-hud-bar-head"><span>Display</span></div>
          <button type="button" id="kiosk-hud-sleep-action" class="kiosk-hud-action">Sleep</button>
        </div>
        <div class="kiosk-hud-bar-handle v"></div>
      </div>
    `;
    while (wrap.firstChild) shadow.appendChild(wrap.firstChild);

    const bar = shadow.querySelector("#kiosk-hud-bar");
    const sleepBar = shadow.querySelector("#kiosk-hud-sleep-bar");
    const linksEl = shadow.querySelector("#kiosk-hud-links");
    const sleepBtn = shadow.querySelector("#kiosk-hud-sleep-action");
    const edgeBottom = shadow.querySelector("#kiosk-hud-edge-bottom");
    const edgeRight = shadow.querySelector("#kiosk-hud-edge-right");
    const deniedEl = shadow.querySelector("#kiosk-hud-denied");
    const deniedHost = shadow.querySelector("#kiosk-hud-denied-host");
    let deniedTimer = null;
    let hideTimer = null;
    let gesture = null;

    function showDenied(name) {
      if (!deniedEl || !deniedHost) return;
      deniedHost.textContent = name || "";
      deniedEl.classList.add("show");
      if (deniedTimer) clearTimeout(deniedTimer);
      deniedTimer = setTimeout(() => deniedEl.classList.remove("show"), 2400);
    }

    window.addEventListener("kiosk-denied", (e) => {
      const name = e && e.detail && e.detail.host;
      showDenied(typeof name === "string" ? name : "");
    });

    function clearHide() {
      if (!hideTimer) return;
      clearTimeout(hideTimer);
      hideTimer = null;
    }

    function closePanels() {
      host.classList.remove("links-open", "sleep-open");
      clearHide();
    }

    function scheduleHide() {
      clearHide();
      hideTimer = setTimeout(closePanels, 5000);
    }

    function setPanel(name, open) {
      const { cls, other } = PANELS[name];
      host.classList.toggle(cls, open);
      if (open) {
        host.classList.remove(other);
        if (name === "links") refresh();
        scheduleHide();
        return;
      }
      if (!host.classList.contains(other)) clearHide();
    }

    function render(links) {
      linksEl.innerHTML = "";
      const items = [{ url: DISPLAY_HOME, label: "Home" }, ...(links || [])];
      for (const item of items) {
        const a = document.createElement("a");
        a.href = item.url;
        a.target = "_self";
        a.textContent = item.label;
        linksEl.appendChild(a);
      }
    }

    async function refresh() {
      try {
        const res = await kioskApi("/api/links");
        if (!res.ok) throw new Error("bad status");
        render(res.data || []);
      } catch (_) {
        render([]);
      }
    }

    function onGestureStart(kind, e) {
      if (!e.isTrusted) return;
      if (e.pointerType === "mouse" && e.button !== 0) return;
      gesture = { kind, id: e.pointerId, x: e.clientX, y: e.clientY };
      try {
        e.currentTarget.setPointerCapture(e.pointerId);
      } catch (_) {}
      e.preventDefault();
      e.stopPropagation();
    }

    function onGestureMove(e) {
      if (!gesture || e.pointerId !== gesture.id || !e.isTrusted) return;
      e.preventDefault();
    }

    function onGestureEnd(e) {
      if (!gesture || e.pointerId !== gesture.id) return;
      if (!e.isTrusted) {
        gesture = null;
        return;
      }
      const g = gesture;
      gesture = null;
      const dx = e.clientX - g.x;
      const dy = e.clientY - g.y;
      const panel = g.kind === "bottom" ? "links" : "sleep";
      const primary = g.kind === "bottom" ? dy : dx;
      const secondary = g.kind === "bottom" ? dx : dy;
      if (primary < -40 && Math.abs(primary) > Math.abs(secondary)) setPanel(panel, true);
      else if (primary > 40 && host.classList.contains(PANELS[panel].cls)) setPanel(panel, false);
    }

    function bindGesture(el, kind, { skipInteractive = false } = {}) {
      el.addEventListener("pointerdown", (e) => {
        if (skipInteractive && e.target.closest("a, button")) return;
        onGestureStart(kind, e);
      }, true);
      el.addEventListener("pointermove", onGestureMove, true);
      el.addEventListener("pointerup", onGestureEnd, true);
      el.addEventListener("pointercancel", () => { gesture = null; }, true);
    }

    bindGesture(edgeBottom, "bottom");
    bindGesture(edgeRight, "right");
    bindGesture(bar, "bottom", { skipInteractive: true });
    bindGesture(sleepBar, "right", { skipInteractive: true });

    sleepBtn.addEventListener("click", async (e) => {
      if (!e.isTrusted) return;
      e.preventDefault();
      e.stopPropagation();
      setPanel("sleep", false);
      try {
        await kioskApi("/api/display/sleep", { method: "POST" });
      } catch (_) {}
    }, true);

    bar.addEventListener("click", (e) => {
      if (!e.isTrusted || !e.target.closest("a")) return;
      setPanel("links", false);
    }, true);

    let wakeSent = 0;
    document.addEventListener("pointerdown", (e) => {
      if (!e.isTrusted) return;
      const panelOpen = host.classList.contains("links-open") || host.classList.contains("sleep-open");
      if (!panelOpen) {
        const now = Date.now();
        if (now - wakeSent > 1000) {
          wakeSent = now;
          kioskApi("/api/display/wake", { method: "POST" }).catch(() => {});
        }
        return;
      }
      if (e.composedPath().includes(host)) {
        scheduleHide();
        return;
      }
      closePanels();
    }, true);

    document.documentElement.appendChild(host);
    refresh();
  }

  mount();
})();
