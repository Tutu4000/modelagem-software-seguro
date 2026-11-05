// Package storage simula o armazenamento de objetos até o Garage (E3.3).
package storage

import (
	"context"
	"errors"
	"net/http"
	"time"
)

var errNotImplemented = errors.New("não implementado")

type Memory struct {
	secret  []byte
	baseURL string
	now     func() time.Time
}

func NewMemory(secret []byte, baseURL string, now func() time.Time) *Memory {
	return &Memory{secret: secret, baseURL: baseURL, now: now}
}

func (m *Memory) Put(ctx context.Context, key string, data []byte) error { return errNotImplemented }

func (m *Memory) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "não implementado", http.StatusNotImplemented)
}

func (m *Memory) Count() int { return 0 }
