package server

import (
	"context"
	"errors"
	"log"
	"net/http"

	"github.com/Tech-S1/kiosk-display/internal/audit"
	"github.com/Tech-S1/kiosk-display/internal/config"
	"github.com/Tech-S1/kiosk-display/internal/kiosk"
	"github.com/Tech-S1/kiosk-display/internal/links"
	"github.com/Tech-S1/kiosk-display/internal/server/display"
	"github.com/Tech-S1/kiosk-display/internal/server/manager"
	"github.com/Tech-S1/kiosk-display/internal/utils"
)

type Servers struct {
	Display *http.Server
	Manager *http.Server
}

func NewServers(store *links.Store, ctrl *kiosk.Controller, auditStore *audit.Store) (*Servers, error) {
	displayMux, err := display.NewMux(display.Options{
		Kiosk: ctrl,
		Links: store,
	})
	if err != nil {
		return nil, err
	}
	managerMux, err := manager.NewMux(manager.Options{
		HomeURL: config.C.DisplayURL(),
		Kiosk:   ctrl,
		Links:   store,
		Audit:   auditStore,
	})
	if err != nil {
		return nil, err
	}
	return &Servers{
		Display: &http.Server{Addr: config.C.DisplayListenAddr(), Handler: displayMux},
		Manager: &http.Server{Addr: config.C.ManagerListenAddr(), Handler: managerMux},
	}, nil
}

func (s *Servers) GoListenTLS(files utils.TLSFiles) {
	go func() {
		log.Printf("display listening on https://%s", config.C.DisplayListenAddr())
		if err := s.Display.ListenAndServeTLS(files.Cert, files.Key); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("display server: %v", err)
		}
	}()
	go func() {
		log.Printf("manager listening on https://%s", config.C.ManagerListenAddr())
		if err := s.Manager.ListenAndServeTLS(files.Cert, files.Key); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("manager server: %v", err)
		}
	}()
}

func (s *Servers) Shutdown(ctx context.Context) {
	_ = s.Display.Shutdown(ctx)
	_ = s.Manager.Shutdown(ctx)
}
