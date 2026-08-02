package manager

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Tech-S1/kiosk-display/internal/authpass"
	"github.com/Tech-S1/kiosk-display/internal/config"
	"github.com/Tech-S1/kiosk-display/internal/server/common"
)

const (
	sessionCookie = "__Host-kiosk_manager_session"
	csrfHeader    = "X-CSRF-Token"
	sessionIdle   = 12 * time.Hour
	sessionMaxAge = 24 * time.Hour
)

type session struct {
	created time.Time
	expiry  time.Time
	csrf    string
	pwdVer  string
}

type sessionStore struct {
	mu       sync.Mutex
	sessions map[string]session
}

func newSessionStore() *sessionStore {
	return &sessionStore{sessions: make(map[string]session)}
}

func passwordVersion() string {
	sum := sha256.Sum256([]byte(config.C.PasswordHash()))
	return hex.EncodeToString(sum[:8])
}

func newCSRF() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func (s *sessionStore) create() (id, csrf string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", "", err
	}
	csrf, err = newCSRF()
	if err != nil {
		return "", "", err
	}
	id = hex.EncodeToString(b)
	now := time.Now()
	s.mu.Lock()
	s.sessions = map[string]session{
		id: {
			created: now,
			expiry:  now.Add(sessionIdle),
			csrf:    csrf,
			pwdVer:  passwordVersion(),
		},
	}
	s.mu.Unlock()
	return id, csrf, nil
}

func (s *sessionStore) valid(id string) (session, bool) {
	id = strings.TrimSpace(id)
	if id == "" {
		return session{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[id]
	if !ok {
		return session{}, false
	}
	now := time.Now()
	if now.After(sess.created.Add(sessionMaxAge)) || now.After(sess.expiry) || sess.pwdVer != passwordVersion() {
		delete(s.sessions, id)
		return session{}, false
	}
	sess.expiry = now.Add(sessionIdle)
	s.sessions[id] = sess
	return sess, true
}

func (s *sessionStore) revoke(id string) {
	s.mu.Lock()
	delete(s.sessions, id)
	s.mu.Unlock()
}

func csrfMatch(got, want string) bool {
	a := []byte(strings.TrimSpace(got))
	b := []byte(want)
	if len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare(a, b) == 1
}

func sessionCookieValue(id string, maxAge int) *http.Cookie {
	return &http.Cookie{
		Name:     sessionCookie,
		Value:    id,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   maxAge,
	}
}

func setSessionCookie(w http.ResponseWriter, id string) {
	http.SetCookie(w, sessionCookieValue(id, int(sessionMaxAge.Seconds())))
}

func clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, sessionCookieValue("", -1))
}

func registerAuth(mux *http.ServeMux, store *sessionStore) {
	limiter := newLoginLimiter()
	mux.HandleFunc("/api/login", func(w http.ResponseWriter, r *http.Request) {
		if !common.RequireMethod(w, r, http.MethodPost) {
			return
		}
		ip := clientIP(r)
		if !limiter.allow(ip) {
			common.ClientError(w, http.StatusTooManyRequests, "too many attempts")
			return
		}
		var body struct {
			Password string `json:"password"`
		}
		if !common.DecodeJSON(w, r, &body) {
			return
		}
		if !authpass.Verify(config.C.PasswordHash(), body.Password) {
			limiter.fail(ip)
			common.ClientError(w, http.StatusUnauthorized, "invalid password")
			return
		}
		limiter.success(ip)
		id, csrf, err := store.create()
		if err != nil {
			common.InternalError(w, err)
			return
		}
		setSessionCookie(w, id)
		common.WriteJSON(w, map[string]string{"status": "ok", "csrf": csrf})
	})

	mux.HandleFunc("/api/logout", func(w http.ResponseWriter, r *http.Request) {
		if !common.RequireMethod(w, r, http.MethodPost) {
			return
		}
		if c, err := r.Cookie(sessionCookie); err == nil {
			store.revoke(c.Value)
		}
		clearSessionCookie(w)
		common.WriteJSON(w, map[string]string{"status": "ok"})
	})

	mux.HandleFunc("/api/auth", func(w http.ResponseWriter, r *http.Request) {
		if !common.RequireMethod(w, r, http.MethodGet) {
			return
		}
		out := map[string]any{"ok": false}
		if c, err := r.Cookie(sessionCookie); err == nil {
			if sess, ok := store.valid(c.Value); ok {
				out["ok"] = true
				out["csrf"] = sess.csrf
			}
		}
		common.WriteJSON(w, out)
	})
}

func requireAuth(store *sessionStore, next http.Handler) http.Handler {
	public := map[string]bool{
		"/api/health": true,
		"/api/login":  true,
		"/api/logout": true,
		"/api/auth":   true,
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if public[r.URL.Path] || !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}
		c, err := r.Cookie(sessionCookie)
		if err != nil {
			common.ClientError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		sess, ok := store.valid(c.Value)
		if !ok {
			common.ClientError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			if !csrfMatch(r.Header.Get(csrfHeader), sess.csrf) {
				common.ClientError(w, http.StatusForbidden, "invalid csrf token")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

const (
	loginWindow   = 15 * time.Minute
	loginMaxFails = 5
)

type loginLimiter struct {
	mu    sync.Mutex
	fails map[string][]time.Time
}

func newLoginLimiter() *loginLimiter {
	return &loginLimiter{fails: make(map[string][]time.Time)}
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (l *loginLimiter) prune(now time.Time, times []time.Time) []time.Time {
	out := times[:0]
	for _, t := range times {
		if now.Sub(t) <= loginWindow {
			out = append(out, t)
		}
	}
	return out
}

func (l *loginLimiter) allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	l.fails[ip] = l.prune(now, l.fails[ip])
	return len(l.fails[ip]) < loginMaxFails
}

func (l *loginLimiter) fail(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	l.fails[ip] = append(l.prune(now, l.fails[ip]), now)
}

func (l *loginLimiter) success(ip string) {
	l.mu.Lock()
	delete(l.fails, ip)
	l.mu.Unlock()
}
