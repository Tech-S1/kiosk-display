package manager

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"github.com/Tech-S1/kiosk-display/internal/config"
	"github.com/Tech-S1/kiosk-display/internal/server/common"
)

const oidcPendingTTL = 10 * time.Minute

type oidcAuth struct {
	verifier             *oidc.IDTokenVerifier
	oauth                oauth2.Config
	pending              *oidcPendingStore
	requiredGroups       []string
	issuer               string
	endSessionEndpoint   string
	postLogoutRedirectURL string
}

type oidcPending struct {
	nonce    string
	verifier string
	expiry   time.Time
}

type oidcPendingStore struct {
	mu   sync.Mutex
	byID map[string]oidcPending
}

func newOIDCPendingStore() *oidcPendingStore {
	return &oidcPendingStore{byID: make(map[string]oidcPending)}
}

func (s *oidcPendingStore) put(state, nonce, verifier string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for id, p := range s.byID {
		if now.After(p.expiry) {
			delete(s.byID, id)
		}
	}
	s.byID[state] = oidcPending{
		nonce:    nonce,
		verifier: verifier,
		expiry:   now.Add(oidcPendingTTL),
	}
}

func (s *oidcPendingStore) take(state string) (oidcPending, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.byID[state]
	if !ok {
		return oidcPending{}, false
	}
	delete(s.byID, state)
	if time.Now().After(p.expiry) {
		return oidcPending{}, false
	}
	return p, true
}

func postLogoutRedirectURL(cfg config.OIDC) string {
	if u := strings.TrimSpace(cfg.PostLogoutRedirectURL); u != "" {
		return u
	}
	u, err := url.Parse(cfg.RedirectURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host + "/"
}

func discoverEndSession(provider *oidc.Provider) string {
	var claims struct {
		EndSessionEndpoint string `json:"end_session_endpoint"`
	}
	if err := provider.Claims(&claims); err != nil {
		return ""
	}
	return strings.TrimSpace(claims.EndSessionEndpoint)
}

func newOIDCAuth(ctx context.Context, cfg config.OIDC) (*oidcAuth, error) {
	provider, err := oidc.NewProvider(ctx, cfg.Issuer)
	if err != nil {
		return nil, fmt.Errorf("oidc provider: %w", err)
	}
	scopes := append([]string(nil), cfg.Scopes...)
	if len(scopes) == 0 {
		scopes = []string{oidc.ScopeOpenID, "profile", "email"}
	}
	hasOpenID := false
	hasGroups := false
	for _, s := range scopes {
		switch s {
		case oidc.ScopeOpenID:
			hasOpenID = true
		case "groups":
			hasGroups = true
		}
	}
	if !hasOpenID {
		scopes = append([]string{oidc.ScopeOpenID}, scopes...)
	}
	if len(cfg.RequiredGroups) > 0 && !hasGroups {
		scopes = append(scopes, "groups")
	}
	return &oidcAuth{
		verifier: provider.Verifier(&oidc.Config{ClientID: cfg.ClientID}),
		oauth: oauth2.Config{
			ClientID:     cfg.ClientID,
			ClientSecret: cfg.ClientSecret,
			RedirectURL:  cfg.RedirectURL,
			Endpoint:     provider.Endpoint(),
			Scopes:       scopes,
		},
		pending:               newOIDCPendingStore(),
		requiredGroups:        append([]string(nil), cfg.RequiredGroups...),
		issuer:                strings.TrimRight(cfg.Issuer, "/"),
		endSessionEndpoint:    discoverEndSession(provider),
		postLogoutRedirectURL: postLogoutRedirectURL(cfg),
	}, nil
}

func randomURLString(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func pkceChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func (a *oidcAuth) beginLogin(w http.ResponseWriter, r *http.Request) {
	if !common.RequireMethod(w, r, http.MethodGet) {
		return
	}
	state, err := randomURLString(32)
	if err != nil {
		common.InternalError(w, err)
		return
	}
	nonce, err := randomURLString(32)
	if err != nil {
		common.InternalError(w, err)
		return
	}
	verifier, err := randomURLString(48)
	if err != nil {
		common.InternalError(w, err)
		return
	}
	a.pending.put(state, nonce, verifier)
	url := a.oauth.AuthCodeURL(
		state,
		oidc.Nonce(nonce),
		oauth2.SetAuthURLParam("code_challenge", pkceChallenge(verifier)),
		oauth2.SetAuthURLParam("code_challenge_method", "S256"),
	)
	http.Redirect(w, r, url, http.StatusFound)
}

func (a *oidcAuth) handleCallback(store *sessionStore, w http.ResponseWriter, r *http.Request) {
	if !common.RequireMethod(w, r, http.MethodGet) {
		return
	}
	if errMsg := strings.TrimSpace(r.URL.Query().Get("error")); errMsg != "" {
		desc := strings.TrimSpace(r.URL.Query().Get("error_description"))
		if desc != "" {
			errMsg = errMsg + ": " + desc
		}
		common.ClientError(w, http.StatusBadRequest, errMsg)
		return
	}
	code := strings.TrimSpace(r.URL.Query().Get("code"))
	state := strings.TrimSpace(r.URL.Query().Get("state"))
	if code == "" || state == "" {
		common.ClientError(w, http.StatusBadRequest, "missing code or state")
		return
	}
	pending, ok := a.pending.take(state)
	if !ok {
		common.ClientError(w, http.StatusBadRequest, "invalid or expired state")
		return
	}
	ctx := r.Context()
	token, err := a.oauth.Exchange(ctx, code, oauth2.SetAuthURLParam("code_verifier", pending.verifier))
	if err != nil {
		common.ClientError(w, http.StatusUnauthorized, "token exchange failed")
		return
	}
	rawID, ok := token.Extra("id_token").(string)
	if !ok || rawID == "" {
		common.ClientError(w, http.StatusUnauthorized, "missing id_token")
		return
	}
	idToken, err := a.verifier.Verify(ctx, rawID)
	if err != nil {
		common.ClientError(w, http.StatusUnauthorized, "invalid id_token")
		return
	}
	if idToken.Nonce != pending.nonce {
		common.ClientError(w, http.StatusUnauthorized, "invalid nonce")
		return
	}
	if !a.groupsAllowed(idToken) {
		common.ClientError(w, http.StatusForbidden, "missing required group")
		return
	}
	id, _, err := store.create(authMethodOIDC, rawID)
	if err != nil {
		common.InternalError(w, err)
		return
	}
	setSessionCookie(w, id)
	http.Redirect(w, r, "/", http.StatusFound)
}

func (a *oidcAuth) logoutURL(idToken string) string {
	postLogout := a.postLogoutRedirectURL
	if a.endSessionEndpoint != "" {
		u, err := url.Parse(a.endSessionEndpoint)
		if err != nil {
			return ""
		}
		q := u.Query()
		if idToken != "" {
			q.Set("id_token_hint", idToken)
		}
		q.Set("client_id", a.oauth.ClientID)
		if postLogout != "" {
			q.Set("post_logout_redirect_uri", postLogout)
		}
		u.RawQuery = q.Encode()
		return u.String()
	}
	if a.issuer == "" {
		return ""
	}
	u, err := url.Parse(a.issuer + "/logout")
	if err != nil {
		return ""
	}
	if postLogout != "" {
		q := u.Query()
		q.Set("rd", postLogout)
		u.RawQuery = q.Encode()
	}
	return u.String()
}

func (a *oidcAuth) groupsAllowed(idToken *oidc.IDToken) bool {
	if len(a.requiredGroups) == 0 {
		return true
	}
	var claims struct {
		Groups []string `json:"groups"`
	}
	if err := idToken.Claims(&claims); err != nil {
		return false
	}
	have := make(map[string]struct{}, len(claims.Groups))
	for _, g := range claims.Groups {
		have[g] = struct{}{}
	}
	for _, want := range a.requiredGroups {
		if _, ok := have[want]; ok {
			return true
		}
	}
	return false
}

func oidcEnabled() bool {
	return config.C.Manager.OIDC != nil && config.C.Manager.OIDC.Enabled()
}

func passwordEnabled() bool {
	return strings.TrimSpace(config.C.PasswordHash()) != ""
}
