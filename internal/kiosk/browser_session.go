package kiosk

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/browser"
	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"
)

type browserSession struct {
	cfg          BrowserConfig
	browserCtx   context.Context
	cancel       context.CancelFunc
	cast         *screencast
	reapplyCh    chan struct{}
	denyCh       chan struct{}
	mainMu       sync.Mutex
	mainID       target.ID
	lastPassword *bool
}

func newBrowserSession(browserCtx context.Context, cancel context.CancelFunc, cfg BrowserConfig, mainID target.ID) *browserSession {
	return &browserSession{
		cfg:        cfg,
		browserCtx: browserCtx,
		cancel:     cancel,
		cast:       &screencast{cfg: cfg, browserCtx: browserCtx},
		reapplyCh:  make(chan struct{}, 1),
		denyCh:     make(chan struct{}, 1),
		mainID:     mainID,
	}
}

func (s *browserSession) getMainID() target.ID {
	s.mainMu.Lock()
	defer s.mainMu.Unlock()
	return s.mainID
}

func (s *browserSession) setMainID(id target.ID) {
	s.mainMu.Lock()
	s.mainID = id
	s.mainMu.Unlock()
}

func (s *browserSession) requestReapply() {
	select {
	case s.reapplyCh <- struct{}{}:
	default:
	}
}

func (s *browserSession) requestDeny() {
	select {
	case s.denyCh <- struct{}{}:
	default:
	}
}

func (s *browserSession) notifyDenied(raw string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return
	}
	payload, err := json.Marshal(raw)
	if err != nil {
		return
	}
	script := fmt.Sprintf(`(() => {
		const url = %s;
		let name = url;
		try { name = new URL(url).hostname || name; } catch (e) {}
		window.dispatchEvent(new CustomEvent("kiosk-denied", { detail: { host: name } }));
		})()`, string(payload))
	go func() {
		_ = chromedp.Run(s.browserCtx, chromedp.Evaluate(script, nil))
	}()
}

func (s *browserSession) closeExtraTarget(id target.ID) {
	main := s.getMainID()
	if id == "" || id == main {
		return
	}
	c := chromedp.FromContext(s.browserCtx)
	if c == nil || c.Browser == nil {
		return
	}
	tctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = target.CloseTarget(id).Do(cdp.WithExecutor(tctx, c.Browser))
}

func (s *browserSession) recordAndMaybeAdopt(raw string) {
	raw = strings.TrimSpace(raw)
	if blankURL(raw) {
		return
	}
	if !s.cfg.pageURLAllowed(raw) {
		s.cfg.Kiosk.RecordNav(raw, s.cfg.Kiosk.BlockedSource(raw))
		s.notifyDenied(raw)
		return
	}
	go func() {
		_ = chromedp.Run(s.browserCtx, chromedp.Navigate(raw))
		s.requestReapply()
	}()
}

func (s *browserSession) handleExtraPage(id target.ID, url string, closeNow bool) {
	url = strings.TrimSpace(url)
	if !blankURL(url) {
		s.recordAndMaybeAdopt(url)
		go s.closeExtraTarget(id)
	} else if closeNow {
		go s.closeExtraTarget(id)
	}
	s.requestDeny()
}

func (s *browserSession) blockURL(raw string) {
	s.cfg.Kiosk.RecordNav(raw, s.cfg.Kiosk.BlockedSource(raw))
	s.notifyDenied(raw)
	s.requestDeny()
}

func (s *browserSession) listen() {
	chromedp.ListenBrowser(s.browserCtx, s.onBrowserEvent)
	chromedp.ListenTarget(s.browserCtx, s.onTargetEvent)
}

func (s *browserSession) onBrowserEvent(ev any) {
	switch e := ev.(type) {
	case *target.EventTargetCreated:
		if e.TargetInfo == nil {
			return
		}
		switch e.TargetInfo.Type {
		case "devtools":
			go s.closeExtraTarget(e.TargetInfo.TargetID)
		case "page":
			main := s.getMainID()
			if main == "" {
				s.setMainID(e.TargetInfo.TargetID)
				return
			}
			if e.TargetInfo.TargetID != main {
				s.handleExtraPage(e.TargetInfo.TargetID, e.TargetInfo.URL, false)
			}
		}
	case *target.EventTargetInfoChanged:
		if e.TargetInfo == nil || e.TargetInfo.Type != "page" {
			return
		}
		main := s.getMainID()
		if main != "" && e.TargetInfo.TargetID != main {
			s.handleExtraPage(e.TargetInfo.TargetID, e.TargetInfo.URL, true)
			return
		}
		if !s.cfg.pageURLAllowed(e.TargetInfo.URL) {
			s.blockURL(e.TargetInfo.URL)
		}
	case *target.EventTargetDestroyed:
		if e.TargetID == s.getMainID() {
			s.setMainID("")
			s.requestDeny()
		}
	case *browser.EventDownloadWillBegin:
		guid := e.GUID
		go func() {
			c := chromedp.FromContext(s.browserCtx)
			if c == nil || c.Browser == nil {
				return
			}
			tctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_ = browser.CancelDownload(guid).Do(cdp.WithExecutor(tctx, c.Browser))
		}()
	}
}

func (s *browserSession) onTargetEvent(ev any) {
	switch e := ev.(type) {
	case *page.EventScreencastFrame:
		sessionID := e.SessionID
		go func() {
			_ = chromedp.Run(s.browserCtx, page.ScreencastFrameAck(sessionID))
		}()
		if s.cfg.Kiosk.IsAsleep() || s.cfg.Kiosk.Hub.Viewers() == 0 {
			return
		}
		data, err := base64.StdEncoding.DecodeString(e.Data)
		if err != nil || len(data) == 0 {
			return
		}
		s.cast.noteFrame()
		s.cfg.Kiosk.Hub.Broadcast(data)
	case *page.EventFrameNavigated:
		if e.Frame == nil || e.Frame.ParentID != "" {
			return
		}
		if s.cfg.pageURLAllowed(e.Frame.URL) {
			s.cfg.Kiosk.RecordNav(e.Frame.URL, s.cfg.Kiosk.NavSource(e.Frame.URL))
		} else {
			s.blockURL(e.Frame.URL)
		}
		s.requestReapply()
	case *page.EventLoadEventFired:
		s.requestReapply()
	case *page.EventJavascriptDialogOpening:
		accept := e.Type == page.DialogTypeBeforeunload
		go func() {
			_ = chromedp.Run(s.browserCtx, page.HandleJavaScriptDialog(accept))
		}()
	case *fetch.EventRequestPaused:
		req := e
		go s.handleFetchPaused(req)
	}
}

func (s *browserSession) handleFetchPaused(ev *fetch.EventRequestPaused) {
	raw := ""
	if ev.Request != nil {
		raw = ev.Request.URL
	}
	if s.cfg.pageURLAllowed(raw) {
		_ = chromedp.Run(s.browserCtx, fetch.ContinueRequest(ev.RequestID))
		return
	}
	s.cfg.Kiosk.RecordNav(raw, s.cfg.Kiosk.BlockedSource(raw))
	s.notifyDenied(raw)
	_ = chromedp.Run(s.browserCtx, fetch.FailRequest(ev.RequestID, network.ErrorReasonAccessDenied))
	s.requestDeny()
}

func (s *browserSession) publishFocus() {
	if s.cfg.Kiosk.Hub.Viewers() == 0 || s.cfg.Kiosk.IsAsleep() {
		return
	}
	res := runCmd(s.browserCtx, Cmd{Kind: "focus"})
	if res.Err != nil {
		return
	}
	if s.lastPassword != nil && *s.lastPassword == res.Password {
		return
	}
	v := res.Password
	s.lastPassword = &v
	s.cfg.Kiosk.Hub.BroadcastStatus(map[string]any{"password": res.Password})
}

func (s *browserSession) lostTab() bool {
	return enforceSingleTab(s.browserCtx, s.cfg, s.getMainID(), s.setMainID, s.notifyDenied)
}

func (s *browserSession) run(ctx context.Context) error {
	ticker := time.NewTicker(500 * time.Millisecond)
	denyTicker := time.NewTicker(150 * time.Millisecond)
	focusTicker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	defer denyTicker.Stop()
	defer focusTicker.Stop()
	defer s.cast.stop()
	s.cast.sync(true)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-s.browserCtx.Done():
			if err := context.Cause(s.browserCtx); err != nil && !errors.Is(err, context.Canceled) {
				return err
			}
			return errors.New("browser context closed")
		case cmd := <-s.cfg.Kiosk.cmds:
			cmd.result <- runCmd(s.browserCtx, cmd)
			if cmd.Kind == "navigate" || cmd.Kind == "reload" {
				s.requestReapply()
			}
			if cmd.Kind == "click" || cmd.Kind == "type" || cmd.Kind == "navigate" || cmd.Kind == "reload" {
				s.lastPassword = nil
				s.publishFocus()
			}
		case <-s.reapplyCh:
			_ = lockViewport(s.browserCtx, s.cfg.Width, s.cfg.Height)
			_ = applyGuards(s.browserCtx)
			s.cast.sync(true)
			s.cast.fallback()
		case <-s.cfg.Kiosk.WakeCh:
			s.cast.sync(true)
			s.cast.fallback()
		case <-s.denyCh:
			_ = applyGuards(s.browserCtx)
			if s.lostTab() {
				log.Printf("kiosk tab session lost; restarting chromium")
				s.cancel()
				return errors.New("kiosk tab session lost")
			}
		case <-denyTicker.C:
			if s.lostTab() {
				log.Printf("kiosk tab session lost; restarting chromium")
				s.cancel()
				return errors.New("kiosk tab session lost")
			}
		case <-ticker.C:
			s.cast.sync(false)
			s.cast.fallback()
		case <-focusTicker.C:
			s.publishFocus()
		}
	}
}

func resolveInitialMainID(browserCtx context.Context) target.ID {
	if pages, err := chromedp.Targets(browserCtx); err == nil {
		for _, t := range pages {
			if t.Type == "page" {
				return t.TargetID
			}
		}
	}
	if c := chromedp.FromContext(browserCtx); c != nil && c.Target != nil {
		return c.Target.TargetID
	}
	return ""
}
