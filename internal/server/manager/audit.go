package manager

import (
	"net/http"
	"strconv"

	"github.com/Tech-S1/kiosk-display/internal/audit"
	"github.com/Tech-S1/kiosk-display/internal/server/common"
)

func registerAudit(mux *http.ServeMux, store *audit.Store) {
	mux.HandleFunc("/api/audit", func(w http.ResponseWriter, r *http.Request) {
		if !common.RequireMethod(w, r, http.MethodGet) {
			return
		}
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
		filter := r.URL.Query().Get("filter")
		common.WriteJSON(w, store.List(page, pageSize, filter))
	})
}
