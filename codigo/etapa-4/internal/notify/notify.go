// Package notify carries the notifications to the customer (E8.4). A message
// has only the recipient and the text: the notifier that delivers it has no
// access to the database, the files or anything else about the order.
package notify

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
)

// Names declared in deploy/rabbitmq/topology.json.
const (
	Exchange = "notifications"
	Queue    = "notifications"
)

const (
	maxAddress = 254
	maxSubject = 200
	maxText    = 4000
)

type Message struct {
	ID      uuid.UUID `json:"id"`
	To      string    `json:"to"`
	Subject string    `json:"subject"`
	Text    string    `json:"text"`
}

// New gives the message a UUIDv7, which the notifier uses as Message-ID.
func New(to, subject, text string) Message {
	return Message{ID: uuid.Must(uuid.NewV7()), To: to, Subject: subject, Text: text}
}

// Validate refuses what could add mail headers (a line break in the address
// or the subject) and anything that is not a bare address.
func (m Message) Validate() error {
	if m.ID == uuid.Nil {
		return errors.New("message without ID")
	}
	if strings.ContainsAny(m.To, "\r\n") || strings.ContainsAny(m.Subject, "\r\n") {
		return errors.New("line break in a header")
	}
	if len(m.To) > maxAddress {
		return errors.New("address too long")
	}
	addr, err := mail.ParseAddress(m.To)
	if err != nil || addr.Name != "" || addr.Address != m.To {
		return fmt.Errorf("not a bare e-mail address: %q", m.To)
	}
	if n := utf8.RuneCountInString(m.Subject); n < 1 || n > maxSubject {
		return fmt.Errorf("subject must have 1 to %d characters", maxSubject)
	}
	if n := utf8.RuneCountInString(m.Text); n < 1 || n > maxText {
		return fmt.Errorf("text must have 1 to %d characters", maxText)
	}
	if !utf8.ValidString(m.Subject) || !utf8.ValidString(m.Text) {
		return errors.New("invalid UTF-8")
	}
	return nil
}

// Sender queues a message for delivery.
type Sender interface {
	Send(ctx context.Context, m Message) error
}

type publisher interface {
	Publish(ctx context.Context, exchange string, msg any) error
}

// Rabbit publishes to the notifications queue, with the broker's confirm.
type Rabbit struct{ broker publisher }

func NewRabbit(b publisher) *Rabbit { return &Rabbit{broker: b} }

func (r *Rabbit) Send(ctx context.Context, m Message) error {
	if err := m.Validate(); err != nil {
		return err
	}
	return r.broker.Publish(ctx, Exchange, m)
}
