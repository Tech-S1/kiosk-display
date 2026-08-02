package audit

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	maxEntries  = 10000
	retention   = 7 * 24 * time.Hour
	dedupWindow = 1500 * time.Millisecond
)

type Entry struct {
	ID     int64     `json:"id"`
	Time   time.Time `json:"time"`
	Label  string    `json:"label,omitempty"`
	URL    string    `json:"url"`
	Source string    `json:"source"`
}

type Page struct {
	Items      []Entry `json:"items"`
	Total      int     `json:"total"`
	Page       int     `json:"page"`
	PageSize   int     `json:"page_size"`
	TotalPages int     `json:"total_pages"`
	Filter     string  `json:"filter"`
}

type Store struct {
	mu     sync.Mutex
	path   string
	items  []Entry
	nextID int64
}

func Open(dataDir string) (*Store, error) {
	s := &Store{
		path:   filepath.Join(dataDir, "audit.jsonl"),
		items:  make([]Entry, 0, 256),
		nextID: 1,
	}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) load() error {
	f, err := os.Open(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var e Entry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			continue
		}
		if e.ID >= s.nextID {
			s.nextID = e.ID + 1
		}
		s.items = append(s.items, e)
	}
	if err := sc.Err(); err != nil {
		return err
	}
	if s.pruneLocked() {
		_ = s.rewriteFile()
	}
	return nil
}

func (s *Store) Add(label, rawURL, source string) {
	label = strings.TrimSpace(label)
	rawURL = strings.TrimSpace(rawURL)
	source = strings.TrimSpace(source)
	if ignoreAuditURL(rawURL) {
		return
	}
	if rawURL == "" && label == "" {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for i := len(s.items) - 1; i >= 0; i-- {
		last := s.items[i]
		if now.Sub(last.Time) >= dedupWindow {
			break
		}
		if last.URL == rawURL && last.Label == label && sameAuditKind(last.Source, source) {
			return
		}
	}

	e := Entry{
		ID:     s.nextID,
		Time:   now.UTC(),
		Label:  label,
		URL:    rawURL,
		Source: source,
	}
	s.nextID++
	s.items = append(s.items, e)
	if s.pruneLocked() {
		_ = s.rewriteFile()
		return
	}
	_ = s.appendFile(e)
}

func ignoreAuditURL(raw string) bool {
	lower := strings.ToLower(strings.TrimSpace(raw))
	return strings.HasPrefix(lower, "about:blank")
}

func sameAuditKind(a, b string) bool {
	return auditKind(a) == auditKind(b)
}

func auditKind(source string) string {
	source = strings.ToLower(strings.TrimSpace(source))
	if strings.HasPrefix(source, "blocked") {
		return "blocked"
	}
	return "allowed"
}

func (s *Store) pruneLocked() bool {
	cutoff := time.Now().UTC().Add(-retention)
	n := len(s.items)
	kept := s.items[:0]
	for _, e := range s.items {
		if e.Time.Before(cutoff) {
			continue
		}
		kept = append(kept, e)
	}
	s.items = kept
	changed := len(s.items) != n
	if len(s.items) > maxEntries {
		s.items = s.items[len(s.items)-maxEntries:]
		changed = true
	}
	return changed
}

func writeEntry(f *os.File, e Entry) error {
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	_, err = f.Write(append(b, '\n'))
	return err
}

func (s *Store) appendFile(e Entry) error {
	f, err := os.OpenFile(s.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	return writeEntry(f, e)
}

func (s *Store) rewriteFile() error {
	tmp := s.path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	for _, e := range s.items {
		if err := writeEntry(f, e); err != nil {
			f.Close()
			_ = os.Remove(tmp)
			return err
		}
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, s.path)
}

func (s *Store) List(page, pageSize int, filter string) Page {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.pruneLocked() {
		_ = s.rewriteFile()
	}

	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 25
	}
	if pageSize > 200 {
		pageSize = 200
	}
	filter = strings.ToLower(strings.TrimSpace(filter))
	if filter != "blocked" && filter != "allowed" {
		filter = "all"
	}

	filtered := make([]Entry, 0, len(s.items))
	for _, e := range s.items {
		blocked := strings.HasPrefix(strings.ToLower(strings.TrimSpace(e.Source)), "blocked")
		switch filter {
		case "blocked":
			if !blocked {
				continue
			}
		case "allowed":
			if blocked {
				continue
			}
		}
		filtered = append(filtered, e)
	}

	total := len(filtered)
	totalPages := 0
	if total > 0 {
		totalPages = (total + pageSize - 1) / pageSize
	}
	if totalPages == 0 {
		return Page{Items: []Entry{}, Total: 0, Page: 1, PageSize: pageSize, TotalPages: 0, Filter: filter}
	}
	if page > totalPages {
		page = totalPages
	}

	start := total - page*pageSize
	end := total - (page-1)*pageSize
	if start < 0 {
		start = 0
	}
	chunk := filtered[start:end]
	out := make([]Entry, len(chunk))
	for i := range chunk {
		out[i] = chunk[len(chunk)-1-i]
	}
	return Page{
		Items:      out,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
		Filter:     filter,
	}
}
