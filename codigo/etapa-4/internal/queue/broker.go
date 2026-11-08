package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sync"
	"time"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
)

// Names declared in deploy/rabbitmq/topology.json. The users cannot declare
// anything (E4.1).
const (
	ExchangeTasks   = "tasks"
	QueueTasks      = "tasks"
	ExchangeResults = "results"
	QueueResults    = "results"
)

// TaskMessage is all the worker receives: only an already validated file, the
// hash to check and a URL that reads only that file (E4.2, C16).
type TaskMessage struct {
	TaskID uuid.UUID `json:"task_id"`
	FileID uuid.UUID `json:"file_id"`
	SHA256 string    `json:"sha256"`
	URL    string    `json:"url"`
}

type ResultMessage struct {
	TaskID    uuid.UUID  `json:"task_id"`
	Status    string     `json:"status"` // StatusDone or StatusFailed
	VolumeMM3 float64    `json:"volume_mm3,omitempty"`
	BBoxMM    [3]float64 `json:"bbox_mm"`
}

const (
	StatusWaiting    = "waiting"
	StatusProcessing = "processing"
	StatusDone       = "done"
	StatusFailed     = "failed"
)

// Broker is a connection to RabbitMQ. When it drops, the process should exit
// and Docker restarts it: simpler and more predictable than reconnecting.
type Broker struct {
	conn *amqp.Connection

	mu sync.Mutex
	ch *amqp.Channel // in confirm mode, used only for publishing
}

func Dial(ctx context.Context, host, user string, password []byte) (*Broker, error) {
	u := url.URL{Scheme: "amqp", Host: host, User: url.UserPassword(user, string(password)), Path: "/"}
	var (
		conn *amqp.Connection
		err  error
	)
	for range 30 {
		if conn, err = amqp.Dial(u.String()); err == nil {
			break
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	if err != nil {
		return nil, fmt.Errorf("RabbitMQ at %s as %s: %w", host, user, err)
	}
	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	if err := ch.Confirm(false); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return &Broker{conn: conn, ch: ch}, nil
}

// Publish only returns after RabbitMQ confirms: the persistent message is
// already in the quorum queue.
func (b *Broker) Publish(ctx context.Context, exchange string, msg any) error {
	body, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	confirm, err := b.ch.PublishWithDeferredConfirmWithContext(ctx, exchange, exchange, false, false, amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		Body:         body,
	})
	if err != nil {
		return err
	}
	ok, err := confirm.WaitContext(ctx)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("RabbitMQ rejected the message")
	}
	return nil
}

// Consume opens a dedicated channel with at most prefetch unacked messages.
func (b *Broker) Consume(queue string, prefetch int) (<-chan amqp.Delivery, error) {
	ch, err := b.conn.Channel()
	if err != nil {
		return nil, err
	}
	if err := ch.Qos(prefetch, 0, false); err != nil {
		return nil, err
	}
	return ch.Consume(queue, "", false, false, false, false, nil)
}

// Closed receives a value when the connection drops.
func (b *Broker) Closed() <-chan *amqp.Error {
	return b.conn.NotifyClose(make(chan *amqp.Error, 1))
}

func (b *Broker) Close() error { return b.conn.Close() }
