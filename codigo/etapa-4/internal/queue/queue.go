// Package queue fica em memória até o RabbitMQ (E4).
package queue

import (
	"context"
	"errors"

	"nuggetmachine/internal/auth"
	"nuggetmachine/internal/files"
	"nuggetmachine/internal/stl"
)

var errNotImplemented = errors.New("não implementado")

type Task struct {
	FileID    files.FileID
	Owner     auth.UserID
	SHA256    [32]byte
	Triangles uint32
}

type Memory struct{}

func NewMemory() *Memory { return &Memory{} }

func (q *Memory) Enqueue(ctx context.Context, f files.File, model stl.Validated) error {
	return errNotImplemented
}

func (q *Memory) Tasks() []Task { return nil }
