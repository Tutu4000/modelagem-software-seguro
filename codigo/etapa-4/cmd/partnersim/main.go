// Command partnersim plays the two external partners of the local
// environment (FC5): the payment gateway and the carrier. It behaves like a
// third party: it authenticates calls with API keys, answers asynchronously
// and reports back through webhooks signed with HMAC, sent to the platform's
// public address.
package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"nuggetmachine/internal/config"
	"nuggetmachine/internal/partners"
	"nuggetmachine/internal/webhook"
)

type sim struct {
	log     *slog.Logger
	http    *http.Client
	hooks   string // platform base address for the webhooks
	delay   time.Duration
	paySec  []byte
	shipSec []byte

	mu      sync.Mutex
	charges map[string]partners.Charge // by idempotency key
	refunds map[string]partners.Refund // by idempotency key
}

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(log); err != nil {
		log.Error("partnersim stopped", "error", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	var env config.Env
	gatewayAddr := env.String("GATEWAY_ADDR", ":8100")
	carrierAddr := env.String("CARRIER_ADDR", ":8200")
	hooks := env.Required("WEBHOOK_BASE_URL")
	ca := env.Secret("WEBHOOK_CA")
	paymentsKey := env.Secret("PAYMENTS_API_KEY")
	carrierKeys := [][]byte{env.Secret("CARRIER_API_KEY_API"), env.Secret("CARRIER_API_KEY_PANEL")}
	paySec, shipSec := env.Secret("PAYMENTS_WEBHOOK_SECRET"), env.Secret("SHIPPING_WEBHOOK_SECRET")
	delay, err := time.ParseDuration(env.String("SIM_DELAY", "1s"))
	if err != nil {
		return fmt.Errorf("SIM_DELAY: %w", err)
	}
	if err := env.Err(); err != nil {
		return err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(ca) {
		return errors.New("WEBHOOK_CA: no certificate")
	}

	s := &sim{
		log: log, hooks: strings.TrimSuffix(hooks, "/"), delay: delay, paySec: paySec, shipSec: shipSec,
		http: &http.Client{
			Timeout:   10 * time.Second,
			Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}},
		},
		charges: map[string]partners.Charge{},
		refunds: map[string]partners.Refund{},
	}

	gateway := http.NewServeMux()
	gateway.Handle("POST /charges", authorized([][]byte{paymentsKey}, http.HandlerFunc(s.charge)))
	gateway.Handle("POST /refunds", authorized([][]byte{paymentsKey}, http.HandlerFunc(s.refund)))
	carrier := http.NewServeMux()
	carrier.Handle("POST /quotes", authorized(carrierKeys, http.HandlerFunc(s.quote)))
	carrier.Handle("POST /shipments", authorized(carrierKeys, http.HandlerFunc(s.shipment)))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	errc := make(chan error, 2)
	servers := []*http.Server{
		{Addr: gatewayAddr, Handler: gateway, ReadHeaderTimeout: 5 * time.Second},
		{Addr: carrierAddr, Handler: carrier, ReadHeaderTimeout: 5 * time.Second},
	}
	for _, srv := range servers {
		go func() { errc <- srv.ListenAndServe() }()
	}
	log.Info("partnersim up", "gateway", gatewayAddr, "carrier", carrierAddr)
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
		return nil
	}
}

// authorized accepts any of the keys, compared in constant time.
func authorized(keys [][]byte, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := []byte(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
		for _, k := range keys {
			if subtle.ConstantTimeCompare(got, k) == 1 {
				next.ServeHTTP(w, r)
				return
			}
		}
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	})
}

// charge creates a pending charge and, a moment later, reports it paid for
// the amount it was created with. The same idempotency key returns the same
// charge.
func (s *sim) charge(w http.ResponseWriter, r *http.Request) {
	var req partners.ChargeRequest
	if !decode(w, r, &req) || req.AmountCents <= 0 || req.IdempotencyKey == "" {
		http.Error(w, "invalid charge", http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	c, seen := s.charges[req.IdempotencyKey]
	if !seen {
		c = partners.Charge{ChargeID: "ch_" + randomHex(12), Status: "pending"}
		s.charges[req.IdempotencyKey] = c
	}
	s.mu.Unlock()
	if !seen {
		go s.later(1, func() {
			s.send("/api/v1/webhooks/payments", s.paySec, partners.PaymentEvent{
				EventID: "evt_" + randomHex(12), Type: partners.PaymentSucceeded,
				ChargeID: c.ChargeID, OrderID: req.OrderID, AmountCents: req.AmountCents,
			})
		})
	}
	reply(w, http.StatusCreated, c)
}

// refund gives the money back at once. The same idempotency key returns the
// same refund.
func (s *sim) refund(w http.ResponseWriter, r *http.Request) {
	var req partners.RefundRequest
	if !decode(w, r, &req) || req.ChargeID == "" || req.AmountCents <= 0 || req.IdempotencyKey == "" {
		http.Error(w, "invalid refund", http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	rf, seen := s.refunds[req.IdempotencyKey]
	if !seen {
		rf = partners.Refund{RefundID: "re_" + randomHex(12), Status: "succeeded"}
		s.refunds[req.IdempotencyKey] = rf
	}
	s.mu.Unlock()
	reply(w, http.StatusCreated, rf)
}

// quote is a fixed formula: a base price plus 10 cents per cm3.
func (s *sim) quote(w http.ResponseWriter, r *http.Request) {
	var req partners.QuoteRequest
	if !decode(w, r, &req) || req.VolumeCM3 <= 0 || req.PostalCode == "" {
		http.Error(w, "invalid quote", http.StatusBadRequest)
		return
	}
	reply(w, http.StatusOK, partners.ShippingQuote{PriceCents: 1500 + int64(math.Ceil(req.VolumeCM3*10)), Days: 5})
}

// shipment books a pickup and later reports it picked up and delivered.
func (s *sim) shipment(w http.ResponseWriter, r *http.Request) {
	var req partners.ShipmentRequest
	if !decode(w, r, &req) || req.Address.Validate() != nil {
		http.Error(w, "invalid shipment", http.StatusBadRequest)
		return
	}
	sh := partners.Shipment{ShipmentID: "shp_" + randomHex(12), TrackingCode: "NG" + strings.ToUpper(randomHex(5)) + "BR"}
	go s.later(1, func() {
		s.send("/api/v1/webhooks/shipping", s.shipSec, partners.ShippingEvent{
			EventID: "evt_" + randomHex(12), Type: partners.ShipmentPickedUp, ShipmentID: sh.ShipmentID, OrderID: req.OrderID,
		})
	})
	go s.later(3, func() {
		s.send("/api/v1/webhooks/shipping", s.shipSec, partners.ShippingEvent{
			EventID: "evt_" + randomHex(12), Type: partners.ShipmentDelivered, ShipmentID: sh.ShipmentID, OrderID: req.OrderID,
		})
	})
	reply(w, http.StatusCreated, sh)
}

func (s *sim) later(steps int, fn func()) {
	time.Sleep(time.Duration(steps) * s.delay)
	fn()
}

// send signs and posts one event, retrying a few times like a real partner.
func (s *sim) send(path string, secret []byte, event any) {
	body, err := json.Marshal(event)
	if err != nil {
		s.log.Error("encode event", "error", err)
		return
	}
	for attempt := range 4 {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * time.Second)
		}
		req, err := http.NewRequest(http.MethodPost, s.hooks+path, bytes.NewReader(body))
		if err != nil {
			s.log.Error("build webhook", "error", err)
			return
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set(webhook.Header, webhook.Sign(secret, time.Now(), body))
		resp, err := s.http.Do(req)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode/100 == 2 {
				s.log.Info("webhook delivered", "path", path, "status", resp.StatusCode)
				return
			}
			s.log.Warn("webhook refused", "path", path, "status", resp.StatusCode)
			if resp.StatusCode/100 == 4 {
				return
			}
			continue
		}
		s.log.Warn("webhook failed", "path", path, "error", err)
	}
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	return json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(v) == nil
}

func reply(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
