package tests

// Session revocation (E5.1) and per-user rate limit (E5.2).

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"nuggetmachine/internal/audit"
	"nuggetmachine/internal/session"
)

func TestRevokedSessionGets401(t *testing.T) {
	e := newEnv(t)
	a := newUser()
	token := e.token(a)
	path := "/api/v1/files/" + uuid.NewString()
	wantStatus(t, e.get(path, token), http.StatusForbidden)

	if err := e.sessions.RevokeSession(context.Background(), sessionOf(a), session.RevocationTTL); err != nil {
		t.Fatal(err)
	}

	rec := e.get(path, token)
	wantStatus(t, rec, http.StatusUnauthorized)
	if code := problemCode(t, rec); code != "unauthenticated" {
		t.Errorf("code %q, want \"unauthenticated\"", code)
	}
	if used := e.events("auth.revoked_token_used"); len(used) != 1 || used[0]["actor"] != a.String() {
		t.Errorf("unexpected auth.revoked_token_used events: %v", used)
	}
}

func TestUserRevocationRefusesOnlyOlderTokens(t *testing.T) {
	e := newEnv(t)
	a := newUser()
	path := "/api/v1/files/" + uuid.NewString()
	before := e.token(a)

	e.clock.Advance(time.Minute)
	if err := e.sessions.RevokeUser(context.Background(), a.String(), e.clock.Now()); err != nil {
		t.Fatal(err)
	}
	e.clock.Advance(time.Second)
	after := e.token(a)

	wantStatus(t, e.get(path, before), http.StatusUnauthorized)
	wantStatus(t, e.get(path, after), http.StatusForbidden)
}

// D03: without the session store the API cannot tell whether a token was
// revoked, so it refuses the request.
func TestSessionStoreDownFailsClosed(t *testing.T) {
	e := newEnv(t)
	e.sessions.Fail = errors.New("connection refused")

	rec := e.get("/api/v1/files/"+uuid.NewString(), e.token(newUser()))

	wantStatus(t, rec, http.StatusServiceUnavailable)
	if code := problemCode(t, rec); code != "unavailable" {
		t.Errorf("code %q, want \"unavailable\"", code)
	}
}

func TestUserAboveRateLimitGets429(t *testing.T) {
	e := newEnvWithRate(t, 5)
	a, b := newUser(), newUser()
	tokenA := e.token(a)
	path := "/api/v1/files/" + uuid.NewString()

	for range 5 {
		wantStatus(t, e.get(path, tokenA), http.StatusForbidden)
	}
	var last *httptest.ResponseRecorder
	for range 10 {
		last = e.get(path, tokenA)
		wantStatus(t, last, http.StatusTooManyRequests)
	}
	if code := problemCode(t, last); code != "rate_limited" {
		t.Errorf("code %q, want \"rate_limited\"", code)
	}
	if last.Header().Get("Retry-After") == "" {
		t.Error("429 without Retry-After")
	}
	if limited := e.events("rate.limited"); len(limited) != 1 || limited[0]["actor"] != a.String() {
		t.Errorf("want one rate.limited event for the window, got %v", limited)
	}

	wantStatus(t, e.get(path, e.token(b)), http.StatusForbidden)
}

// logoutClaims has the shape of a Keycloak back-channel logout token.
func (e *env) logoutClaims(user string) jwt.MapClaims {
	return jwt.MapClaims{
		"iss":    testIssuer,
		"aud":    "web",
		"sub":    user,
		"sid":    "session-" + user,
		"jti":    uuid.NewString(),
		"iat":    e.clock.Now().Unix(),
		"events": map[string]any{"http://schemas.openid.net/event/backchannel-logout": map[string]any{}},
	}
}

func (e *env) logout(token string) *httptest.ResponseRecorder {
	h := session.LogoutHandler{
		Verifier: e.verifier,
		Clients:  []string{"web", "panel-web", "e2e"},
		Store:    e.sessions,
		Audit:    audit.New(e.audit),
		Log:      slog.New(slog.NewJSONHandler(io.Discard, nil)),
	}
	form := url.Values{"logout_token": {token}}
	req := httptest.NewRequest(http.MethodPost, "/oidc/backchannel-logout", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestBackchannelLogoutRevokesTheSession(t *testing.T) {
	e := newEnv(t)
	a := newUser()
	token := e.token(a)
	path := "/api/v1/files/" + uuid.NewString()

	rec := e.logout(sign(t, e.key, e.logoutClaims(a.String())))

	if rec.Code != http.StatusOK {
		t.Fatalf("logout got %d, want 200: %s", rec.Code, rec.Body.String())
	}
	wantStatus(t, e.get(path, token), http.StatusUnauthorized)
	if revoked := e.events("token.revoked"); len(revoked) != 1 || revoked[0]["target"] != sessionOf(a) {
		t.Errorf("unexpected token.revoked events: %v", revoked)
	}
}

func TestInvalidLogoutTokenRevokesNothing(t *testing.T) {
	e := newEnv(t)
	a := newUser()
	user := a.String()
	token := e.token(a)
	_, otherKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name  string
		token string
	}{
		{"signed by another key", sign(t, otherKey, e.logoutClaims(user))},
		{"issued for another client", sign(t, e.key, with(e.logoutClaims(user), "aud", "api"))},
		{"without the logout event", sign(t, e.key, with(e.logoutClaims(user), "events", map[string]any{}))},
		{"with a nonce, like an ID token", sign(t, e.key, with(e.logoutClaims(user), "nonce", "abc"))},
		{"without sid", sign(t, e.key, with(e.logoutClaims(user), "sid", ""))},
		{"too old", sign(t, e.key, with(e.logoutClaims(user), "iat", e.clock.Now().Add(-10*time.Minute).Unix()))},
		{"from another issuer", sign(t, e.key, with(e.logoutClaims(user), "iss", "https://evil.test/realms/nugget"))},
		{"garbage", "not-a-jwt"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if rec := e.logout(tc.token); rec.Code != http.StatusBadRequest {
				t.Errorf("got %d, want 400", rec.Code)
			}
		})
	}

	t.Run("replayed jti", func(t *testing.T) {
		claims := e.logoutClaims(user)
		claims["sid"] = "another-session"
		first := sign(t, e.key, claims)
		if rec := e.logout(first); rec.Code != http.StatusOK {
			t.Fatalf("first use got %d", rec.Code)
		}
		if rec := e.logout(first); rec.Code != http.StatusBadRequest {
			t.Errorf("replay got %d, want 400", rec.Code)
		}
	})

	wantStatus(t, e.get("/api/v1/files/"+uuid.NewString(), token), http.StatusForbidden)
}
