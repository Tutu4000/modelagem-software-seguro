package tests

// Internal panel (D02, E6.1, E6.4): MFA, staff roles, audit of every access
// and the critical operations with step-up and justification.

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"nuggetmachine/internal/audit"
	"nuggetmachine/internal/auth"
	"nuggetmachine/internal/server/panel"
	"nuggetmachine/internal/session"
)

type fakeIdP struct {
	revoked []uuid.UUID
	err     error
}

func (f *fakeIdP) LogoutAndDisable(_ context.Context, user uuid.UUID) error {
	if f.err != nil {
		return f.err
	}
	f.revoked = append(f.revoked, user)
	return nil
}

type panelEnv struct {
	*env
	idp *fakeIdP
}

func newPanelEnv(t *testing.T) *panelEnv {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	clk := &clock{t: time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)}
	e := &env{t: t, audit: &bytes.Buffer{}, sessions: session.NewMemory(clk.Now), key: priv, clock: clk}
	idp := &fakeIdP{}
	router, err := panel.New(panel.Deps{
		Verifier: auth.NewVerifier(auth.VerifierConfig{
			Keys: auth.StaticKey(pub), Issuer: testIssuer, Audience: "panel", Now: clk.Now,
		}),
		Sessions:      e.sessions,
		Limiter:       e.sessions,
		RatePerMinute: 1000,
		Audit:         audit.New(e.audit),
		IdP:           idp,
		StepUpMaxAge:  5 * time.Minute,
		Now:           clk.Now,
	})
	if err != nil {
		t.Fatal(err)
	}
	e.router = router
	return &panelEnv{env: e, idp: idp}
}

// staff has the shape of a panel-web access token: audience panel, level 2
// (password and TOTP) and the time of the login.
func (e *panelEnv) staff(user auth.UserID, role auth.Role, loggedInAgo time.Duration) string {
	now := e.clock.Now()
	return sign(e.t, e.key, jwt.MapClaims{
		"iss":          testIssuer,
		"aud":          []string{"panel"},
		"typ":          "Bearer",
		"sub":          user.String(),
		"sid":          sessionOf(user),
		"acr":          "2",
		"auth_time":    now.Add(-loggedInAgo).Unix(),
		"realm_access": map[string]any{"roles": []string{string(role)}},
		"iat":          now.Unix(),
		"exp":          now.Add(5 * time.Minute).Unix(),
	})
}

func (e *panelEnv) post(path, token, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	return e.do(req)
}

func revokePath(user auth.UserID) string { return "/panel/v1/users/" + user.String() + "/revoke" }

const justification = `{"justification":"account used in the credential stuffing incident"}`

func TestPanelRefusesTokenWithoutMFA(t *testing.T) {
	e := newPanelEnv(t)
	tech := newUser()
	claims := jwt.MapClaims{
		"iss": testIssuer, "aud": []string{"panel"}, "typ": "Bearer", "sub": tech.String(), "sid": sessionOf(tech),
		"acr": "1", "realm_access": map[string]any{"roles": []string{"technician"}},
		"iat": e.clock.Now().Unix(), "exp": e.clock.Now().Add(5 * time.Minute).Unix(),
	}

	rec := e.get("/panel/v1/me", sign(t, e.key, claims))

	wantStatus(t, rec, http.StatusUnauthorized)
	if code := problemCode(t, rec); code != "mfa_required" {
		t.Errorf("code %q, want \"mfa_required\"", code)
	}
}

func TestPanelRefusesCustomers(t *testing.T) {
	e := newPanelEnv(t)

	rec := e.get("/panel/v1/me", e.staff(newUser(), auth.RoleCustomer, time.Minute))

	wantStatus(t, rec, http.StatusForbidden)
}

func TestPanelAccessIsAudited(t *testing.T) {
	e := newPanelEnv(t)
	tech := newUser()

	wantStatus(t, e.get("/panel/v1/me", e.staff(tech, auth.RoleTechnician, time.Hour)), http.StatusOK)

	access := e.events("panel.access")
	if len(access) != 1 || access[0]["actor"] != tech.String() || access[0]["result"] != "200" {
		t.Errorf("unexpected panel.access events: %v", access)
	}
}

func TestCriticalOperationNeedsRecentLogin(t *testing.T) {
	e := newPanelEnv(t)

	rec := e.post(revokePath(newUser()), e.staff(newUser(), auth.RoleAdmin, 10*time.Minute), justification)

	wantStatus(t, rec, http.StatusUnauthorized)
	if code := problemCode(t, rec); code != "step_up_required" {
		t.Errorf("code %q, want \"step_up_required\"", code)
	}
	challenge := rec.Header().Get("WWW-Authenticate")
	if !strings.Contains(challenge, `error="insufficient_user_authentication"`) || !strings.Contains(challenge, "max_age=300") {
		t.Errorf("WWW-Authenticate %q does not ask for a step-up (RFC 9470)", challenge)
	}
	if len(e.idp.revoked) != 0 {
		t.Error("the user was revoked without a recent login")
	}
}

func TestCriticalOperationNeedsJustification(t *testing.T) {
	e := newPanelEnv(t)
	admin := e.staff(newUser(), auth.RoleAdmin, time.Minute)

	for _, body := range []string{``, `{}`, `{"justification":"short"}`, `{"justification":"` + strings.Repeat("x", 501) + `"}`} {
		rec := e.post(revokePath(newUser()), admin, body)
		wantStatus(t, rec, http.StatusUnprocessableEntity)
		if code := problemCode(t, rec); code != "justification_required" {
			t.Errorf("body %.20q: code %q, want \"justification_required\"", body, code)
		}
	}
	if len(e.idp.revoked) != 0 {
		t.Error("the user was revoked without a justification")
	}
}

func TestRevokeCutsTokensAndDisablesTheUser(t *testing.T) {
	e := newPanelEnv(t)
	admin, victim := newUser(), newUser()
	issuedBefore := e.clock.Now().Add(-time.Second)

	rec := e.post(revokePath(victim), e.staff(admin, auth.RoleAdmin, time.Minute), justification)

	wantStatus(t, rec, http.StatusOK)
	if len(e.idp.revoked) != 1 || e.idp.revoked[0] != uuid.UUID(victim) {
		t.Errorf("Keycloak calls: %v, want one for %s", e.idp.revoked, victim)
	}
	if revoked, _ := e.sessions.Revoked(context.Background(), "any", victim.String(), issuedBefore); !revoked {
		t.Error("a token issued before the revocation is still accepted")
	}
	if ev := e.events("user.revoked"); len(ev) != 1 || ev[0]["target"] != victim.String() || ev[0]["result"] != "revoked" {
		t.Errorf("unexpected user.revoked events: %v", ev)
	}
	if ev := e.events("panel.critical"); len(ev) != 1 || !strings.Contains(ev[0]["reason"].(string), "credential stuffing") {
		t.Errorf("unexpected panel.critical events: %v", ev)
	}
}

func TestAdminCannotRevokeThemselves(t *testing.T) {
	e := newPanelEnv(t)
	admin := newUser()

	rec := e.post(revokePath(admin), e.staff(admin, auth.RoleAdmin, time.Minute), justification)

	wantStatus(t, rec, http.StatusBadRequest)
	if len(e.idp.revoked) != 0 {
		t.Error("the admin revoked themselves")
	}
}

func TestRevokeWithKeycloakDownStillCutsTokens(t *testing.T) {
	e := newPanelEnv(t)
	e.idp.err = errors.New("connection refused")
	victim := newUser()

	rec := e.post(revokePath(victim), e.staff(newUser(), auth.RoleAdmin, time.Minute), justification)

	wantStatus(t, rec, http.StatusBadGateway)
	if code := problemCode(t, rec); code != "idp_unavailable" {
		t.Errorf("code %q, want \"idp_unavailable\"", code)
	}
	if revoked, _ := e.sessions.Revoked(context.Background(), "any", victim.String(), e.clock.Now().Add(-time.Second)); !revoked {
		t.Error("the cut in the session store did not hold")
	}
	if ev := e.events("user.revoked"); len(ev) != 1 || ev[0]["result"] != "tokens_cut_idp_failed" {
		t.Errorf("unexpected user.revoked events: %v", ev)
	}
}

func TestOnlyAdminsRevoke(t *testing.T) {
	e := newPanelEnv(t)
	for _, role := range []auth.Role{auth.RoleSupport, auth.RoleTechnician} {
		rec := e.post(revokePath(newUser()), e.staff(newUser(), role, time.Minute), justification)
		wantStatus(t, rec, http.StatusForbidden)
	}
}
