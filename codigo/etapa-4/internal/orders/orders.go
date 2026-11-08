// Package orders is the orders module (E8.1): quote, submission, technical
// review and the order history. The API reaches it with the "orders" database
// role and the panel with "panel_orders" (D02); the grants per column decide
// what each one can change.
package orders

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"nuggetmachine/internal/orders/ordersdb"
	"nuggetmachine/internal/pricing"
)

type Status string

const (
	StatusQuoted          Status = "quoted"
	StatusAwaitingReview  Status = "awaiting_review"
	StatusAwaitingPayment Status = "awaiting_payment"
	StatusRejected        Status = "rejected"
	StatusPaid            Status = "paid"
	StatusInProduction    Status = "in_production"
	StatusShipped         Status = "shipped"
	StatusDelivered       Status = "delivered"
	StatusCancelled       Status = "cancelled"
)

// transitions is the only place that says which status may follow which.
var transitions = map[Status][]Status{
	StatusQuoted:          {StatusAwaitingReview, StatusCancelled},
	StatusAwaitingReview:  {StatusAwaitingPayment, StatusRejected, StatusCancelled},
	StatusAwaitingPayment: {StatusPaid, StatusCancelled},
	StatusPaid:            {StatusInProduction},
	StatusInProduction:    {StatusShipped},
	StatusShipped:         {StatusDelivered},
}

func CanTransition(from, to Status) bool {
	for _, next := range transitions[from] {
		if next == to {
			return true
		}
	}
	return false
}

var (
	// ErrNotFound covers a missing order and someone else's order alike.
	ErrNotFound = errors.New("order not found")
	// ErrConflict is a change that does not apply to the current status, or
	// that someone else made first.
	ErrConflict = errors.New("the order is not in the expected status")
	// ErrSameApprover is the second admin being the one who proposed (D02).
	ErrSameApprover = errors.New("a price table must be approved by another admin")
)

// QuoteValidity is how long a quote holds before it must be redone.
const QuoteValidity = 24 * time.Hour

type Order = ordersdb.Order

type Review struct {
	Decision   string    `json:"decision"`
	Report     string    `json:"report"`
	ReviewedAt time.Time `json:"reviewed_at"`
	Technician uuid.UUID `json:"-"`
}

type Change struct {
	From *string   `json:"from"`
	To   string    `json:"to"`
	At   time.Time `json:"at"`
}

type PriceTable struct {
	Version    int64         `json:"version"`
	Status     string        `json:"status"`
	Rules      pricing.Rules `json:"rules"`
	CreatedBy  *uuid.UUID    `json:"created_by"`
	CreatedAt  time.Time     `json:"created_at"`
	ApprovedBy *uuid.UUID    `json:"approved_by"`
	ApprovedAt *time.Time    `json:"approved_at"`
}

// Store runs the queries of queries.sql. The same code serves both binaries;
// the role of the pool limits what each one can do.
type Store struct {
	db *pgxpool.Pool
	q  *ordersdb.Queries
}

func NewStore(db *pgxpool.Pool) *Store { return &Store{db: db, q: ordersdb.New(db)} }

func (s *Store) ActiveRules(ctx context.Context) (int64, pricing.Rules, error) {
	row, err := s.q.ActivePriceTable(ctx)
	if err != nil {
		return 0, pricing.Rules{}, fmt.Errorf("active price table: %w", err)
	}
	var r pricing.Rules
	if err := json.Unmarshal(row.Rules, &r); err != nil {
		return 0, pricing.Rules{}, fmt.Errorf("price table %d: %w", row.Version, err)
	}
	return row.Version, r, nil
}

// Create stores a new order and its first history entry together.
func (s *Store) Create(ctx context.Context, o Order) error {
	return s.tx(ctx, func(q *ordersdb.Queries) error {
		if err := q.InsertOrder(ctx, ordersdb.InsertOrderParams{
			ID: o.ID, OwnerID: o.OwnerID, FileID: o.FileID,
			Material: o.Material, Color: o.Color, Quality: o.Quality, Finishing: o.Finishing, Quantity: o.Quantity,
			PriceTableVersion: o.PriceTableVersion, UnitCents: o.UnitCents, DiscountCents: o.DiscountCents,
			ShippingCents: o.ShippingCents, TotalCents: o.TotalCents,
			VolumeMm3: o.VolumeMm3, BboxMm: o.BboxMm, DfmFlags: o.DfmFlags, Status: o.Status, QuoteExpiresAt: o.QuoteExpiresAt,
			ShipRecipient: o.ShipRecipient, ShipStreet: o.ShipStreet, ShipNumber: o.ShipNumber,
			ShipCity: o.ShipCity, ShipState: o.ShipState, ShipPostalCode: o.ShipPostalCode,
		}); err != nil {
			return err
		}
		return q.InsertStatusHistory(ctx, ordersdb.InsertStatusHistoryParams{OrderID: o.ID, ToStatus: o.Status, Actor: o.OwnerID})
	})
}

func (s *Store) GetForOwner(ctx context.Context, owner, id uuid.UUID) (Order, error) {
	o, err := s.q.GetOrderForOwner(ctx, ordersdb.GetOrderForOwnerParams{ID: id, OwnerID: owner})
	return o, notFound(err)
}

func (s *Store) Get(ctx context.Context, id uuid.UUID) (Order, error) {
	o, err := s.q.GetOrder(ctx, id)
	return o, notFound(err)
}

func (s *Store) ListForOwner(ctx context.Context, owner uuid.UUID) ([]Order, error) {
	return s.q.ListOrdersForOwner(ctx, owner)
}

func (s *Store) ListByStatus(ctx context.Context, status string) ([]Order, error) {
	return s.q.ListOrdersByStatus(ctx, status)
}

func (s *Store) ListAssigned(ctx context.Context, technician uuid.UUID) ([]Order, error) {
	return s.q.ListOrdersAssigned(ctx, &technician)
}

func (s *Store) History(ctx context.Context, id uuid.UUID) ([]Change, error) {
	rows, err := s.q.ListStatusHistory(ctx, id)
	if err != nil {
		return nil, err
	}
	out := make([]Change, 0, len(rows))
	for _, r := range rows {
		out = append(out, Change{From: r.FromStatus, To: r.ToStatus, At: r.At})
	}
	return out, nil
}

// Details loads what the detail page of an order shows besides the order.
func (s *Store) Details(ctx context.Context, id uuid.UUID) (Details, error) {
	var d Details
	var err error
	if d.History, err = s.History(ctx, id); err != nil {
		return d, err
	}
	if d.Review, err = s.ReviewOf(ctx, id); err != nil {
		return d, err
	}
	d.Payment, err = s.LivePayment(ctx, id)
	return d, err
}

// ReviewOf returns nil when the order was not reviewed yet.
func (s *Store) ReviewOf(ctx context.Context, id uuid.UUID) (*Review, error) {
	r, err := s.q.GetReview(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &Review{Decision: r.Decision, Report: r.Report, ReviewedAt: r.CreatedAt, Technician: r.TechnicianID}, nil
}

// Transition changes the status only if it is still from, and records who did
// it.
func (s *Store) Transition(ctx context.Context, id uuid.UUID, from, to Status, actor uuid.UUID) error {
	if !CanTransition(from, to) {
		return fmt.Errorf("%w: %s to %s is not allowed", ErrConflict, from, to)
	}
	return s.tx(ctx, func(q *ordersdb.Queries) error {
		return transition(ctx, q, id, from, to, actor)
	})
}

func (s *Store) Assign(ctx context.Context, id, technician uuid.UUID) error {
	n, err := s.q.AssignOrder(ctx, ordersdb.AssignOrderParams{ID: id, Technician: &technician})
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("%w: only an order awaiting review can be assigned", ErrConflict)
	}
	return nil
}

// Review records the decision and moves the order in the same transaction.
// The review's primary key is the order, so a second review fails.
func (s *Store) Review(ctx context.Context, id, technician uuid.UUID, approved bool, report string) error {
	to, decision := StatusRejected, "rejected"
	if approved {
		to, decision = StatusAwaitingPayment, "approved"
	}
	return s.tx(ctx, func(q *ordersdb.Queries) error {
		if err := transition(ctx, q, id, StatusAwaitingReview, to, technician); err != nil {
			return err
		}
		return q.InsertReview(ctx, ordersdb.InsertReviewParams{OrderID: id, TechnicianID: technician, Decision: decision, Report: report})
	})
}

func (s *Store) PriceTables(ctx context.Context) ([]PriceTable, error) {
	rows, err := s.q.ListPriceTables(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]PriceTable, 0, len(rows))
	for _, r := range rows {
		t := PriceTable{Version: r.Version, Status: r.Status, CreatedBy: r.CreatedBy, CreatedAt: r.CreatedAt, ApprovedBy: r.ApprovedBy, ApprovedAt: r.ApprovedAt}
		if err := json.Unmarshal(r.Rules, &t.Rules); err != nil {
			return nil, fmt.Errorf("price table %d: %w", r.Version, err)
		}
		out = append(out, t)
	}
	return out, nil
}

func (s *Store) ProposePriceTable(ctx context.Context, rules pricing.Rules, admin uuid.UUID) (int64, error) {
	data, err := json.Marshal(rules)
	if err != nil {
		return 0, err
	}
	return s.q.InsertPriceTable(ctx, ordersdb.InsertPriceTableParams{Rules: data, CreatedBy: &admin})
}

// ApprovePriceTable activates a draft proposed by another admin and retires
// the active one, atomically. Orders keep the version they were quoted with.
func (s *Store) ApprovePriceTable(ctx context.Context, version int64, admin uuid.UUID) error {
	return s.tx(ctx, func(q *ordersdb.Queries) error {
		t, err := q.LockPriceTable(ctx, version)
		if err != nil {
			return notFound(err)
		}
		if t.Status != "draft" {
			return fmt.Errorf("%w: version %d is %s", ErrConflict, version, t.Status)
		}
		if t.CreatedBy != nil && *t.CreatedBy == admin {
			return ErrSameApprover
		}
		if err := q.RetireActivePriceTable(ctx); err != nil {
			return err
		}
		n, err := q.ActivatePriceTable(ctx, ordersdb.ActivatePriceTableParams{Version: version, ApprovedBy: &admin})
		if err != nil {
			return err
		}
		if n == 0 {
			return ErrConflict
		}
		return nil
	})
}

// SystemActor marks the changes made by a partner, not by a person, in the
// status history.
var SystemActor = uuid.Nil

// ErrRejected is a partner event that does not match what the platform
// knows: unknown charge, other order, wrong amount or wrong shipment.
var ErrRejected = errors.New("partner event rejected")

type Payment struct {
	ChargeID    string    `json:"charge_id"`
	AmountCents int64     `json:"amount_cents"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
}

// LivePayment returns the pending or succeeded charge of the order, or nil.
func (s *Store) LivePayment(ctx context.Context, order uuid.UUID) (*Payment, error) {
	p, err := s.q.LivePayment(ctx, order)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &Payment{ChargeID: p.ChargeID, AmountCents: p.AmountCents, Status: p.Status, CreatedAt: p.CreatedAt}, nil
}

func (s *Store) AddPayment(ctx context.Context, order uuid.UUID, charge string, amount int64) error {
	return s.q.InsertPayment(ctx, ordersdb.InsertPaymentParams{ChargeID: charge, OrderID: order, AmountCents: amount})
}

// PaymentEvent is what the gateway reports, already authenticated by the
// webhook signature.
type PaymentEvent struct {
	EventID     string
	ChargeID    string
	OrderID     uuid.UUID
	AmountCents int64
}

// ApplyPayment marks the order paid when the event matches the charge and the
// order total. A replayed event changes nothing and reports applied=false.
// Everything happens in one transaction: a rejected event leaves no trace in
// the tables.
func (s *Store) ApplyPayment(ctx context.Context, ev PaymentEvent) (applied bool, err error) {
	err = s.tx(ctx, func(q *ordersdb.Queries) error {
		n, err := q.RecordPartnerEvent(ctx, ordersdb.RecordPartnerEventParams{EventID: ev.EventID, Partner: "payments", Type: "payment.succeeded"})
		if err != nil || n == 0 {
			return err
		}
		p, err := q.LockPayment(ctx, ev.ChargeID)
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("%w: unknown charge %s", ErrRejected, ev.ChargeID)
		}
		if err != nil {
			return err
		}
		o, err := q.GetOrder(ctx, ev.OrderID)
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && p.OrderID != o.ID) {
			return fmt.Errorf("%w: charge %s is not of order %s", ErrRejected, ev.ChargeID, ev.OrderID)
		}
		if err != nil {
			return err
		}
		if ev.AmountCents != p.AmountCents || ev.AmountCents != o.TotalCents {
			return fmt.Errorf("%w: amount_mismatch: %d paid, order total %d", ErrRejected, ev.AmountCents, o.TotalCents)
		}
		changed, err := q.SucceedPayment(ctx, p.ChargeID)
		if err != nil {
			return err
		}
		if changed == 0 {
			return fmt.Errorf("%w: charge %s is %s", ErrRejected, p.ChargeID, p.Status)
		}
		if err := transition(ctx, q, o.ID, StatusAwaitingPayment, StatusPaid, SystemActor); err != nil {
			return fmt.Errorf("%w: order %s is %s", ErrRejected, o.ID, o.Status)
		}
		applied = true
		return nil
	})
	return applied, err
}

// SetShipment keeps the carrier's references, once, while in production.
func (s *Store) SetShipment(ctx context.Context, id uuid.UUID, shipment, tracking string) error {
	n, err := s.q.SetShipment(ctx, ordersdb.SetShipmentParams{ID: id, ShipmentID: &shipment, TrackingCode: &tracking})
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("%w: only an order in production without a shipment can ship", ErrConflict)
	}
	return nil
}

// ShippingEvent is what the carrier reports, already authenticated.
type ShippingEvent struct {
	EventID    string
	Type       string // shipment.picked_up or shipment.delivered
	ShipmentID string
	OrderID    uuid.UUID
}

// ApplyShipping moves the order with the carrier: picked up means shipped,
// then delivered. The shipment must be the one the panel created for the
// order.
func (s *Store) ApplyShipping(ctx context.Context, ev ShippingEvent) (applied bool, err error) {
	from, to := StatusInProduction, StatusShipped
	if ev.Type == "shipment.delivered" {
		from, to = StatusShipped, StatusDelivered
	} else if ev.Type != "shipment.picked_up" {
		return false, fmt.Errorf("%w: event type %q", ErrRejected, ev.Type)
	}
	err = s.tx(ctx, func(q *ordersdb.Queries) error {
		n, err := q.RecordPartnerEvent(ctx, ordersdb.RecordPartnerEventParams{EventID: ev.EventID, Partner: "shipping", Type: ev.Type})
		if err != nil || n == 0 {
			return err
		}
		o, err := q.GetOrder(ctx, ev.OrderID)
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && (o.ShipmentID == nil || *o.ShipmentID != ev.ShipmentID)) {
			return fmt.Errorf("%w: shipment %s is not of order %s", ErrRejected, ev.ShipmentID, ev.OrderID)
		}
		if err != nil {
			return err
		}
		if err := transition(ctx, q, o.ID, from, to, SystemActor); err != nil {
			return fmt.Errorf("%w: order %s is %s", ErrRejected, o.ID, o.Status)
		}
		applied = true
		return nil
	})
	return applied, err
}

func transition(ctx context.Context, q *ordersdb.Queries, id uuid.UUID, from, to Status, actor uuid.UUID) error {
	n, err := q.TransitionOrder(ctx, ordersdb.TransitionOrderParams{ID: id, FromStatus: string(from), ToStatus: string(to)})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrConflict
	}
	f := string(from)
	return q.InsertStatusHistory(ctx, ordersdb.InsertStatusHistoryParams{OrderID: id, FromStatus: &f, ToStatus: string(to), Actor: actor})
}

func (s *Store) tx(ctx context.Context, fn func(*ordersdb.Queries) error) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no effect after the commit
	if err := fn(s.q.WithTx(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func notFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}
