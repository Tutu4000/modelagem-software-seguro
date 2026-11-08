// Package panel is the internal panel router (D02). Only cmd/panel imports
// it: the public API binary does not even contain these routes.
package panel

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"

	"nuggetmachine/internal/audit"
	"nuggetmachine/internal/auth"
	"nuggetmachine/internal/server"
	"nuggetmachine/internal/session"
)

// IdP is what the panel asks of Keycloak in an incident.
type IdP interface {
	LogoutAndDisable(ctx context.Context, user uuid.UUID) error
}

type Deps struct {
	Verifier      *auth.Verifier
	Sessions      session.Store
	Limiter       session.Limiter
	RatePerMinute int
	Audit         *audit.Logger
	IdP           IdP
	// StepUpMaxAge is how recent the login must be for a critical operation.
	StepUpMaxAge   time.Duration
	Now            func() time.Time
	Log            *slog.Logger
	TrustedProxies []string
}

func New(d Deps) (*server.Router, error) {
	r, err := server.NewRouter(d.TrustedProxies, d.Log)
	if err != nil {
		return nil, err
	}
	// The layers of D02 after the network: role (in Handle), MFA, live
	// session, per-user rate limit and the audit of every access.
	v1 := r.Group("/panel/v1", d.Verifier,
		RequireMFA(),
		session.Active(d.Sessions, d.Audit),
		session.RateLimit(d.Limiter, d.Audit, d.RatePerMinute),
		Access(d.Audit),
	)
	h := handlers{d: d}
	v1.Handle(http.MethodGet, "/me", auth.StaffRoles, h.me)
	v1.Handle(http.MethodPost, "/users/:id/revoke", []auth.Role{auth.RoleAdmin},
		Critical(d.StepUpMaxAge, d.Now, d.Audit), h.revoke)
	return r, nil
}
