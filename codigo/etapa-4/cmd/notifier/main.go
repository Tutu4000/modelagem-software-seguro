// Command notifier delivers the notifications (E8.4). It consumes the
// notifications queue and sends each message by SMTP. It only ever sees the
// recipient and the text: no database, no files, no order data. Its logs
// carry the message ID, never the address (C17).
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"mime"
	"mime/quotedprintable"
	"net/smtp"
	"os"
	"os/signal"
	"syscall"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"nuggetmachine/internal/config"
	"nuggetmachine/internal/notify"
	"nuggetmachine/internal/queue"
)

type notifier struct {
	smtp string
	from string
	log  *slog.Logger
}

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(log); err != nil {
		log.Error("the notifier stopped", "error", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	var env config.Env
	host := env.String("AMQP_HOST", "rabbitmq:5672")
	user := env.String("AMQP_USER", "notifier")
	password := env.Secret("AMQP_PASSWORD")
	n := notifier{
		smtp: env.String("SMTP_ADDR", "mailpit:1025"),
		from: env.String("MAIL_FROM", "nao-responda@nugget.local"),
		log:  log,
	}
	if err := env.Err(); err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	broker, err := queue.Dial(ctx, host, user, password)
	if err != nil {
		return err
	}
	defer broker.Close()
	closed := broker.Closed()
	deliveries, err := broker.Consume(notify.Queue, 4)
	if err != nil {
		return err
	}
	log.Info("notifier up")

	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-closed:
			return fmt.Errorf("connection to RabbitMQ dropped: %v", err)
		case d, ok := <-deliveries:
			if !ok {
				return errors.New("the notifications channel closed")
			}
			n.handle(d)
		}
	}
}

// handle acks only after the SMTP server took the message: delivery is at
// least once. A message that can never be sent goes to the failed queue.
func (n notifier) handle(d amqp.Delivery) {
	var m notify.Message
	if err := json.Unmarshal(d.Body, &m); err != nil {
		n.log.Error("unreadable notification sent to the failed queue", "error", err)
		_ = d.Reject(false)
		return
	}
	log := n.log.With("message", m.ID)
	if err := m.Validate(); err != nil {
		log.Error("invalid notification sent to the failed queue", "error", err)
		_ = d.Reject(false)
		return
	}
	if err := smtp.SendMail(n.smtp, nil, n.from, []string{m.To}, n.compose(m)); err != nil {
		log.Warn("notification returned to the queue", "error", err)
		_ = d.Nack(false, true)
		return
	}
	log.Info("notification sent")
	_ = d.Ack(false)
}

// compose builds a plain-text UTF-8 message. Validate already refused line
// breaks in the headers, and the subject is encoded as RFC 2047.
func (n notifier) compose(m notify.Message) []byte {
	var b bytes.Buffer
	fmt.Fprintf(&b, "From: %s\r\n", n.from)
	fmt.Fprintf(&b, "To: %s\r\n", m.To)
	fmt.Fprintf(&b, "Subject: %s\r\n", mime.QEncoding.Encode("utf-8", m.Subject))
	fmt.Fprintf(&b, "Message-ID: <%s@nugget.local>\r\n", m.ID)
	fmt.Fprintf(&b, "Date: %s\r\n", time.Now().UTC().Format(time.RFC1123Z))
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	b.WriteString("Content-Transfer-Encoding: quoted-printable\r\n\r\n")
	w := quotedprintable.NewWriter(&b)
	_, _ = w.Write([]byte(m.Text))
	_ = w.Close()
	return b.Bytes()
}
