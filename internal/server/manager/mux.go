package manager

import (
	"context"
	"net/http"
	"strings"

	"github.com/Tech-S1/kiosk-display/internal/assets"
	"github.com/Tech-S1/kiosk-display/internal/audit"
	"github.com/Tech-S1/kiosk-display/internal/config"
	"github.com/Tech-S1/kiosk-display/internal/kiosk"
	"github.com/Tech-S1/kiosk-display/internal/links"
	"github.com/Tech-S1/kiosk-display/internal/policy"
	"github.com/Tech-S1/kiosk-display/internal/server/common"
)

type Options struct {
	HomeURL string
	Kiosk   *kiosk.Controller
	Links   *links.Store
	Audit   *audit.Store
}

var configKeys = []string{"width", "height", "allow_edit_links"}

func NewMux(opts Options) (http.Handler, error) {
	mux := http.NewServeMux()
	sessions := newSessionStore()
	oidc, err := initOIDC(context.Background())
	if err != nil {
		return nil, err
	}
	registerAuth(mux, sessions, oidc)
	common.RegisterHealth(mux)
	if err := common.RegisterConfig(mux, configKeys...); err != nil {
		return nil, err
	}
	registerLinks(mux, opts.Links, opts.Kiosk, opts.HomeURL)
	registerDisplayControl(mux, opts.Kiosk)
	registerRemoteAPI(mux, opts.Kiosk, opts.HomeURL, opts.Links)
	registerAudit(mux, opts.Audit)
	if err := common.MountStaticSPA(mux, assets.WebManager, "web-manager", nil, "/audit"); err != nil {
		return nil, err
	}
	return common.SecurityHeaders(requireAuth(sessions, mux)), nil
}

func registerLinks(mux *http.ServeMux, store *links.Store, ctrl *kiosk.Controller, homeURL string) {
	mux.HandleFunc("/api/links", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			items, err := store.Load()
			if err != nil {
				common.InternalError(w, err)
				return
			}
			common.WriteJSON(w, items)
		case http.MethodPut:
			if !config.C.AllowEditLinks {
				common.ClientError(w, http.StatusForbidden, "editing links is disabled")
				return
			}
			var items []links.Item
			if !common.DecodeJSON(w, r, &items) {
				return
			}
			cleaned, ok := cleanLinkItems(w, items)
			if !ok {
				return
			}
			if err := store.Save(cleaned); err != nil {
				common.InternalError(w, err)
				return
			}
			reloadHomeIfNeeded(r.Context(), ctrl, homeURL)
			common.WriteJSON(w, cleaned)
		default:
			common.ClientError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	})
}

func cleanLinkItems(w http.ResponseWriter, items []links.Item) ([]links.Item, bool) {
	cleaned := make([]links.Item, 0, len(items))
	for _, item := range items {
		item.Label = strings.TrimSpace(item.Label)
		item.URL = strings.TrimSpace(item.URL)
		if item.Label == "" || item.URL == "" {
			continue
		}
		if policy.ParseHTTP(item.URL) == nil {
			common.ClientError(w, http.StatusBadRequest, "links must use http or https URLs")
			return nil, false
		}
		cleaned = append(cleaned, item)
	}
	return cleaned, true
}

func reloadHomeIfNeeded(ctx context.Context, ctrl *kiosk.Controller, homeURL string) {
	if ctrl.IsAsleep() {
		return
	}
	loc := ctrl.Request(ctx, kiosk.Cmd{Kind: "location"})
	if loc.Err != nil || !sameHomePage(loc.URL, homeURL) {
		return
	}
	_ = ctrl.Request(ctx, kiosk.Cmd{Kind: "reload"})
}

func sameHomePage(current, home string) bool {
	a := strings.TrimRight(strings.ToLower(strings.TrimSpace(current)), "/")
	b := strings.TrimRight(strings.ToLower(strings.TrimSpace(home)), "/")
	return a != "" && a == b
}

func registerDisplayControl(mux *http.ServeMux, ctrl *kiosk.Controller) {
	mux.HandleFunc("/api/display/status", func(w http.ResponseWriter, r *http.Request) {
		if !common.RequireMethod(w, r, http.MethodGet) {
			return
		}
		common.WriteJSON(w, map[string]any{"asleep": ctrl.IsAsleep()})
	})
	mux.HandleFunc("/api/display/sleep", func(w http.ResponseWriter, r *http.Request) {
		if !common.RequireMethod(w, r, http.MethodPost) {
			return
		}
		if err := ctrl.SleepDisplay("remote"); err != nil {
			common.InternalError(w, err)
			return
		}
		common.WriteJSON(w, map[string]string{"display": "off"})
	})
	mux.HandleFunc("/api/display/wake", func(w http.ResponseWriter, r *http.Request) {
		if !common.RequireMethod(w, r, http.MethodPost) {
			return
		}
		if err := ctrl.WakeDisplay("remote"); err != nil {
			common.InternalError(w, err)
			return
		}
		common.WriteJSON(w, map[string]string{"display": "on"})
	})
}
