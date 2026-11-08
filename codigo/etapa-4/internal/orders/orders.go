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
			PriceTableVersion: o.PriceTableVersion, UnitCents: o.UnitCents, DiscountCents: o.DiscountCents, TotalCents: o.TotalCents,
			VolumeMm3: o.VolumeMm3, BboxMm: o.BboxMm, DfmFlags: o.DfmFlags, Status: o.Status, QuoteExpiresAt: o.QuoteExpiresAt,
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
