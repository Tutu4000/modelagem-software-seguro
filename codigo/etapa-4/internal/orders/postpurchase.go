package orders

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"nuggetmachine/internal/orders/ordersdb"
	"nuggetmachine/internal/partners"
)

var (
	// ErrPaymentPending: the order cannot be cancelled while a charge is in
	// flight at the gateway, or the money could arrive after the cancellation.
	ErrPaymentPending = errors.New("a payment is pending at the gateway")
	// ErrNoAddressChange: there is no pending address change for the order.
	ErrNoAddressChange = errors.New("no pending address change")
	// ErrAddressChangeClosed: the change expired, was locked by too many wrong
	// codes, or the order was dispatched meanwhile.
	ErrAddressChangeClosed = errors.New("the address change is no longer open")
	// ErrWrongCode: the confirmation code does not match.
	ErrWrongCode = errors.New("wrong confirmation code")
)

const (
	// AddressChangeValidity is how long the code sent to the customer holds.
	AddressChangeValidity = 30 * time.Minute
	// MaxCodeAttempts locks the change after this many wrong codes.
	MaxCodeAttempts = 5
	codeDigits      = 8
)

// Cancellable says whether the customer may cancel from this status (RF10).
// From production on, the order is the support's to handle.
func Cancellable(s Status) bool {
	return CanTransition(s, StatusCancelled)
}

// Cancel moves the order to cancelled, from the status it is in now, and
// returns the charge to refund when it was paid. The status changes first, in
// one transaction with the charge, so a technician starting the production
// at the same time either wins or loses cleanly: the refund is only asked for
// an order that is already cancelled.
func (s *Store) Cancel(ctx context.Context, id uuid.UUID, from Status, actor uuid.UUID) (refund *Payment, err error) {
	if !Cancellable(from) {
		return nil, fmt.Errorf("%w: an order %s cannot be cancelled", ErrConflict, from)
	}
	err = s.tx(ctx, func(q *ordersdb.Queries) error {
		p, err := latestPayment(ctx, q, id)
		if err != nil {
			return err
		}
		switch {
		case from == StatusPaid:
			if p == nil || p.Status != "succeeded" {
				return fmt.Errorf("paid order %s without a succeeded charge", id)
			}
			n, err := q.RequestRefund(ctx, p.ChargeID)
			if err != nil {
				return err
			}
			if n == 0 {
				return ErrConflict
			}
			p.Status = "refund_pending"
			refund = p
		case p != nil && p.Status == "pending":
			return ErrPaymentPending
		}
		return transition(ctx, q, id, from, StatusCancelled, actor)
	})
	return refund, err
}

// RefundPending returns the charge of a cancelled order whose refund did not
// complete, so that a new cancel call can finish it.
func (s *Store) RefundPending(ctx context.Context, id uuid.UUID) (*Payment, error) {
	p, err := latestPayment(ctx, s.q, id)
	if err != nil || p == nil || p.Status != "refund_pending" {
		return nil, err
	}
	return p, nil
}

func (s *Store) CompleteRefund(ctx context.Context, charge, refundID string) error {
	_, err := s.q.CompleteRefund(ctx, ordersdb.CompleteRefundParams{ChargeID: charge, RefundID: &refundID})
	return err
}

func latestPayment(ctx context.Context, q *ordersdb.Queries, order uuid.UUID) (*Payment, error) {
	p, err := q.LatestPayment(ctx, order)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &Payment{ChargeID: p.ChargeID, AmountCents: p.AmountCents, Status: p.Status, RefundID: p.RefundID, CreatedAt: p.CreatedAt}, nil
}

// FilePurged says whether the retention already deleted the order's design.
func (s *Store) FilePurged(ctx context.Context, file uuid.UUID) (bool, error) {
	return s.q.FilePurged(ctx, file)
}

// AddressChange is a change proposed by support, waiting for the customer's
// confirmation (C21).
type AddressChange struct {
	ID        uuid.UUID        `json:"id"`
	Address   partners.Address `json:"address"`
	ExpiresAt time.Time        `json:"expires_at"`
	Status    string           `json:"status"`
}

func newAddressChange(c ordersdb.AddressChange) AddressChange {
	return AddressChange{
		ID: c.ID, ExpiresAt: c.ExpiresAt, Status: c.Status,
		Address: partners.Address{
			Recipient: c.NewRecipient, Street: c.NewStreet, Number: c.NewNumber,
			City: c.NewCity, State: c.NewState, PostalCode: c.NewPostalCode,
		},
	}
}

// AddressChangeAllowed is the C21 rule: no change after the dispatch.
func AddressChangeAllowed(o Order) bool {
	switch Status(o.Status) {
	case StatusQuoted, StatusAwaitingReview, StatusAwaitingPayment, StatusPaid, StatusInProduction:
		return o.ShipmentID == nil && o.AddressPurgedAt == nil
	}
	return false
}

// ProposeAddressChange records a pending change and returns the code to send
// to the customer's registered e-mail. Only its hash is stored. A pending
// change of the same order is superseded.
func (s *Store) ProposeAddressChange(ctx context.Context, order, by uuid.UUID, ticket *uuid.UUID, a partners.Address, now time.Time) (AddressChange, string, error) {
	id := uuid.Must(uuid.NewV7())
	code, err := newCode()
	if err != nil {
		return AddressChange{}, "", err
	}
	row := ordersdb.InsertAddressChangeParams{
		ID: id, OrderID: order, RequestedBy: by, TicketID: ticket,
		NewRecipient: a.Recipient, NewStreet: a.Street, NewNumber: a.Number,
		NewCity: a.City, NewState: a.State, NewPostalCode: a.PostalCode,
		CodeHash: codeHash(id, code), ExpiresAt: now.Add(AddressChangeValidity),
	}
	err = s.tx(ctx, func(q *ordersdb.Queries) error {
		if err := q.SupersedeAddressChanges(ctx, order); err != nil {
			return err
		}
		return q.InsertAddressChange(ctx, row)
	})
	return AddressChange{ID: id, Address: a, ExpiresAt: row.ExpiresAt, Status: "pending"}, code, err
}

// WithdrawAddressChange closes a change whose code could not be sent.
func (s *Store) WithdrawAddressChange(ctx context.Context, id uuid.UUID) error {
	return s.q.DecideAddressChange(ctx, ordersdb.DecideAddressChangeParams{ID: id, Status: "expired"})
}

// PendingAddressChange returns nil when there is none.
func (s *Store) PendingAddressChange(ctx context.Context, order uuid.UUID) (*AddressChange, error) {
	c, err := s.q.PendingAddressChange(ctx, order)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	ac := newAddressChange(c)
	return &ac, nil
}

// ConfirmResult tells the handler what happened, so it can audit it.
type ConfirmResult struct {
	Change       AddressChange
	Before       partners.Address
	AttemptsLeft int
}

// ConfirmAddressChange checks the code and, when it matches, changes the
// order's address. A wrong code, an expiry or a lock is committed before the
// error is returned: the attempts count even though the confirmation fails.
func (s *Store) ConfirmAddressChange(ctx context.Context, order uuid.UUID, code string, now time.Time) (ConfirmResult, error) {
	var res ConfirmResult
	var outcome error
	err := s.tx(ctx, func(q *ordersdb.Queries) error {
		c, err := q.LockPendingAddressChange(ctx, order)
		if errors.Is(err, pgx.ErrNoRows) {
			outcome = ErrNoAddressChange
			return nil
		}
		if err != nil {
			return err
		}
		res.Change = newAddressChange(c)
		if now.After(c.ExpiresAt) {
			outcome = fmt.Errorf("%w: it expired at %s", ErrAddressChangeClosed, c.ExpiresAt)
			return q.DecideAddressChange(ctx, ordersdb.DecideAddressChangeParams{ID: c.ID, Status: "expired"})
		}
		if subtle.ConstantTimeCompare(codeHash(c.ID, code), c.CodeHash) != 1 {
			r, err := q.CountAddressAttempt(ctx, ordersdb.CountAddressAttemptParams{ID: c.ID, MaxAttempts: MaxCodeAttempts})
			if err != nil {
				return err
			}
			res.AttemptsLeft = max(0, MaxCodeAttempts-int(r.Attempts))
			outcome = ErrWrongCode
			if r.Status == "locked" {
				outcome = fmt.Errorf("%w: locked after %d wrong codes", ErrAddressChangeClosed, r.Attempts)
			}
			return nil
		}
		o, err := q.GetOrder(ctx, order)
		if err != nil {
			return err
		}
		res.Before = AddressOf(o)
		a := res.Change.Address
		n, err := q.SetAddress(ctx, ordersdb.SetAddressParams{
			ID: order, Recipient: a.Recipient, Street: a.Street, Number: a.Number,
			City: a.City, State: a.State, PostalCode: a.PostalCode,
		})
		if err != nil {
			return err
		}
		if n == 0 {
			outcome = fmt.Errorf("%w: the order was dispatched", ErrAddressChangeClosed)
			return q.DecideAddressChange(ctx, ordersdb.DecideAddressChangeParams{ID: c.ID, Status: "expired"})
		}
		res.Change.Status = "confirmed"
		return q.DecideAddressChange(ctx, ordersdb.DecideAddressChangeParams{ID: c.ID, Status: "confirmed"})
	})
	if err != nil {
		return res, err
	}
	return res, outcome
}

// newCode is a uniform 8-digit code from crypto/rand.
func newCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(100_000_000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%0*d", codeDigits, n), nil
}

// codeHash binds the code to its change, so the same code in two changes
// gives two hashes. The code lives 30 minutes and allows 5 attempts.
func codeHash(id uuid.UUID, code string) []byte {
	sum := sha256.Sum256(append(id[:], strings.TrimSpace(code)...))
	return sum[:]
}

// MaskPostalCode keeps the region of a CEP for the audit log, without the
// full address (C17).
func MaskPostalCode(cep string) string {
	if len(cep) < 5 {
		return "***"
	}
	return cep[:5] + "-***"
}

// ChangedFields names the address fields that differ, for the audit log.
func ChangedFields(a, b partners.Address) []string {
	var out []string
	for _, f := range []struct {
		name string
		a, b string
	}{
		{"recipient", a.Recipient, b.Recipient}, {"street", a.Street, b.Street}, {"number", a.Number, b.Number},
		{"city", a.City, b.City}, {"state", a.State, b.State}, {"postal_code", a.PostalCode, b.PostalCode},
	} {
		if f.a != f.b {
			out = append(out, f.name)
		}
	}
	return out
}
