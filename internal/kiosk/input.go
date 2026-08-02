package kiosk

import (
	"context"
	"sync"

	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/cdproto/page"

	"github.com/Tech-S1/kiosk-display/internal/config"
)

var (
	touchMu     sync.Mutex
	touchActive bool
)

func cancelActiveTouch(ctx context.Context) error {
	touchMu.Lock()
	defer touchMu.Unlock()
	if !touchActive {
		return nil
	}
	err := input.DispatchTouchEvent(input.TouchCancel, []*input.TouchPoint{}).Do(ctx)
	touchActive = false
	return err
}

func setTouchActive(v bool) {
	touchMu.Lock()
	touchActive = v
	touchMu.Unlock()
}

func isTouchActive() bool {
	touchMu.Lock()
	defer touchMu.Unlock()
	return touchActive
}

func mapRemoteXY(ctx context.Context, x, y float64) (float64, float64) {
	cfgW := float64(config.C.Chrome.Width)
	cfgH := float64(config.C.Chrome.Height)
	if cfgW < 1 || cfgH < 1 {
		return x, y
	}
	_, _, _, cssLayout, _, _, err := page.GetLayoutMetrics().Do(ctx)
	if err != nil || cssLayout == nil || cssLayout.ClientWidth < 1 || cssLayout.ClientHeight < 1 {
		return x, y
	}
	vw := float64(cssLayout.ClientWidth)
	vh := float64(cssLayout.ClientHeight)
	if vw == cfgW && vh == cfgH {
		return x, y
	}
	return x * vw / cfgW, y * vh / cfgH
}
