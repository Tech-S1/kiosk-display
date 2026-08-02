package links

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"github.com/Tech-S1/kiosk-display/internal/config"
)

type Item = config.Link

type Store struct {
	mu   sync.Mutex
	path string
}

func Open(dataDir string) (*Store, error) {
	s := &Store{path: filepath.Join(dataDir, "links.json")}
	if err := s.ensureDefaults(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) ensureDefaults() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	defaults := make([]Item, 0, len(config.C.Links))
	defaults = append(defaults, config.C.Links...)
	if !config.C.AllowEditLinks {
		return s.saveLocked(defaults)
	}
	if _, err := os.Stat(s.path); err == nil {
		return nil
	}
	return s.saveLocked(defaults)
}

func (s *Store) Load() ([]Item, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := os.ReadFile(s.path)
	if err != nil {
		return nil, err
	}
	var items []Item
	if err := json.Unmarshal(b, &items); err != nil {
		return nil, err
	}
	return items, nil
}

func (s *Store) Save(items []Item) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveLocked(items)
}

func (s *Store) saveLocked(items []Item) error {
	b, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, b, 0o600)
}
