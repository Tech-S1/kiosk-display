package kiosk

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/Tech-S1/kiosk-display/internal/audit"
	"github.com/Tech-S1/kiosk-display/internal/config"
	"github.com/Tech-S1/kiosk-display/internal/links"
	"github.com/Tech-S1/kiosk-display/internal/policy"
)

type Result struct {
	Err      error
	Frame    []byte
	Password bool
	URL      string
}

type Cmd struct {
	Kind   string
	X, Y   float64
	DX, DY float64
	Phase  string
	Text   string
	URL    string
	result chan Result
}

type Controller struct {
	cmds      chan Cmd
	Hub       *FrameHub
	Audit     *audit.Store
	Links     *links.Store
	display   string
	sleepCmds [][]string
	wakeCmds  [][]string
	WakeCh    chan struct{}

	mu          sync.Mutex
	asleep      bool
	remoteUntil time.Time
	remoteHost  string
}

func New(store *audit.Store, linkStore *links.Store) *Controller {
	return &Controller{
		cmds:      make(chan Cmd, 64),
		Hub:       newFrameHub(),
		Audit:     store,
		Links:     linkStore,
		display:   config.C.Chrome.Display,
		sleepCmds: config.C.Display.Sleep,
		wakeCmds:  config.C.Display.Wake,
		WakeCh:    make(chan struct{}, 1),
	}
}

func (k *Controller) RecordNav(url, source string) {
	if k.IsAsleep() {
		_ = k.WakeDisplay(k.InputSource("screen"))
	}
	url = strings.TrimSpace(url)
	label := ""
	if policy.IsDisplayUI(url, config.C.Display.Port) {
		label = "Homepage"
	} else if k.Links != nil {
		if items, err := k.Links.Load(); err == nil {
			label = policy.ExactLinkLabel(url, items)
		}
	}
	k.Audit.Add(label, url, source)
}

func remoteNavHost(raw string) string {
	u := policy.ParseHTTP(raw)
	if u == nil {
		return ""
	}
	return strings.ToLower(u.Hostname())
}

func (k *Controller) MarkRemoteNav(raw string) {
	k.mu.Lock()
	k.remoteUntil = time.Now().Add(8 * time.Second)
	k.remoteHost = remoteNavHost(raw)
	k.mu.Unlock()
}

func (k *Controller) MarkRemoteInput() {
	k.mu.Lock()
	k.remoteUntil = time.Now().Add(3 * time.Second)
	k.remoteHost = ""
	k.mu.Unlock()
}

func (k *Controller) ClearRemoteNav() {
	k.mu.Lock()
	k.remoteUntil = time.Time{}
	k.remoteHost = ""
	k.mu.Unlock()
}

func (k *Controller) NavSource(raw string) string {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.remoteUntil.IsZero() || time.Now().After(k.remoteUntil) {
		k.remoteUntil = time.Time{}
		k.remoteHost = ""
		return "screen"
	}
	host := remoteNavHost(raw)
	if k.remoteHost == "" || host == k.remoteHost {
		return "remote"
	}
	k.remoteUntil = time.Time{}
	k.remoteHost = ""
	return "screen"
}

func (k *Controller) InputSource(fallback string) string {
	k.mu.Lock()
	defer k.mu.Unlock()
	if !k.remoteUntil.IsZero() && time.Now().Before(k.remoteUntil) {
		return "remote"
	}
	fallback = strings.TrimSpace(fallback)
	if fallback == "" {
		return "screen"
	}
	return fallback
}

func (k *Controller) BlockedSource(raw string) string {
	return "blocked (" + k.NavSource(raw) + ")"
}

func (k *Controller) SleepDisplay(source string) error {
	if k.IsAsleep() {
		return nil
	}
	if err := runDisplayCommands(k.display, k.sleepCmds); err != nil {
		return err
	}
	k.SetAsleep(true)
	k.recordDisplay("Display sleep", source)
	return nil
}

func (k *Controller) WakeDisplay(source string) error {
	if !k.IsAsleep() {
		return nil
	}
	if err := runDisplayCommands(k.display, k.wakeCmds); err != nil {
		return err
	}
	k.SetAsleep(false)
	k.recordDisplay("Display wake", source)
	return nil
}

func (k *Controller) recordDisplay(event, source string) {
	if k.Audit == nil {
		return
	}
	source = strings.TrimSpace(source)
	if source == "" {
		source = "display"
	}
	k.Audit.Add(event, "", source)
}

func (k *Controller) SetAsleep(v bool) {
	k.mu.Lock()
	was := k.asleep
	k.asleep = v
	k.mu.Unlock()
	k.Hub.BroadcastStatus(map[string]any{"asleep": v})
	if was && !v {
		select {
		case k.WakeCh <- struct{}{}:
		default:
		}
	}
}

func (k *Controller) IsAsleep() bool {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.asleep
}

func (k *Controller) ensureAwake() error {
	if !k.IsAsleep() {
		return nil
	}
	if err := k.WakeDisplay(k.InputSource("screen")); err != nil {
		return err
	}
	time.Sleep(300 * time.Millisecond)
	return nil
}

func (k *Controller) Request(ctx context.Context, cmd Cmd) Result {
	if err := k.ensureAwake(); err != nil {
		return Result{Err: err}
	}
	cmd.result = make(chan Result, 1)
	select {
	case k.cmds <- cmd:
	case <-ctx.Done():
		return Result{Err: ctx.Err()}
	case <-time.After(2 * time.Second):
		return Result{Err: errors.New("kiosk browser is busy")}
	}
	select {
	case res := <-cmd.result:
		return res
	case <-ctx.Done():
		return Result{Err: ctx.Err()}
	case <-time.After(30 * time.Second):
		return Result{Err: errors.New("kiosk command timed out")}
	}
}

func (k *Controller) Enqueue(cmd Cmd) {
	if err := k.ensureAwake(); err != nil {
		return
	}
	cmd.result = make(chan Result, 1)
	select {
	case k.cmds <- cmd:
	default:
	}
}
