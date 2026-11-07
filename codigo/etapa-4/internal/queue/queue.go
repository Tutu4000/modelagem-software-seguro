// Package queue fica em memória até o RabbitMQ (E4).
package queue

import (
	"context"
	"slices"
	"sync"

	"nuggetmachine/internal/auth"
	"nuggetmachine/internal/files"
	"nuggetmachine/internal/stl"
)

type Task struct {
	FileID    files.FileID
	Owner     auth.UserID
	SHA256    [32]byte
	Triangles uint32
}

type Memory struct {
	mu    sync.Mutex
	tasks []Task
}

func NewMemory() *Memory { return &Memory{} }

func (q *Memory) Enqueue(_ context.Context, f files.File, model stl.Validated) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.tasks = append(q.tasks, Task{
		FileID:    f.ID,
		Owner:     f.Owner,
		SHA256:    f.SHA256,
		Triangles: model.Triangles(),
	})
	return nil
}

func (q *Memory) Tasks() []Task {
	q.mu.Lock()
	defer q.mu.Unlock()
	return slices.Clone(q.tasks)
}

// StatusOf vê toda tarefa em memória como em processamento: não há worker.
func (q *Memory) StatusOf(_ context.Context, id files.FileID) (files.TaskStatus, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, t := range q.tasks {
		if t.FileID == id {
			return files.TaskStatus{State: "processando"}, nil
		}
	}
	return files.TaskStatus{}, files.ErrNotFound
}
