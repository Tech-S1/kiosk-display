package common

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"strings"

	"github.com/Tech-S1/kiosk-display/internal/assets"
	"github.com/Tech-S1/kiosk-display/internal/config"
)

const maxJSONBody = 1 << 20

var contentSecurityPolicy = buildCSP()

func buildCSP() string {
	css, err := assets.Extension.ReadFile("extension/content.css")
	if err != nil {
		log.Printf("csp: content.css: %v", err)
	}
	sum := sha256.Sum256(css)
	styleHash := "'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'"
	return strings.Join([]string{
		"default-src 'self'",
		"img-src 'self' blob:",
		"media-src 'none'",
		"object-src 'none'",
		"base-uri 'self'",
		"form-action 'self'",
		"frame-ancestors 'none'",
		"connect-src 'self' chrome-extension:",
		"style-src 'self' " + styleHash,
		"script-src 'self'",
	}, "; ")
}

func WriteJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func ClientError(w http.ResponseWriter, status int, msg string) {
	http.Error(w, msg, status)
}

func InternalError(w http.ResponseWriter, err error) {
	if err != nil {
		log.Printf("http: %v", err)
	}
	http.Error(w, "internal error", http.StatusInternalServerError)
}

func Unavailable(w http.ResponseWriter, err error) {
	if err != nil {
		log.Printf("http: %v", err)
	}
	http.Error(w, "unavailable", http.StatusServiceUnavailable)
}

func RequireMethod(w http.ResponseWriter, r *http.Request, method string) bool {
	if r.Method == method {
		return true
	}
	ClientError(w, http.StatusMethodNotAllowed, "method not allowed")
	return false
}

func DecodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	defer r.Body.Close()
	dec := json.NewDecoder(io.LimitReader(r.Body, maxJSONBody))
	if err := dec.Decode(dst); err != nil {
		ClientError(w, http.StatusBadRequest, "invalid json")
		return false
	}
	return true
}

func RegisterHealth(mux *http.ServeMux) {
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		if !RequireMethod(w, r, http.MethodGet) {
			return
		}
		WriteJSON(w, map[string]string{"status": "ok"})
	})
}

func configValues() map[string]any {
	return map[string]any{
		"allow_edit_links": config.C.AllowEditLinks,
		"width":            config.C.Chrome.Width,
		"height":           config.C.Chrome.Height,
	}
}

func RegisterConfig(mux *http.ServeMux, keys ...string) error {
	all := configValues()
	for _, key := range keys {
		if _, ok := all[key]; !ok {
			return fmt.Errorf("unknown config key %q", key)
		}
	}
	mux.HandleFunc("/api/config", func(w http.ResponseWriter, r *http.Request) {
		if !RequireMethod(w, r, http.MethodGet) {
			return
		}
		all := configValues()
		out := make(map[string]any, len(keys))
		for _, key := range keys {
			out[key] = all[key]
		}
		WriteJSON(w, out)
	})
	return nil
}

func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", contentSecurityPolicy)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		next.ServeHTTP(w, r)
	})
}

func MountStaticSPA(mux *http.ServeMux, fsys fs.FS, root string, patchIndex func([]byte) []byte, spaPaths ...string) error {
	sub, err := fs.Sub(fsys, root)
	if err != nil {
		return err
	}
	index, err := fs.ReadFile(sub, "index.html")
	if err != nil {
		return err
	}
	if patchIndex != nil {
		index = patchIndex(index)
	}
	spa := map[string]bool{"/": true, "": true}
	for _, p := range spaPaths {
		spa[p] = true
		spa[p+"/"] = true
	}
	files := http.FileServer(http.FS(sub))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if spa[path] || path == "/index.html" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write(index)
			return
		}
		name := strings.TrimPrefix(path, "/")
		if f, err := fs.Stat(sub, name); err == nil && !f.IsDir() {
			files.ServeHTTP(w, r)
			return
		}
		http.NotFound(w, r)
	})
	return nil
}
