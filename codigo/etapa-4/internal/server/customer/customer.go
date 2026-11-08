// Package customer is the public API router. The panel routes do not exist
// here.
package customer

import (
	"log/slog"
	"net/http"

	"nuggetmachine/internal/audit"
	"nuggetmachine/internal/auth"
	"nuggetmachine/internal/files"
	"nuggetmachine/internal/server"
	"nuggetmachine/internal/session"
)

type Deps struct {
	Verifier *auth.Verifier
	Files    files.Repo
	Store    files.ObjectStore
	Queue    files.Enqueuer
	Tasks    files.TaskReader
	Audit    *audit.Logger
	Sessions session.Store
	Limiter  session.Limiter
	// RatePerMinute is the per-user request limit (E5.2).
	RatePerMinute int
	Log           *slog.Logger // nil turns off the request log
	// TrustedProxies are the IPs allowed to report the origin through
	// X-Forwarded-For. When empty, the origin of audit events comes from the
	// connection, not from a forged header. In the compose, only nginx (E2.3).
	TrustedProxies []string
}

func New(d Deps) (*server.Router, error) {
	r, err := server.NewRouter(d.TrustedProxies, d.Log)
	if err != nil {
		return nil, err
	}

	h := &files.Handler{Repo: d.Files, Store: d.Store, Queue: d.Queue, Tasks: d.Tasks, Audit: d.Audit}
	api := r.Group("/api/v1", d.Verifier,
		session.Active(d.Sessions, d.Audit),
		session.RateLimit(d.Limiter, d.Audit, d.RatePerMinute),
	)
	customer := []auth.Role{auth.RoleCustomer}
	api.Handle(http.MethodGet, "/files/:id", customer, h.Status)
	api.Handle(http.MethodGet, "/files/:id/download", customer, h.Download)
	api.Handle(http.MethodPost, "/files", customer, h.Upload)
	return r, nil
}
