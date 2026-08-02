package kiosk

import (
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	goruntime "runtime"

	"github.com/chromedp/chromedp"
)

func chromeAllocatorOpts(cfg BrowserConfig, execPath, extDir string) []chromedp.ExecAllocatorOption {
	opts := []chromedp.ExecAllocatorOption{
		chromedp.ExecPath(execPath),
		chromedp.UserDataDir(cfg.ProfileDir),
		chromedp.NoFirstRun,
		chromedp.NoDefaultBrowserCheck,
		chromedp.Flag("disable-extensions", false),
		chromedp.Flag("load-extension", extDir),
		chromedp.Flag("disable-extensions-except", extDir),
		chromedp.Flag("disable-blink-features", "AutomationControlled"),
		chromedp.Flag("enable-automation", false),
		chromedp.Flag("kiosk", true),
		chromedp.Flag("start-fullscreen", true),
		chromedp.Flag("disable-infobars", true),
		chromedp.Flag("disable-session-crashed-bubble", true),
		chromedp.Flag("disable-dev-tools", true),
		chromedp.Flag("disable-print-preview", true),
		chromedp.Flag("disable-background-networking", true),
		chromedp.Flag("disable-background-timer-throttling", true),
		chromedp.Flag("disable-backgrounding-occluded-windows", true),
		chromedp.Flag("disable-breakpad", true),
		chromedp.Flag("disable-client-side-phishing-detection", true),
		chromedp.Flag("disable-component-extensions-with-background-pages", true),
		chromedp.Flag("disable-domain-reliability", true),
		chromedp.Flag("disable-hang-monitor", true),
		chromedp.Flag("disable-prompt-on-repost", true),
		chromedp.Flag("disable-renderer-backgrounding", true),
		chromedp.Flag("disable-search-engine-choice-screen", true),
		chromedp.Flag("disable-features", "TranslateUI,Translate,MediaRouter,ChromeWhatsNewUI,AutosaveDOMInfo,InterestFeedContentSuggestions,GlobalMediaControls,PasswordImport,AutofillServerCommunication,DialMediaRouteProvider"),
		chromedp.Flag("deny-permission-prompts", true),
		chromedp.Flag("disable-notifications", true),
		chromedp.Flag("noerrdialogs", true),
		chromedp.Flag("disable-sync", true),
		chromedp.Flag("disable-component-update", true),
		chromedp.Flag("disable-default-apps", true),
		chromedp.Flag("password-store", "basic"),
		chromedp.Flag("check-for-update-interval", "31536000"),
		chromedp.Flag("window-position", "0,0"),
		chromedp.WindowSize(cfg.Width, cfg.Height),
	}
	if spki := certSPKIHash(cfg.CertDER); spki != "" {
		opts = append(opts, chromedp.Flag("ignore-certificate-errors-spki-list", spki))
	}
	if goruntime.GOOS != "darwin" {
		opts = append(opts,
			chromedp.Flag("disable-dev-shm-usage", true),
			chromedp.Env("DISPLAY="+cfg.Display),
		)
	}
	return opts
}

func certSPKIHash(certDERBase64 string) string {
	der, err := base64.StdEncoding.DecodeString(certDERBase64)
	if err != nil || len(der) == 0 {
		return ""
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil || len(cert.RawSubjectPublicKeyInfo) == 0 {
		return ""
	}
	sum := sha256.Sum256(cert.RawSubjectPublicKeyInfo)
	return base64.StdEncoding.EncodeToString(sum[:])
}
