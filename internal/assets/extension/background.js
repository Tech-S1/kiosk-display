const DISPLAY_API = "https://127.0.0.1:8080";

chrome.runtime.onMessage.addListener((msg, _sender, sendResponse) => {
  if (!msg || msg.type !== "kiosk-api") return;
  (async () => {
    try {
      const headers = {};
      if (msg.body != null) {
        headers["Content-Type"] = "application/json";
      }
      const res = await fetch(DISPLAY_API + msg.path, {
        method: msg.method || "GET",
        cache: "no-store",
        headers,
        body: msg.body,
      });
      const text = await res.text();
      let data = null;
      if (text) {
        try {
          data = JSON.parse(text);
        } catch {
          data = text;
        }
      }
      sendResponse({ ok: res.ok, status: res.status, data });
    } catch (err) {
      sendResponse({ ok: false, error: String(err && err.message ? err.message : err) });
    }
  })();
  return true;
});
