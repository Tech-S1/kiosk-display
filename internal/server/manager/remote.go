package manager

import (
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/Tech-S1/kiosk-display/internal/config"
	"github.com/Tech-S1/kiosk-display/internal/kiosk"
	"github.com/Tech-S1/kiosk-display/internal/links"
	"github.com/Tech-S1/kiosk-display/internal/policy"
	"github.com/Tech-S1/kiosk-display/internal/server/common"
)

const maxTypeRunes = 4096

func remoteOK(w http.ResponseWriter, ctrl *kiosk.Controller, extra map[string]any) {
	out := map[string]any{"status": "ok", "asleep": ctrl.IsAsleep()}
	for k, v := range extra {
		out[k] = v
	}
	common.WriteJSON(w, out)
}

func remoteRequest(w http.ResponseWriter, r *http.Request, ctrl *kiosk.Controller, cmd kiosk.Cmd, extra map[string]any) {
	res := ctrl.Request(r.Context(), cmd)
	if res.Err != nil {
		common.Unavailable(w, res.Err)
		return
	}
	remoteOK(w, ctrl, extra)
}

func registerRemoteAPI(mux *http.ServeMux, ctrl *kiosk.Controller, homeURL string, store *links.Store) {
	mux.HandleFunc("/api/remote/stream", func(w http.ResponseWriter, r *http.Request) {
		serveRemoteStream(w, r, ctrl)
	})

	mux.HandleFunc("/api/remote/screenshot", func(w http.ResponseWriter, r *http.Request) {
		if !common.RequireMethod(w, r, http.MethodGet) {
			return
		}
		res := ctrl.Request(r.Context(), kiosk.Cmd{Kind: "screenshot"})
		if res.Err != nil {
			common.Unavailable(w, res.Err)
			return
		}
		w.Header().Set("Content-Type", "image/jpeg")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(res.Frame)
	})

	mux.HandleFunc("/api/remote/click", func(w http.ResponseWriter, r *http.Request) {
		if !common.RequireMethod(w, r, http.MethodPost) {
			return
		}
		var body struct {
			X float64 `json:"x"`
			Y float64 `json:"y"`
		}
		if !common.DecodeJSON(w, r, &body) {
			return
		}
		ctrl.MarkRemoteInput()
		res := ctrl.Request(r.Context(), kiosk.Cmd{Kind: "click", X: body.X, Y: body.Y})
		if res.Err != nil {
			common.Unavailable(w, res.Err)
			return
		}
		remoteOK(w, ctrl, nil)
	})

	mux.HandleFunc("/api/remote/type", func(w http.ResponseWriter, r *http.Request) {
		if !common.RequireMethod(w, r, http.MethodPost) {
			return
		}
		var body struct {
			Text string `json:"text"`
		}
		if !common.DecodeJSON(w, r, &body) {
			return
		}
		if strings.TrimSpace(body.Text) == "" {
			common.ClientError(w, http.StatusBadRequest, "text required")
			return
		}
		if utf8.RuneCountInString(body.Text) > maxTypeRunes {
			common.ClientError(w, http.StatusBadRequest, "text too long")
			return
		}
		ctrl.MarkRemoteInput()
		remoteRequest(w, r, ctrl, kiosk.Cmd{Kind: "type", Text: body.Text}, nil)
	})

	mux.HandleFunc("/api/remote/touch", func(w http.ResponseWriter, r *http.Request) {
		if !common.RequireMethod(w, r, http.MethodPost) {
			return
		}
		var body struct {
			Phase string  `json:"phase"`
			X     float64 `json:"x"`
			Y     float64 `json:"y"`
		}
		if !common.DecodeJSON(w, r, &body) {
			return
		}
		body.Phase = strings.TrimSpace(strings.ToLower(body.Phase))
		switch body.Phase {
		case "start", "move", "end", "cancel":
		default:
			common.ClientError(w, http.StatusBadRequest, `phase must be "start", "move", "end", or "cancel"`)
			return
		}
		cmd := kiosk.Cmd{Kind: "touch", Phase: body.Phase, X: body.X, Y: body.Y}
		if body.Phase == "move" {
			ctrl.Enqueue(cmd)
			remoteOK(w, ctrl, nil)
			return
		}
		if body.Phase == "start" || body.Phase == "end" {
			ctrl.MarkRemoteInput()
		}
		remoteRequest(w, r, ctrl, cmd, nil)
	})

	mux.HandleFunc("/api/remote/wheel", func(w http.ResponseWriter, r *http.Request) {
		if !common.RequireMethod(w, r, http.MethodPost) {
			return
		}
		var body struct {
			X      float64 `json:"x"`
			Y      float64 `json:"y"`
			DeltaX float64 `json:"deltaX"`
			DeltaY float64 `json:"deltaY"`
		}
		if !common.DecodeJSON(w, r, &body) {
			return
		}
		if body.DeltaX == 0 && body.DeltaY == 0 {
			common.ClientError(w, http.StatusBadRequest, "deltaX or deltaY required")
			return
		}
		remoteRequest(w, r, ctrl, kiosk.Cmd{Kind: "wheel", X: body.X, Y: body.Y, DX: body.DeltaX, DY: body.DeltaY}, nil)
	})

	mux.HandleFunc("/api/remote/navigate", func(w http.ResponseWriter, r *http.Request) {
		if !common.RequireMethod(w, r, http.MethodPost) {
			return
		}
		var body struct {
			URL string `json:"url"`
		}
		if !common.DecodeJSON(w, r, &body) {
			return
		}
		body.URL = strings.TrimSpace(body.URL)
		if body.URL == "" {
			body.URL = homeURL
		}
		items, err := store.Load()
		if err != nil {
			common.InternalError(w, err)
			return
		}
		if !policy.NavURLAllowed(body.URL, items, config.C.AllowedHosts, config.C.AutoAllowLinkHosts, config.C.AutoAllowSubdomains, config.C.Display.Port) {
			ctrl.RecordNav(body.URL, "blocked (remote)")
			common.ClientError(w, http.StatusForbidden, "URL not allowed")
			return
		}
		ctrl.MarkRemoteNav(body.URL)
		res := ctrl.Request(r.Context(), kiosk.Cmd{Kind: "navigate", URL: body.URL})
		if res.Err != nil {
			ctrl.ClearRemoteNav()
			common.Unavailable(w, res.Err)
			return
		}
		remoteOK(w, ctrl, map[string]any{"url": body.URL})
	})

	mux.HandleFunc("/api/remote/reload", func(w http.ResponseWriter, r *http.Request) {
		if !common.RequireMethod(w, r, http.MethodPost) {
			return
		}
		remoteRequest(w, r, ctrl, kiosk.Cmd{Kind: "reload"}, nil)
	})
}
