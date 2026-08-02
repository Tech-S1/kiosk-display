package kiosk

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"

	"github.com/Tech-S1/kiosk-display/internal/config"
	"github.com/Tech-S1/kiosk-display/internal/links"
	"github.com/Tech-S1/kiosk-display/internal/policy"
)

type BrowserConfig struct {
	URL          string
	ChromePath   string
	Display      string
	Width        int
	Height       int
	RestartDelay time.Duration
	ExtensionDir string
	ProfileDir   string
	TLSClient    *tls.Config
	CertDER      string
	Kiosk        *Controller
	Links        *links.Store
}

func BrowserConfigFor(dataDir, extDir, certDER string, tlsClient *tls.Config, ctrl *Controller, store *links.Store) BrowserConfig {
	return BrowserConfig{
		URL:          config.C.DisplayURL(),
		ChromePath:   config.C.Chrome.Path,
		Display:      config.C.Chrome.Display,
		Width:        config.C.Chrome.Width,
		Height:       config.C.Chrome.Height,
		RestartDelay: config.C.Chrome.Restart,
		ExtensionDir: extDir,
		ProfileDir:   filepath.Join(dataDir, "chrome-profile"),
		TLSClient:    tlsClient,
		CertDER:      strings.TrimSpace(certDER),
		Kiosk:        ctrl,
		Links:        store,
	}
}

func (cfg BrowserConfig) pageURLAllowed(raw string) bool {
	items, _ := cfg.Links.Load()
	return policy.PageURLAllowed(raw, items, config.C.AllowedHosts, config.C.AutoAllowLinkHosts, config.C.AutoAllowSubdomains, config.C.Display.Port)
}

func ManageBrowser(ctx context.Context, cfg BrowserConfig) {
	for {
		if ctx.Err() != nil {
			return
		}
		if err := waitForDisplay(ctx, cfg.Display, config.C.Display.Wake); err != nil {
			log.Printf("display wait failed: %v", err)
			return
		}
		if err := waitForURL(ctx, cfg.URL, cfg.TLSClient); err != nil {
			log.Printf("ui wait failed: %v", err)
			return
		}
		log.Printf("starting chromium for %s", cfg.URL)
		err := runBrowser(ctx, cfg)
		if ctx.Err() != nil {
			return
		}
		log.Printf("chromium exited: %v; restarting in %s", err, cfg.RestartDelay)
		select {
		case <-ctx.Done():
			return
		case <-time.After(cfg.RestartDelay):
		}
	}
}

func runBrowser(ctx context.Context, cfg BrowserConfig) error {
	execPath := strings.TrimSpace(cfg.ChromePath)
	if st, err := os.Stat(execPath); err != nil {
		return fmt.Errorf("chrome path %q: %w", execPath, err)
	} else if st.IsDir() {
		return fmt.Errorf("chrome path %q is a directory", execPath)
	}
	log.Printf("using chrome %s", execPath)
	if err := os.MkdirAll(cfg.ProfileDir, 0o700); err != nil {
		return err
	}
	if err := configureChromeProfile(cfg.ProfileDir, cfg.CertDER); err != nil {
		return fmt.Errorf("chrome prefs: %w", err)
	}

	extDir, err := filepath.Abs(cfg.ExtensionDir)
	if err != nil {
		return err
	}

	allocCtx, allocCancel := chromedp.NewExecAllocator(ctx, chromeAllocatorOpts(cfg, execPath, extDir)...)
	defer allocCancel()

	browserCtx, browserCancel := chromedp.NewContext(allocCtx)
	defer browserCancel()

	if err := chromedp.Run(browserCtx, chromedp.Navigate(cfg.URL)); err != nil {
		return err
	}
	if err := lockViewport(browserCtx, cfg.Width, cfg.Height); err != nil {
		log.Printf("viewport lock: %v", err)
	}
	if err := applyGuards(browserCtx); err != nil {
		log.Printf("browser guards: %v", err)
	}
	_ = chromedp.Run(browserCtx, target.SetDiscoverTargets(true))

	session := newBrowserSession(browserCtx, browserCancel, cfg, resolveInitialMainID(browserCtx))
	session.listen()
	return session.run(ctx)
}
