// Package audit writes audit events as JSON, with no password, token, card
// number or file content (C17, C18).
package audit

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync"
	"time"
)

type Event struct {
	Time   time.Time `json:"time"`
	Name   string    `json:"event"`
	Actor  string    `json:"actor"`
	Action string    `json:"action"`
	Target string    `json:"target"`
	Result string    `json:"result"`
	Reason string    `json:"reason,omitempty"`
	Origin string    `json:"origin"`
}

// Store keeps events outside the process, such as the audit database (C18).
type Store interface {
	Insert(ctx context.Context, e Event) error
}

type Logger struct {
	mu    sync.Mutex
	w     io.Writer
	store Store
}

func New(w io.Writer) *Logger { return &Logger{w: w} }

// NewWithStore also writes every event to the store.
func NewWithStore(w io.Writer, s Store) *Logger { return &Logger{w: w, store: s} }

// storeTimeout does not depend on the request: a client that disconnects does
// not erase its own trail.
const storeTimeout = 3 * time.Second

// Record writes one line per event, with the time in UTC.
func (l *Logger) Record(e Event) error {
	e.Time = time.Now().UTC()
	line, err := json.Marshal(e)
	if err != nil {
		return err
	}
	l.mu.Lock()
	_, err = l.w.Write(append(line, '\n'))
	l.mu.Unlock()
	if l.store != nil {
		ctx, cancel := context.WithTimeout(context.Background(), storeTimeout)
		defer cancel()
		err = errors.Join(err, l.store.Insert(ctx, e))
	}
	return err
}
