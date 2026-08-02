package kiosk

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

type screencast struct {
	mu         sync.Mutex
	casting    bool
	lastFrame  time.Time
	cfg        BrowserConfig
	browserCtx context.Context
}

func (s *screencast) noteFrame() {
	s.mu.Lock()
	s.lastFrame = time.Now()
	s.mu.Unlock()
}

func (s *screencast) start() error {
	_ = chromedp.Run(s.browserCtx, page.StopScreencast())
	return chromedp.Run(s.browserCtx,
		page.StartScreencast().
			WithFormat(page.ScreencastFormatJpeg).
			WithQuality(55).
			WithEveryNthFrame(1).
			WithMaxWidth(int64(s.cfg.Width)).
			WithMaxHeight(int64(s.cfg.Height)),
	)
}

func (s *screencast) stop() {
	_ = chromedp.Run(s.browserCtx, page.StopScreencast())
}

func (s *screencast) sync(force bool) {
	want := !s.cfg.Kiosk.IsAsleep() && s.cfg.Kiosk.Hub.Viewers() > 0
	if !want {
		s.mu.Lock()
		casting := s.casting
		s.mu.Unlock()
		if casting {
			s.stop()
			s.mu.Lock()
			s.casting = false
			s.mu.Unlock()
		}
		return
	}
	s.mu.Lock()
	casting := s.casting
	s.mu.Unlock()
	if force || !casting {
		if err := s.start(); err != nil {
			log.Printf("screencast start: %v", err)
			s.mu.Lock()
			s.casting = false
			s.mu.Unlock()
			return
		}
		s.mu.Lock()
		s.casting = true
		s.mu.Unlock()
	}
}

func (s *screencast) fallback() {
	if s.cfg.Kiosk.Hub.Viewers() == 0 || s.cfg.Kiosk.IsAsleep() {
		return
	}
	s.mu.Lock()
	casting := s.casting
	var age time.Duration
	if s.lastFrame.IsZero() {
		age = time.Hour
	} else {
		age = time.Since(s.lastFrame)
	}
	s.mu.Unlock()
	if casting && age < 800*time.Millisecond {
		return
	}
	res := runCmd(s.browserCtx, Cmd{Kind: "screenshot"})
	if res.Err != nil || len(res.Frame) == 0 {
		return
	}
	s.noteFrame()
	s.cfg.Kiosk.Hub.Broadcast(res.Frame)
}
