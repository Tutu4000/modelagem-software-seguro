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
	"nuggetmachine/internal/orders"
	"nuggetmachine/internal/server"
	"nuggetmachine/internal/session"
)

// IdP is what the panel asks of Keycloak: the incident response and whether
// an account has a role.
type IdP interface {
	LogoutAndDisable(ctx context.Context, user uuid.UUID) error
	HasRealmRole(ctx context.Context, user uuid.UUID, role string) (bool, error)
}

// Presigner signs read URLs with the panel's read-only storage key.
type Presigner interface {
	PresignGet(key string, ttl time.Duration) (string, error)
}

type Deps struct {
	Verifier      *auth.Verifier
	Sessions      session.Store
	Limiter       session.Limiter
	RatePerMinute int
	Audit         *audit.Logger
	IdP           IdP
	Orders        *orders.Store
	Files         Presigner
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
	critical := Critical(d.StepUpMaxAge, d.Now, d.Audit)
	admin := []auth.Role{auth.RoleAdmin}
	technician := []auth.Role{auth.RoleTechnician}

	v1.Handle(http.MethodGet, "/me", auth.StaffRoles, h.me)
	v1.Handle(http.MethodPost, "/users/:id/revoke", admin, critical, h.revoke)

	v1.Handle(http.MethodGet, "/orders", []auth.Role{auth.RoleAdmin, auth.RoleSupport}, h.listOrders)
	v1.Handle(http.MethodGet, "/orders/assigned", technician, h.assignedOrders)
	v1.Handle(http.MethodGet, "/orders/:id", auth.StaffRoles, h.getOrder)
	v1.Handle(http.MethodPost, "/orders/:id/assign", admin, h.assign)
	v1.Handle(http.MethodPost, "/orders/:id/file", technician, critical, h.file)
	v1.Handle(http.MethodPost, "/orders/:id/review", technician, h.review)

	v1.Handle(http.MethodGet, "/price-tables", admin, h.priceTables)
	v1.Handle(http.MethodPost, "/price-tables", admin, h.proposePriceTable)
	v1.Handle(http.MethodPost, "/price-tables/:version/approve", admin, critical, h.approvePriceTable)
	return r, nil
}
