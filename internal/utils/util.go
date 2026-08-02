package utils

import (
	"os"
	"path/filepath"
)

func EnsureDataDirectory() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		home = filepath.Join(os.TempDir(), "kiosk-display")
	} else {
		home = filepath.Join(home, ".local", "share", "kiosk-display")
	}
	if err := os.MkdirAll(home, 0o700); err != nil {
		return "", err
	}
	_ = os.Chmod(home, 0o700)
	return home, nil
}
