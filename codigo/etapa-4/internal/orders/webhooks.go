package orders

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"nuggetmachine/internal/audit"
	"nuggetmachine/internal/partners"
	"nuggetmachine/internal/problem"
	"nuggetmachine/internal/webhook"
)

// maxWebhookBody bounds the raw body read for the signature check.
const maxWebhookBody = 64 << 10

// Webhooks receives the partners' notifications. The routes are public, as
// they would be for a real gateway and carrier, so the authentication is the
// HMAC signature of the exact bytes received, with a timestamp (C14, C15).
type Webhooks struct {
	Store          *Store
	PaymentsSecret []byte
	ShippingSecret []byte
	Audit          *audit.Logger
	Now            func() time.Time
	Log            *slog.Logger
}

func (w *Webhooks) Payments(c *gin.Context) {
	body, ok := w.verified(c, "payment", "payments", w.PaymentsSecret)
	if !ok {
		return
	}
	var ev partners.PaymentEvent
	if err := json.Unmarshal(body, &ev); err != nil || ev.EventID == "" || ev.ChargeID == "" || ev.Type != partners.PaymentSucceeded {
		w.reject(c, "payment.rejected", "payments", "", fmt.Errorf("unreadable or unknown event: %v", err))
		return
	}
	applied, err := w.Store.ApplyPayment(c.Request.Context(), PaymentEvent{
		EventID: ev.EventID, ChargeID: ev.ChargeID, OrderID: ev.OrderID, AmountCents: ev.AmountCents,
	})
	w.respond(c, "payment", "payments", ev.OrderID.String(), applied, err)
}

func (w *Webhooks) Shipping(c *gin.Context) {
	body, ok := w.verified(c, "shipment", "shipping", w.ShippingSecret)
	if !ok {
		return
	}
	var ev partners.ShippingEvent
	if err := json.Unmarshal(body, &ev); err != nil || ev.EventID == "" || ev.ShipmentID == "" {
		w.reject(c, "shipment.rejected", "shipping", "", fmt.Errorf("unreadable event: %v", err))
		return
	}
	applied, err := w.Store.ApplyShipping(c.Request.Context(), ShippingEvent{
		EventID: ev.EventID, Type: ev.Type, ShipmentID: ev.ShipmentID, OrderID: ev.OrderID,
	})
	name := "shipment"
	if ev.Type == partners.ShipmentPickedUp || ev.Type == partners.ShipmentDelivered {
		name = ev.Type
	}
	w.respond(c, name, "shipping", ev.OrderID.String(), applied, err)
}

// verified reads the raw body and checks its signature. A forged, altered or
// old notification stops here, before any parsing of its content.
func (w *Webhooks) verified(c *gin.Context, event, partner string, secret []byte) ([]byte, bool) {
	body, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, maxWebhookBody))
	if err != nil {
		w.reject(c, event+".rejected", partner, "", err)
		return nil, false
	}
	if err := webhook.Verify(secret, c.GetHeader(webhook.Header), body, w.Now(), webhook.Tolerance); err != nil {
		w.record(c, event+".rejected", partner, "", "rejected", "invalid_signature: "+err.Error())
		problem.Abort(c, problem.New(problem.InvalidSignature, err))
		return nil, false
	}
	return body, true
}

// respond: 200 for an applied or replayed event, 400 for an event that does
// not match the order, with the reason in the audit log.
func (w *Webhooks) respond(c *gin.Context, name, partner, target string, applied bool, err error) {
	switch {
	case errors.Is(err, ErrRejected):
		w.reject(c, rejectedName(name), partner, target, err)
	case err != nil:
		problem.Abort(c, err)
	case !applied:
		c.JSON(http.StatusOK, gin.H{"status": "duplicate"})
	default:
		if name == "payment" {
			name = "payment.succeeded"
		}
		w.record(c, name, partner, target, "ok", "")
		c.JSON(http.StatusOK, gin.H{"status": "applied"})
	}
}

func rejectedName(name string) string {
	if name == "payment" {
		return "payment.rejected"
	}
	return "shipment.rejected"
}

func (w *Webhooks) reject(c *gin.Context, name, partner, target string, err error) {
	w.record(c, name, partner, target, "rejected", err.Error())
	problem.Abort(c, problem.New(problem.InvalidRequest, err))
}

func (w *Webhooks) record(c *gin.Context, name, partner, target, result, reason string) {
	if err := w.Audit.Record(audit.Event{
		Name: name, Actor: "partner:" + partner, Action: "webhook." + partner, Target: target,
		Result: result, Reason: reason, Origin: c.ClientIP(),
	}); err != nil {
		_ = c.Error(err)
	}
}
