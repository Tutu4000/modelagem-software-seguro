// Package partners talks to the external services (FC5): the payment gateway
// and the carrier. The messages are shared with cmd/partnersim, which plays
// both partners in the local environment.
package partners

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
)

// ErrUnavailable is a partner that did not answer. The order stays where it
// was (D03).
var ErrUnavailable = errors.New("partner unavailable")

type ChargeRequest struct {
	OrderID        uuid.UUID `json:"order_id"`
	AmountCents    int64     `json:"amount_cents"`
	IdempotencyKey string    `json:"idempotency_key"`
}

type Charge struct {
	ChargeID string `json:"charge_id"`
	Status   string `json:"status"`
}

// PaymentEvent is the notification of the gateway, signed with HMAC.
type PaymentEvent struct {
	EventID     string    `json:"event_id"`
	Type        string    `json:"type"` // payment.succeeded
	ChargeID    string    `json:"charge_id"`
	OrderID     uuid.UUID `json:"order_id"`
	AmountCents int64     `json:"amount_cents"`
}

const PaymentSucceeded = "payment.succeeded"

// RefundRequest gives back the whole charge of a cancelled order (RF10). The
// idempotency key makes a retry return the same refund.
type RefundRequest struct {
	ChargeID       string `json:"charge_id"`
	AmountCents    int64  `json:"amount_cents"`
	IdempotencyKey string `json:"idempotency_key"`
}

type Refund struct {
	RefundID string `json:"refund_id"`
	Status   string `json:"status"` // succeeded
}

type QuoteRequest struct {
	PostalCode string  `json:"postal_code"`
	VolumeCM3  float64 `json:"volume_cm3"`
}

type ShippingQuote struct {
	PriceCents int64 `json:"price_cents"`
	Days       int   `json:"days"`
}

// Address is the minimum to deliver a part (LGPD: data minimization).
type Address struct {
	Recipient  string `json:"recipient"`
	Street     string `json:"street"`
	Number     string `json:"number"`
	City       string `json:"city"`
	State      string `json:"state"`
	PostalCode string `json:"postal_code"`
}

var (
	postalCode = regexp.MustCompile(`^[0-9]{5}-?[0-9]{3}$`)
	state      = regexp.MustCompile(`^[A-Z]{2}$`)
)

// Validate checks formats and lengths. The text fields are free, but bounded.
func (a Address) Validate() error {
	fields := map[string]string{"recipient": a.Recipient, "street": a.Street, "number": a.Number, "city": a.City}
	for name, v := range fields {
		if n := utf8.RuneCountInString(strings.TrimSpace(v)); n < 1 || n > 120 {
			return fmt.Errorf("shipping %s must have 1 to 120 characters", name)
		}
		// The address goes to the database, to the carrier and to an e-mail:
		// a NUL makes Postgres fail, and a line break could forge lines there.
		if !utf8.ValidString(v) || strings.ContainsFunc(v, unicode.IsControl) {
			return fmt.Errorf("shipping %s must not have control characters", name)
		}
	}
	if !state.MatchString(a.State) {
		return errors.New("shipping state must be a two-letter code such as SP")
	}
	if !postalCode.MatchString(a.PostalCode) {
		return errors.New("shipping postal_code must be a CEP such as 01310-100")
	}
	return nil
}

type ShipmentRequest struct {
	OrderID uuid.UUID `json:"order_id"`
	Address Address   `json:"address"`
}

type Shipment struct {
	ShipmentID   string `json:"shipment_id"`
	TrackingCode string `json:"tracking_code"`
}

// ShippingEvent is the notification of the carrier, signed with HMAC.
type ShippingEvent struct {
	EventID    string    `json:"event_id"`
	Type       string    `json:"type"` // shipment.picked_up, shipment.delivered or shipment.exception
	ShipmentID string    `json:"shipment_id"`
	OrderID    uuid.UUID `json:"order_id"`
}

const (
	ShipmentPickedUp  = "shipment.picked_up"
	ShipmentDelivered = "shipment.delivered"
	// ShipmentException is a problem in the delivery (RF15). It does not
	// change the order's status.
	ShipmentException = "shipment.exception"
)

// Client calls one partner with its API key.
type Client struct {
	base string
	key  string
	http *http.Client
}

func NewClient(base string, key []byte) *Client {
	return &Client{base: strings.TrimSuffix(base, "/"), key: string(key), http: &http.Client{Timeout: 5 * time.Second}}
}

func (c *Client) CreateCharge(ctx context.Context, r ChargeRequest) (Charge, error) {
	var out Charge
	return out, c.post(ctx, "/charges", r, &out)
}

func (c *Client) Refund(ctx context.Context, r RefundRequest) (Refund, error) {
	var out Refund
	return out, c.post(ctx, "/refunds", r, &out)
}

func (c *Client) Quote(ctx context.Context, r QuoteRequest) (ShippingQuote, error) {
	var out ShippingQuote
	return out, c.post(ctx, "/quotes", r, &out)
}

func (c *Client) CreateShipment(ctx context.Context, r ShipmentRequest) (Shipment, error) {
	var out Shipment
	return out, c.post(ctx, "/shipments", r, &out)
}

func (c *Client) post(ctx context.Context, path string, in, out any) error {
	body, err := json.Marshal(in)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.key)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("%w: POST %s: %s", ErrUnavailable, path, resp.Status)
	}
	return json.Unmarshal(data, out)
}
