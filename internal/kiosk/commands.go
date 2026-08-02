package kiosk

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

func runCmd(browserCtx context.Context, cmd Cmd) Result {
	timeout := 20 * time.Second
	if cmd.Kind == "screenshot" || cmd.Kind == "focus" {
		timeout = 2 * time.Second
	}
	ctx, cancel := context.WithTimeout(browserCtx, timeout)
	defer cancel()

	switch cmd.Kind {
	case "screenshot":
		return cmdScreenshot(ctx)
	case "click":
		return cmdClick(ctx, cmd)
	case "focus":
		return cmdFocus(ctx)
	case "touch":
		return cmdTouch(ctx, cmd)
	case "wheel":
		return cmdWheel(ctx, cmd)
	case "type":
		return cmdType(ctx, cmd)
	case "navigate":
		return cmdNavigate(ctx, cmd.URL)
	case "reload":
		return cmdReload(ctx)
	case "location":
		return cmdLocation(ctx)
	default:
		return Result{Err: fmt.Errorf("unknown remote command %q", cmd.Kind)}
	}
}

func cmdScreenshot(ctx context.Context) Result {
	var img []byte
	err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		var err error
		img, err = page.CaptureScreenshot().
			WithFormat(page.CaptureScreenshotFormatJpeg).
			WithQuality(70).
			WithFromSurface(true).
			WithOptimizeForSpeed(true).
			Do(ctx)
		return err
	}))
	return Result{Err: err, Frame: img}
}

func cmdClick(ctx context.Context, cmd Cmd) Result {
	err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		if err := cancelActiveTouch(ctx); err != nil {
			return err
		}
		x, y := mapRemoteXY(ctx, cmd.X, cmd.Y)
		if err := input.DispatchMouseEvent(input.MouseMoved, x, y).Do(ctx); err != nil {
			return err
		}
		return chromedp.MouseClickXY(x, y).Do(ctx)
	}))
	return Result{Err: err}
}

func cmdFocus(ctx context.Context) Result {
	var password bool
	err := chromedp.Run(ctx, chromedp.Evaluate(`(() => {
		const el = document.activeElement;
		if (!el) return false;
		const tag = (el.tagName || '').toUpperCase();
		const type = ((el.getAttribute && el.getAttribute('type')) || el.type || '').toLowerCase();
		return tag === 'INPUT' && type === 'password';
	})()`, &password))
	return Result{Err: err, Password: password}
}

func cmdTouch(ctx context.Context, cmd Cmd) Result {
	err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		x, y := mapRemoteXY(ctx, cmd.X, cmd.Y)
		pt := []*input.TouchPoint{{X: x, Y: y, ID: 1, Force: 1}}
		switch cmd.Phase {
		case "start":
			if err := cancelActiveTouch(ctx); err != nil {
				return err
			}
			if err := input.DispatchTouchEvent(input.TouchStart, pt).Do(ctx); err != nil {
				return err
			}
			setTouchActive(true)
			return nil
		case "move":
			if !isTouchActive() {
				return nil
			}
			return input.DispatchTouchEvent(input.TouchMove, pt).Do(ctx)
		case "end", "cancel":
			if !isTouchActive() {
				return nil
			}
			typ := input.TouchEnd
			if cmd.Phase == "cancel" {
				typ = input.TouchCancel
			}
			if err := input.DispatchTouchEvent(typ, []*input.TouchPoint{}).Do(ctx); err != nil {
				return err
			}
			setTouchActive(false)
			return nil
		default:
			return fmt.Errorf("unknown touch phase %q", cmd.Phase)
		}
	}))
	return Result{Err: err}
}

func cmdWheel(ctx context.Context, cmd Cmd) Result {
	err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		if err := cancelActiveTouch(ctx); err != nil {
			return err
		}
		x, y := mapRemoteXY(ctx, cmd.X, cmd.Y)
		return input.DispatchMouseEvent(input.MouseWheel, x, y).
			WithDeltaX(cmd.DX).
			WithDeltaY(cmd.DY).
			Do(ctx)
	}))
	return Result{Err: err}
}

func cmdType(ctx context.Context, cmd Cmd) Result {
	text := sanitizeInsertText(cmd.Text)
	if text == "" {
		return Result{Err: errors.New("text required")}
	}
	err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return input.InsertText(text).Do(ctx)
	}))
	return Result{Err: err}
}

func cmdNavigate(ctx context.Context, url string) Result {
	err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		_ = cancelActiveTouch(ctx)
		return chromedp.Navigate(url).Do(ctx)
	}))
	return Result{Err: err}
}

func cmdReload(ctx context.Context) Result {
	err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		_ = cancelActiveTouch(ctx)
		return chromedp.Reload().Do(ctx)
	}))
	return Result{Err: err}
}

func cmdLocation(ctx context.Context) Result {
	var loc string
	err := chromedp.Run(ctx, chromedp.Location(&loc))
	return Result{Err: err, URL: loc}
}

func sanitizeInsertText(s string) string {
	const maxRunes = 4096
	var b strings.Builder
	b.Grow(len(s))
	n := 0
	for _, r := range s {
		if unicode.IsControl(r) {
			continue
		}
		if n >= maxRunes {
			break
		}
		b.WriteRune(r)
		n++
	}
	return b.String()
}
