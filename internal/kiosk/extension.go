package kiosk

import (
	"encoding/json"
	"io"
	"io/fs"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/Tech-S1/kiosk-display/internal/assets"
	"github.com/Tech-S1/kiosk-display/internal/buildinfo"
	"github.com/Tech-S1/kiosk-display/internal/config"
)

const extensionDisplayAPIPlaceholder = "https://127.0.0.1:8080"

func ExtractExtension(dataDir string) (dir string, err error) {
	dir = filepath.Join(dataDir, "extension")
	displayAPI := config.C.DisplayAPIBase()
	if err := resetDir(dir); err != nil {
		return "", err
	}
	if err := fs.WalkDir(assets.Extension, "extension", func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel("extension", path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		return writeExtensionEntry(filepath.Join(dir, rel), path, d, displayAPI)
	}); err != nil {
		return "", err
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	return abs, nil
}

func resetDir(dir string) error {
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	return os.MkdirAll(dir, 0o755)
}

func writeExtensionEntry(target, source string, d fs.DirEntry, displayAPI string) error {
	if d.IsDir() {
		return os.MkdirAll(target, 0o755)
	}

	in, err := assets.Extension.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}

	b, err := io.ReadAll(in)
	if err != nil {
		return err
	}
	return os.WriteFile(target, patchExtensionFile(source, b, displayAPI), 0o644)
}

func patchExtensionFile(source string, b []byte, displayAPI string) []byte {
	if filepath.Base(source) == "manifest.json" {
		if patched, ok := patchManifest(b, displayAPI); ok {
			return patched
		}
	}
	return patchPlaceholders(b, displayAPI)
}

func patchManifest(b []byte, displayAPI string) ([]byte, bool) {
	var manifest map[string]any
	if err := json.Unmarshal(b, &manifest); err != nil {
		return nil, false
	}
	manifest["host_permissions"] = displayAPIHostPerms(displayAPI)
	manifest["version"] = chromeExtensionVersion(buildinfo.Version)
	out, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, false
	}
	return append(out, '\n'), true
}

func chromeExtensionVersion(v string) string {
	v = strings.TrimSpace(strings.TrimPrefix(v, "v"))
	if v == "" || v == "dev" {
		return "0.0.0"
	}
	return v
}

func displayAPIHostPerms(apiBase string) []string {
	u, err := url.Parse(apiBase)
	if err != nil {
		return nil
	}
	port := u.Port()
	host := u.Hostname()
	hostPort := net.JoinHostPort(host, port)
	perms := []string{
		"https://" + hostPort + "/*",
		"http://" + hostPort + "/*",
	}
	if host != "localhost" {
		perms = append(perms, "https://localhost:"+port+"/*", "http://localhost:"+port+"/*")
	}
	if host != "127.0.0.1" {
		perms = append(perms, "https://127.0.0.1:"+port+"/*", "http://127.0.0.1:"+port+"/*")
	}
	return perms
}

func patchPlaceholders(b []byte, displayAPI string) []byte {
	s := strings.ReplaceAll(string(b), extensionDisplayAPIPlaceholder, displayAPI)
	return []byte(s)
}
