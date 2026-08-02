package display

import (
	"net/http"

	"github.com/Tech-S1/kiosk-display/internal/assets"
	"github.com/Tech-S1/kiosk-display/internal/kiosk"
	"github.com/Tech-S1/kiosk-display/internal/links"
	"github.com/Tech-S1/kiosk-display/internal/server/common"
)

type Options struct {
	Kiosk *kiosk.Controller
	Links *links.Store
}

func NewMux(opts Options) (http.Handler, error) {
	mux := http.NewServeMux()
	common.RegisterHealth(mux)
	registerLinksGet(mux, opts.Links)
	registerDisplaySleep(mux, opts.Kiosk)
	if err := common.MountStaticSPA(mux, assets.WebDisplay, "web-display", nil); err != nil {
		return nil, err
	}
	return common.SecurityHeaders(mux), nil
}

func registerLinksGet(mux *http.ServeMux, store *links.Store) {
	mux.HandleFunc("/api/links", func(w http.ResponseWriter, r *http.Request) {
		if !common.RequireMethod(w, r, http.MethodGet) {
			return
		}
		items, err := store.Load()
		if err != nil {
			common.InternalError(w, err)
			return
		}
		common.WriteJSON(w, items)
	})
}

func registerDisplaySleep(mux *http.ServeMux, ctrl *kiosk.Controller) {
	mux.HandleFunc("/api/display/sleep", func(w http.ResponseWriter, r *http.Request) {
		if !common.RequireMethod(w, r, http.MethodPost) {
			return
		}
		if err := ctrl.SleepDisplay(ctrl.InputSource("screen")); err != nil {
			common.InternalError(w, err)
			return
		}
		common.WriteJSON(w, map[string]string{"display": "off"})
	})
	mux.HandleFunc("/api/display/wake", func(w http.ResponseWriter, r *http.Request) {
		if !common.RequireMethod(w, r, http.MethodPost) {
			return
		}
		if err := ctrl.WakeDisplay(ctrl.InputSource("screen")); err != nil {
			common.InternalError(w, err)
			return
		}
		common.WriteJSON(w, map[string]string{"display": "on"})
	})
}
