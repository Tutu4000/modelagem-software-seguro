// Package support is the support channel (RF11): tickets opened by the
// customer, optionally about one of their orders, and answered by support in
// the panel. The API reaches it with the "support" database role and the
// panel with "panel_support".
package support

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"nuggetmachine/internal/notify"
	"nuggetmachine/internal/support/supportdb"
)

const (
	StatusOpen     = "open"
	StatusAnswered = "answered"
	StatusClosed   = "closed"

	// MaxOpenTickets bounds what one customer can open at once.
	MaxOpenTickets = 10
	maxSubject     = 200
	maxBody        = 2000
)

var (
	// ErrNotFound covers a missing ticket and someone else's alike (D01).
	ErrNotFound = errors.New("ticket not found")
	// ErrNotYourOrder: the ticket points at an order of another customer.
	ErrNotYourOrder = errors.New("the order is not the customer's")
	// ErrClosed: the ticket no longer takes messages.
	ErrClosed = errors.New("the ticket is closed")
	// ErrTooManyOpen: the customer reached MaxOpenTickets.
	ErrTooManyOpen = errors.New("too many open tickets")
)

type Ticket struct {
	ID        uuid.UUID  `json:"id"`
	OrderID   *uuid.UUID `json:"order_id"`
	Subject   string     `json:"subject"`
	Status    string     `json:"status"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	Messages  []Message  `json:"messages,omitempty"`

	owner uuid.UUID
	email string
}

// StaffTicket adds the owner, for the panel.
type StaffTicket struct {
	Ticket
	OwnerID uuid.UUID `json:"owner_id"`
}

// Message has no author ID: the customer sees "staff", not who on the staff.
// The body is plain text, as typed. A front-end must show it as text, never
// as HTML (stored XSS, E8.6).
type Message struct {
	ID         uuid.UUID `json:"id"`
	AuthorKind string    `json:"author_kind"` // customer or staff
	Body       string    `json:"body"`
	CreatedAt  time.Time `json:"created_at"`
}

func (t Ticket) Staff() StaffTicket { return StaffTicket{Ticket: t, OwnerID: t.owner} }

func newTicket(r supportdb.Ticket) Ticket {
	return Ticket{
		ID: r.ID, OrderID: r.OrderID, Subject: r.Subject, Status: r.Status,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, owner: r.OwnerID, email: r.ContactEmail,
	}
}

// CleanText normalizes line breaks and refuses control characters other than
// the line break, which could hide content or break a log line.
func CleanText(s string, max int) (string, error) {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\r\n", "\n"))
	n := utf8.RuneCountInString(s)
	if n < 1 || n > max {
		return "", fmt.Errorf("text must have 1 to %d characters", max)
	}
	if !utf8.ValidString(s) {
		return "", errors.New("invalid UTF-8")
	}
	for _, r := range s {
		if unicode.IsControl(r) && r != '\n' {
			return "", fmt.Errorf("control character %U", r)
		}
	}
	return s, nil
}

type Store struct {
	db *pgxpool.Pool
	q  *supportdb.Queries
}

func NewStore(db *pgxpool.Pool) *Store { return &Store{db: db, q: supportdb.New(db)} }

// Open creates a ticket with its first message, after checking that the
// order, when given, is the customer's.
func (s *Store) Open(ctx context.Context, owner uuid.UUID, email string, order *uuid.UUID, subject, body string) (Ticket, error) {
	id := uuid.Must(uuid.NewV7())
	err := s.tx(ctx, func(q *supportdb.Queries) error {
		if order != nil {
			owned, err := q.OrderOwnedBy(ctx, supportdb.OrderOwnedByParams{OrderID: *order, OwnerID: owner})
			if err != nil {
				return err
			}
			if !owned {
				return ErrNotYourOrder
			}
		}
		open, err := q.CountOpenTickets(ctx, owner)
		if err != nil {
			return err
		}
		if open >= MaxOpenTickets {
			return ErrTooManyOpen
		}
		if err := q.InsertTicket(ctx, supportdb.InsertTicketParams{
			ID: id, OwnerID: owner, OrderID: order, Subject: subject, ContactEmail: email,
		}); err != nil {
			return err
		}
		return q.InsertMessage(ctx, supportdb.InsertMessageParams{TicketID: id, AuthorID: owner, AuthorKind: "customer", Body: body})
	})
	if err != nil {
		return Ticket{}, err
	}
	return s.withMessages(ctx, id, s.q.GetTicket)
}

func (s *Store) GetForOwner(ctx context.Context, owner, id uuid.UUID) (Ticket, error) {
	return s.withMessages(ctx, id, func(ctx context.Context, id uuid.UUID) (supportdb.Ticket, error) {
		return s.q.GetTicketForOwner(ctx, supportdb.GetTicketForOwnerParams{ID: id, OwnerID: owner})
	})
}

func (s *Store) Get(ctx context.Context, id uuid.UUID) (Ticket, error) {
	return s.withMessages(ctx, id, s.q.GetTicket)
}

func (s *Store) withMessages(ctx context.Context, id uuid.UUID, get func(context.Context, uuid.UUID) (supportdb.Ticket, error)) (Ticket, error) {
	r, err := get(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Ticket{}, ErrNotFound
	}
	if err != nil {
		return Ticket{}, err
	}
	t := newTicket(r)
	rows, err := s.q.ListMessages(ctx, id)
	if err != nil {
		return Ticket{}, err
	}
	for _, m := range rows {
		t.Messages = append(t.Messages, Message{ID: m.ID, AuthorKind: m.AuthorKind, Body: m.Body, CreatedAt: m.CreatedAt})
	}
	return t, nil
}

func (s *Store) ListForOwner(ctx context.Context, owner uuid.UUID) ([]Ticket, error) {
	rows, err := s.q.ListTicketsForOwner(ctx, owner)
	return tickets(rows), err
}

func (s *Store) ListByStatus(ctx context.Context, status string) ([]Ticket, error) {
	rows, err := s.q.ListTicketsByStatus(ctx, status)
	return tickets(rows), err
}

func tickets(rows []supportdb.Ticket) []Ticket {
	out := make([]Ticket, 0, len(rows))
	for _, r := range rows {
		out = append(out, newTicket(r))
	}
	return out
}

// Reply adds a message and moves the ticket: a staff reply makes it
// answered, a customer reply opens it again. A closed ticket refuses both.
func (s *Store) Reply(ctx context.Context, t Ticket, author uuid.UUID, staff bool, body string) error {
	kind, status := "customer", StatusOpen
	if staff {
		kind, status = "staff", StatusAnswered
	}
	return s.tx(ctx, func(q *supportdb.Queries) error {
		n, err := q.SetTicketStatus(ctx, supportdb.SetTicketStatusParams{ID: t.ID, Status: status})
		if err != nil {
			return err
		}
		if n == 0 {
			return ErrClosed
		}
		return q.InsertMessage(ctx, supportdb.InsertMessageParams{TicketID: t.ID, AuthorID: author, AuthorKind: kind, Body: body})
	})
}

func (s *Store) Close(ctx context.Context, id uuid.UUID) error {
	n, err := s.q.SetTicketStatus(ctx, supportdb.SetTicketStatusParams{ID: id, Status: StatusClosed})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrClosed
	}
	return nil
}

func (s *Store) tx(ctx context.Context, fn func(*supportdb.Queries) error) error {
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

// NoticeReply only says there is an answer: the conversation stays behind
// the login.
func NoticeReply(t Ticket) notify.Message {
	short := t.ID.String()[:8]
	return notify.New(t.email, "Resposta no chamado "+short,
		"O suporte respondeu o seu chamado "+short+" (\""+t.Subject+"\").\n\n"+
			"Entre na plataforma para ler a resposta.")
}
