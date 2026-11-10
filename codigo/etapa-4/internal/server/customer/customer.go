// Package customer is the public API router. The panel routes do not exist
// here.
package customer

import (
	"log/slog"
	"net/http"
	"time"

	"nuggetmachine/internal/audit"
	"nuggetmachine/internal/auth"
	"nuggetmachine/internal/files"
	"nuggetmachine/internal/notify"
	"nuggetmachine/internal/orders"
	"nuggetmachine/internal/server"
	"nuggetmachine/internal/session"
	"nuggetmachine/internal/support"
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
	Orders   *orders.Store
	Support  *support.Store
	// Notify queues the notices to the customer's registered e-mail (E8.4).
	Notify notify.Sender
	// BuildVolume is the printer's build volume, for the DFM check.
	BuildVolume [3]float64
	Now         func() time.Time
	Gateway     orders.Gateway
	Carrier     orders.Carrier
	// The partners sign their webhooks with these secrets (C14, C15).
	PaymentsWebhookSecret []byte
	ShippingWebhookSecret []byte
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

	notices := orders.Notifier{Sender: d.Notify, Audit: d.Audit, Log: logger(d.Log)}
	o := &orders.Handler{
		Store: d.Orders, Files: d.Files, Tasks: d.Tasks, Audit: d.Audit, BuildVolume: d.BuildVolume, Now: d.Now,
		Gateway: d.Gateway, Carrier: d.Carrier, Notify: notices,
	}
	api.Handle(http.MethodGet, "/catalog", customer, o.Catalog)
	api.Handle(http.MethodPost, "/orders", customer, o.Create)
	api.Handle(http.MethodGet, "/orders", customer, o.List)
	api.Handle(http.MethodGet, "/orders/:id", customer, o.Get)
	api.Handle(http.MethodPost, "/orders/:id/submit", customer, o.Submit)
	api.Handle(http.MethodPost, "/orders/:id/payment", customer, o.Pay)
	api.Handle(http.MethodPost, "/orders/:id/cancel", customer, o.Cancel)
	api.Handle(http.MethodGet, "/orders/:id/address-change", customer, o.PendingAddressChange)
	api.Handle(http.MethodPost, "/orders/:id/address-change/confirm", customer, o.ConfirmAddressChange)

	t := &support.Handler{Store: d.Support, Audit: d.Audit}
	api.Handle(http.MethodPost, "/tickets", customer, t.Open)
	api.Handle(http.MethodGet, "/tickets", customer, t.List)
	api.Handle(http.MethodGet, "/tickets/:id", customer, t.Get)
	api.Handle(http.MethodPost, "/tickets/:id/messages", customer, t.Reply)

	// The partners' notifications: no user, authenticated by the signature.
	w := &orders.Webhooks{
		Store: d.Orders, PaymentsSecret: d.PaymentsWebhookSecret, ShippingSecret: d.ShippingWebhookSecret,
		Audit: d.Audit, Now: d.Now, Log: logger(d.Log), Notify: notices,
	}
	r.HandleSigned(http.MethodPost, "/api/v1/webhooks/payments", "hmac:payments", w.Payments)
	r.HandleSigned(http.MethodPost, "/api/v1/webhooks/shipping", "hmac:shipping", w.Shipping)
	return r, nil
}

// logger keeps the handlers from checking for a nil logger.
func logger(l *slog.Logger) *slog.Logger {
	if l == nil {
		return slog.New(slog.DiscardHandler)
	}
	return l
}
