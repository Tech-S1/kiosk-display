import { api, apiJSON, clearError, postJSON, setError } from "./api.js";
import { isAppVisible } from "./auth.js";

const screenWrap = document.getElementById("screen-wrap");
const screen = document.getElementById("screen");
const typeText = document.getElementById("type-text");

let screenW = 1280;
let screenH = 720;
let drag = null;
let moveBusy = false;
let pendingMove = null;
let prevFrameURL = "";

function setDisplayAspect(w, h) {
  const width = Number(w);
  const height = Number(h);
  if (!(width > 0) || !(height > 0)) return;
  document.documentElement.style.setProperty("--display-aspect", width + " / " + height);
}

export function setScreenSize(w, h) {
  const width = Number(w);
  const height = Number(h);
  if (!(width > 0) || !(height > 0)) return;
  screenW = width;
  screenH = height;
  setDisplayAspect(width, height);
}

function setAsleepUI(asleep) {
  screenWrap.classList.toggle("asleep", !!asleep);
}

function applyRemoteState(res) {
  if (res && typeof res.asleep === "boolean") setAsleepUI(res.asleep);
}

async function syncDisplayStatus() {
  try {
    const data = await apiJSON("/api/display/status");
    if (typeof data.asleep === "boolean") setAsleepUI(data.asleep);
  } catch {}
}

function setPasswordMode(on) {
  const next = on ? "password" : "text";
  if (typeText.type === next) return;
  const val = typeText.value;
  typeText.type = next;
  typeText.value = val;
  typeText.placeholder = on
    ? "Password field — text only, input masked"
    : "Insert text into focused field";
}

function applyFrame(blob) {
  if (!(blob instanceof Blob) || blob.size < 32) return;
  const url = URL.createObjectURL(blob);
  const prev = prevFrameURL;
  const img = new Image();
  img.onload = () => {
    screen.src = url;
    prevFrameURL = url;
    if (prev && prev.startsWith("blob:") && prev !== url) URL.revokeObjectURL(prev);
  };
  img.onerror = () => URL.revokeObjectURL(url);
  img.src = url;
}

async function refreshScreen() {
  try {
    await postJSON("/api/remote/reload", {});
    const res = await api("/api/remote/screenshot?ts=" + Date.now());
    if (!res.ok) throw new Error(await res.text());
    applyFrame(await res.blob());
  } catch (e) {
    setError(e);
  }
}

export function connectStream() {
  const proto = location.protocol === "https:" ? "wss:" : "ws:";
  const ws = new WebSocket(proto + "//" + location.host + "/api/remote/stream");
  ws.binaryType = "blob";
  ws.onopen = () => syncDisplayStatus();
  ws.onmessage = async (e) => {
    if (typeof e.data === "string") {
      try {
        const msg = JSON.parse(e.data);
        if (typeof msg.asleep === "boolean") {
          if (!msg.asleep) setAsleepUI(false);
          else await syncDisplayStatus();
        }
        if (typeof msg.password === "boolean") setPasswordMode(msg.password);
      } catch {}
      return;
    }
    if (e.data instanceof Blob) applyFrame(e.data);
  };
  ws.onclose = () => {
    if (!isAppVisible()) return;
    setTimeout(() => {
      if (isAppVisible()) connectStream();
    }, 800);
  };
  ws.onerror = () => {
    try { ws.close(); } catch {}
  };
}

function coordsFromEvent(e) {
  const rect = screen.getBoundingClientRect();
  if (!rect.width || !rect.height) return null;
  const nw = screen.naturalWidth || screenW;
  const nh = screen.naturalHeight || screenH;
  if (!(nw > 0) || !(nh > 0)) return null;
  const scale = Math.min(rect.width / nw, rect.height / nh);
  const contentW = nw * scale;
  const contentH = nh * scale;
  if (!(contentW > 0) || !(contentH > 0)) return null;
  const left = rect.left + (rect.width - contentW) / 2;
  const top = rect.top + (rect.height - contentH) / 2;
  const px = (e.clientX - left) / contentW;
  const py = (e.clientY - top) / contentH;
  if (px < 0 || py < 0 || px > 1 || py > 1) return null;
  return {
    x: Math.max(0, Math.min(screenW, px * screenW)),
    y: Math.max(0, Math.min(screenH, py * screenH)),
  };
}

async function sendClick(x, y) {
  const res = await postJSON("/api/remote/click", { x, y });
  clearError();
  applyRemoteState(res);
  if (res && typeof res.password === "boolean") setPasswordMode(res.password);
  return res;
}

async function sendTouch(phase, x, y) {
  const res = await postJSON("/api/remote/touch", { phase, x, y });
  clearError();
  applyRemoteState(res);
  return res;
}

async function flushMove() {
  if (moveBusy || !pendingMove) return;
  const pt = pendingMove;
  pendingMove = null;
  moveBusy = true;
  try {
    await sendTouch("move", pt.x, pt.y);
  } catch (err) {
    setError(err);
  } finally {
    moveBusy = false;
    if (pendingMove) flushMove();
  }
}

async function wakeDisplayOnly() {
  setAsleepUI(false);
  try {
    await postJSON("/api/display/wake", {});
    setAsleepUI(false);
    clearError();
  } catch (err) {
    await syncDisplayStatus();
    setError(err);
  }
}

async function withRemote(fn) {
  try {
    await fn();
  } catch (err) {
    await syncDisplayStatus();
    setError(err);
  }
}

async function endDrag(e) {
  if (!drag || e.pointerId !== drag.id) return;
  const start = drag;
  drag = null;
  pendingMove = null;
  screen.classList.remove("dragging");
  try { screenWrap.releasePointerCapture(e.pointerId); } catch {}

  const distCss = Math.hypot(
    (e.clientX || start.startClientX) - start.startClientX,
    (e.clientY || start.startClientY) - start.startClientY
  );
  const cancelled = e.type === "pointercancel";
  const asClick = !cancelled && (!start.moved || distCss <= 12);

  if (cancelled) {
    if (start.startPromise) {
      try { await start.startPromise; } catch {}
      if (start.started) {
        try { await sendTouch("cancel", start.lastX, start.lastY); } catch {}
      }
    }
    return;
  }

  if (asClick) {
    if (start.startPromise) {
      try { await start.startPromise; } catch {}
      if (start.started) {
        try { await sendTouch("cancel", start.startX, start.startY); } catch {}
      }
    }
    await withRemote(() => sendClick(start.startX, start.startY));
    return;
  }

  try {
    if (start.startPromise) await start.startPromise;
    if (start.started) {
      await sendTouch("end", start.lastX, start.lastY);
    } else {
      await sendClick(start.startX, start.startY);
    }
  } catch (err) {
    await syncDisplayStatus();
    setError(err);
    try { await sendTouch("cancel", start.lastX, start.lastY); } catch {}
  }
}

async function sendText() {
  const text = typeText.value;
  if (!text) return;
  try {
    await postJSON("/api/remote/type", { text });
    typeText.value = "";
    clearError();
  } catch (e) {
    setError(e);
  }
}

export function wireRemote() {
  screenWrap.addEventListener("pointerdown", (e) => {
    if (e.button !== 0) return;
    e.preventDefault();
    if (screenWrap.classList.contains("asleep")) {
      wakeDisplayOnly();
      return;
    }
    const pt = coordsFromEvent(e);
    if (!pt) return;
    drag = {
      id: e.pointerId,
      startX: pt.x,
      startY: pt.y,
      lastX: pt.x,
      lastY: pt.y,
      startClientX: e.clientX,
      startClientY: e.clientY,
      moved: false,
      started: false,
      startPromise: null,
    };
    screen.classList.add("dragging");
    screenWrap.setPointerCapture(e.pointerId);
  });

  screenWrap.addEventListener("pointermove", (e) => {
    if (!drag || e.pointerId !== drag.id) return;
    const pt = coordsFromEvent(e);
    if (!pt) return;
    drag.lastX = pt.x;
    drag.lastY = pt.y;
    const distCss = Math.hypot(e.clientX - drag.startClientX, e.clientY - drag.startClientY);
    if (distCss <= 12) return;
    if (!drag.moved) {
      const gesture = drag;
      gesture.moved = true;
      gesture.startPromise = sendTouch("start", gesture.startX, gesture.startY).then(() => {
        gesture.started = true;
        if (drag !== gesture) return;
        pendingMove = { x: gesture.lastX, y: gesture.lastY };
        flushMove();
      }).catch(setError);
      return;
    }
    if (!drag.started) return;
    pendingMove = pt;
    flushMove();
  });

  screenWrap.addEventListener("pointerup", endDrag);
  screenWrap.addEventListener("pointercancel", endDrag);

  screenWrap.addEventListener("wheel", async (e) => {
    e.preventDefault();
    if (screenWrap.classList.contains("asleep")) {
      wakeDisplayOnly();
      return;
    }
    const pt = coordsFromEvent(e);
    if (!pt) return;
    await withRemote(async () => {
      const res = await postJSON("/api/remote/wheel", {
        x: pt.x,
        y: pt.y,
        deltaX: e.deltaX,
        deltaY: e.deltaY,
      });
      clearError();
      applyRemoteState(res);
    });
  }, { passive: false });

  document.getElementById("send-text").onclick = () => sendText();
  typeText.addEventListener("keydown", (e) => {
    if (e.key === "Enter") {
      e.preventDefault();
      sendText();
    }
  });

  document.getElementById("refresh").onclick = () => refreshScreen();

  document.getElementById("wake").onclick = async () => {
    try {
      await postJSON("/api/display/wake", {});
      setAsleepUI(false);
      clearError();
      await refreshScreen();
    } catch (e) {
      setError(e);
    }
  };
  document.getElementById("sleep").onclick = async () => {
    try {
      await postJSON("/api/display/sleep", {});
      setAsleepUI(true);
      clearError();
    } catch (e) {
      setError(e);
    }
  };
}
