package tests

// Revocation paths that used to fail open when something went wrong in the
// middle (E5.1, F3).

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"nuggetmachine/internal/audit"
	"nuggetmachine/internal/auth"
	"nuggetmachine/internal/session"
)

// flakyStore fails the first revocations, like Valkey during a hiccup.
type flakyStore struct {
	*session.Memory
	failures int
}

func (s *flakyStore) RevokeSession(ctx context.Context, sid string, ttl time.Duration) error {
	if s.failures > 0 {
		s.failures--
		return errors.New("valkey: i/o timeout")
	}
	return s.Memory.RevokeSession(ctx, sid, ttl)
}

// Keycloak sends the same logout token again when the first delivery fails.
// The retry must revoke the session, not be refused as a replay.
func TestLogoutRetriedAfterAStoreFailureStillRevokes(t *testing.T) {
	e := newEnv(t)
	a := newUser()
	h := session.LogoutHandler{
		Verifier: e.verifier,
		Clients:  []string{"web"},
		Store:    &flakyStore{Memory: e.sessions, failures: 1},
		Audit:    audit.New(e.audit),
		Log:      slog.New(slog.NewJSONHandler(io.Discard, nil)),
	}
	token := sign(t, e.key, e.logoutClaims(a.String()))
	deliver := func() int {
		form := url.Values{"logout_token": {token}}
		req := httptest.NewRequest(http.MethodPost, "/oidc/backchannel-logout", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}

	if code := deliver(); code != http.StatusServiceUnavailable {
		t.Fatalf("delivery during the failure got %d, want 503", code)
	}
	if code := deliver(); code != http.StatusOK {
		t.Errorf("the retry got %d, want 200", code)
	}
	revoked, err := e.sessions.Revoked(context.Background(), sessionOf(a), a.String(), e.clock.Now())
	if err != nil {
		t.Fatal(err)
	}
	if !revoked {
		t.Error("the session is still valid after the logout was retried")
	}
}

// A token refreshed while Keycloak is still disabling the account is newer
// than the first cutoff. It must not survive the revocation.
func TestRevokeAlsoCutsTokensRefreshedWhileItRuns(t *testing.T) {
	e := newPanelEnv(t)
	admin, victim := newUser(), newUser()
	adminToken := e.staff(admin, auth.RoleAdmin, time.Minute)
	var refreshedAt time.Time
	e.idp.onDisable = func() {
		e.clock.Advance(2 * time.Second)
		refreshedAt = e.clock.Now()
		e.clock.Advance(3 * time.Second)
	}

	rec := e.post(revokePath(victim), adminToken, justification)

	wantStatus(t, rec, http.StatusOK)
	revoked, err := e.sessions.Revoked(context.Background(), "any", victim.String(), refreshedAt)
	if err != nil {
		t.Fatal(err)
	}
	if !revoked {
		t.Error("a token refreshed during the revocation is still accepted")
	}
}
