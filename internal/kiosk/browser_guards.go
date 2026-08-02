package kiosk

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/chromedp/cdproto/browser"
	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"
)

func applyGuards(ctx context.Context) error {
	c := chromedp.FromContext(ctx)
	if c == nil || c.Browser == nil {
		return errors.New("browser not ready")
	}
	bctx := cdp.WithExecutor(ctx, c.Browser)
	if err := browser.SetDownloadBehavior(browser.SetDownloadBehaviorBehaviorDeny).
		WithEventsEnabled(true).
		Do(bctx); err != nil {
		return err
	}
	return chromedp.Run(ctx,
		page.SetInterceptFileChooserDialog(true).WithCancel(true),
		network.SetBypassServiceWorker(true),
		fetch.Enable().WithPatterns([]*fetch.RequestPattern{{
			ResourceType: network.ResourceTypeDocument,
			RequestStage: fetch.RequestStageRequest,
		}}),
	)
}

func lockViewport(ctx context.Context, width, height int) error {
	return chromedp.Run(ctx,
		emulation.SetDeviceMetricsOverride(int64(width), int64(height), 1, false),
	)
}

func blankURL(u string) bool {
	u = strings.TrimSpace(u)
	return u == "" || strings.HasPrefix(u, "about:blank")
}

func pageTargets(all []*target.Info) []*target.Info {
	pages := make([]*target.Info, 0, len(all))
	for _, t := range all {
		if t.Type == "page" {
			pages = append(pages, t)
		}
	}
	return pages
}

func hasTarget(pages []*target.Info, id target.ID) bool {
	for _, t := range pages {
		if t.TargetID == id {
			return true
		}
	}
	return false
}

func resolveMainID(c *chromedp.Context, pages []*target.Info, mainID target.ID, setMain func(target.ID)) (target.ID, bool) {
	if c.Target != nil {
		attached := c.Target.TargetID
		if !hasTarget(pages, attached) {
			return mainID, true
		}
		if mainID == "" || mainID != attached {
			mainID = attached
			setMain(mainID)
		}
		return mainID, false
	}
	if mainID == "" && len(pages) > 0 {
		mainID = pages[0].TargetID
		setMain(mainID)
	}
	return mainID, false
}

func enforceSingleTab(ctx context.Context, cfg BrowserConfig, mainID target.ID, setMain func(target.ID), onDenied func(string)) (restart bool) {
	targets, err := chromedp.Targets(ctx)
	if err != nil {
		return false
	}
	c := chromedp.FromContext(ctx)
	if c == nil || c.Browser == nil {
		return false
	}
	pages := pageTargets(targets)
	if len(pages) == 0 {
		return true
	}
	mainID, lost := resolveMainID(c, pages, mainID, setMain)
	if lost {
		return true
	}

	tctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	exec := cdp.WithExecutor(tctx, c.Browser)

	var adopt, blockedMain string
	for _, t := range pages {
		u := strings.TrimSpace(t.URL)
		if t.TargetID == mainID {
			if !cfg.pageURLAllowed(u) {
				blockedMain = u
				cfg.Kiosk.RecordNav(u, cfg.Kiosk.BlockedSource(u))
			}
			continue
		}
		if blankURL(u) {
			continue
		}
		if cfg.pageURLAllowed(u) {
			if adopt == "" {
				adopt = u
			}
		} else {
			cfg.Kiosk.RecordNav(u, cfg.Kiosk.BlockedSource(u))
			onDenied(u)
		}
		_ = target.CloseTarget(t.TargetID).Do(exec)
	}

	switch {
	case adopt != "":
		_ = chromedp.Run(ctx, chromedp.Navigate(adopt))
	case blockedMain != "":
		_ = chromedp.Run(ctx, chromedp.Navigate(cfg.URL))
		go func() {
			time.Sleep(400 * time.Millisecond)
			onDenied(blockedMain)
		}()
	}
	return false
}
