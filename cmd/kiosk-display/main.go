package main

import (
	"context"
	"crypto/tls"
	"log"
	"os/signal"
	"syscall"
	"time"

	"github.com/Tech-S1/kiosk-display/internal/audit"
	"github.com/Tech-S1/kiosk-display/internal/config"
	"github.com/Tech-S1/kiosk-display/internal/kiosk"
	"github.com/Tech-S1/kiosk-display/internal/links"
	"github.com/Tech-S1/kiosk-display/internal/server"
	"github.com/Tech-S1/kiosk-display/internal/utils"
)

func main() {
	loadConfig()
	dataDir := loadDataDir()
	tlsFiles := loadTLSFiles(dataDir)
	certDER, tlsClient := loadTLSClient(tlsFiles.Cert)
	extDir := loadExtension(dataDir)
	store := loadLinksStore(dataDir)
	auditStore := loadAuditStore(dataDir)
	ctrl := loadKiosk(auditStore, store)
	srvs := loadServers(store, ctrl, auditStore)

	rootCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	srvs.GoListenTLS(tlsFiles)
	go kiosk.ManageBrowser(rootCtx, kiosk.BrowserConfigFor(dataDir, extDir, certDER, tlsClient, ctrl, store))

	<-rootCtx.Done()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	srvs.Shutdown(ctx)
}

func loadConfig() {
	if err := config.Load(); err != nil {
		log.Fatalf("config: %v", err)
	}
	log.Printf("config %s", config.Path)
}

func loadDataDir() string {
	dataDir, err := utils.EnsureDataDirectory()
	if err != nil {
		log.Fatalf("data dir: %v", err)
	}
	return dataDir
}

func loadTLSFiles(dataDir string) utils.TLSFiles {
	hosts := []string{config.C.Manager.Host, config.C.DisplayHost()}
	tlsFiles, err := utils.EnsureSelfSigned(dataDir, hosts)
	if err != nil {
		log.Fatalf("tls cert: %v", err)
	}
	log.Printf("tls cert %s", tlsFiles.Cert)
	return tlsFiles
}

func loadTLSClient(certFile string) (string, *tls.Config) {
	certDER, err := utils.CertDERBase64(certFile)
	if err != nil {
		log.Fatalf("tls cert der: %v", err)
	}
	tlsClient, err := utils.ClientConfig(certFile)
	if err != nil {
		log.Fatalf("tls client: %v", err)
	}
	return certDER, tlsClient
}

func loadExtension(dataDir string) string {
	extDir, err := kiosk.ExtractExtension(dataDir)
	if err != nil {
		log.Fatalf("extension extract: %v", err)
	}
	log.Printf("kiosk HUD extension at %s", extDir)
	return extDir
}

func loadLinksStore(dataDir string) *links.Store {
	store, err := links.Open(dataDir)
	if err != nil {
		log.Fatalf("links store: %v", err)
	}
	return store
}

func loadAuditStore(dataDir string) *audit.Store {
	store, err := audit.Open(dataDir)
	if err != nil {
		log.Fatalf("audit log: %v", err)
	}
	return store
}

func loadKiosk(auditStore *audit.Store, linkStore *links.Store) *kiosk.Controller {
	return kiosk.New(auditStore, linkStore)
}

func loadServers(store *links.Store, ctrl *kiosk.Controller, auditStore *audit.Store) *server.Servers {
	srvs, err := server.NewServers(store, ctrl, auditStore)
	if err != nil {
		log.Fatalf("servers: %v", err)
	}
	return srvs
}
