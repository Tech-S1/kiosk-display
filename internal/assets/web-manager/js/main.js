import { apiJSON, onUnauthorized, setError, setLoginStatus } from "./api.js";
import { ensureAuth, resetStarted, showLogin, wireAuth } from "./auth.js";
import { wireAudit } from "./audit.js";
import { loadLinks, setLinksEditable, wireLinks } from "./links.js";
import { connectStream, setScreenSize, wireRemote } from "./remote.js";
import { setTab, tabFromPath, wireTabs } from "./tabs.js";

onUnauthorized(() => {
  resetStarted();
  showLogin();
  setLoginStatus("Session expired — sign in again", true);
});

async function loadConfig() {
  try {
    const cfg = await apiJSON("/api/config");
    setScreenSize(cfg.width, cfg.height);
    setLinksEditable(!!cfg.allow_edit_links);
  } catch {}
}

wireAuth(
  () => setTab(tabFromPath(), false),
  () => {
    loadConfig();
    loadLinks().catch(setError);
    connectStream();
  }
);
wireTabs();
wireAudit();
wireLinks();
wireRemote();
ensureAuth();
