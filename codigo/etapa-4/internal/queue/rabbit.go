package queue

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"nuggetmachine/internal/audit"
	"nuggetmachine/internal/files"
	"nuggetmachine/internal/queue/queuedb"
	"nuggetmachine/internal/stl"
)

const (
	// MaxActive is the per-account quota of tasks being processed. The others
	// wait their turn (E4.3, C10).
	MaxActive = 3
	// workerURLTTL covers the time spent in the queue. An expired URL makes the
	// task fail, and does not open the file to anyone else.
	workerURLTTL = 30 * time.Minute
)

type Presigner interface {
	PresignInternal(key string, ttl time.Duration) (string, error)
}

// Rabbit is the queue module: it keeps task state in Postgres, with the
// "queue" role, and publishes the tasks to RabbitMQ. The queries live in
// queries.sql.
type Rabbit struct {
	db     *pgxpool.Pool
	q      *queuedb.Queries
	broker *Broker
	urls   Presigner
	audit  *audit.Logger
	log    *slog.Logger
}

func NewRabbit(db *pgxpool.Pool, b *Broker, urls Presigner, a *audit.Logger, l *slog.Logger) *Rabbit {
	return &Rabbit{db: db, q: queuedb.New(db), broker: b, urls: urls, audit: a, log: l}
}

type task struct {
	id     uuid.UUID
	file   uuid.UUID
	sha256 []byte
}

// Enqueue creates the task. Within the quota it is published before the
// commit: if the commit fails, the worker processes a task that does not exist
// and its result is discarded, but no stored task is left without a message.
func (q *Rabbit) Enqueue(ctx context.Context, f files.File, model stl.Validated) error {
	tx, err := q.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no effect after the commit
	qtx := q.q.WithTx(tx)

	owner := uuid.UUID(f.Owner)
	if err := qtx.LockOwner(ctx, owner.String()); err != nil {
		return err
	}
	active, err := qtx.CountProcessing(ctx, owner)
	if err != nil {
		return err
	}
	t := task{id: uuid.New(), file: uuid.UUID(f.ID), sha256: f.SHA256[:]}
	status := StatusProcessing
	if active >= MaxActive {
		status = StatusWaiting
	}
	err = qtx.InsertTask(ctx, queuedb.InsertTaskParams{
		ID:        t.id,
		FileID:    t.file,
		OwnerID:   owner,
		Sha256:    t.sha256,
		Triangles: int64(model.Triangles()),
		Status:    status,
	})
	if err != nil {
		return err
	}
	if status == StatusProcessing {
		if err := q.publish(ctx, t); err != nil {
			return err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	if status == StatusWaiting {
		// The task already exists: an audit failure goes to the log without
		// rejecting the upload.
		err := q.audit.Record(audit.Event{
			Name:   "quota.waiting",
			Actor:  owner.String(),
			Action: "file.process",
			Target: t.file.String(),
			Result: "waiting",
			Reason: fmt.Sprintf("%d tasks processing", active),
		})
		if err != nil {
			q.log.Error("failed to audit the wait", "error", err)
		}
	}
	return nil
}

func (q *Rabbit) StatusOf(ctx context.Context, id files.FileID) (files.TaskStatus, error) {
	row, err := q.q.TaskStatusByFile(ctx, uuid.UUID(id))
	if err != nil {
		return files.TaskStatus{}, fmt.Errorf("task of file %s: %w", id, err)
	}
	st := files.TaskStatus{State: row.Status}
	if row.VolumeMm3 != nil && len(row.BboxMm) == 3 {
		st.VolumeMM3 = row.VolumeMm3
		st.BBoxMM = &[3]float64{row.BboxMm[0], row.BboxMm[1], row.BboxMm[2]}
	}
	return st, nil
}

// ConsumeResults runs until the context ends or the connection drops.
func (q *Rabbit) ConsumeResults(ctx context.Context) error {
	deliveries, err := q.broker.Consume(QueueResults, 10)
	if err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case d, ok := <-deliveries:
			if !ok {
				return errors.New("the results channel closed")
			}
			var m ResultMessage
			if err := json.Unmarshal(d.Body, &m); err != nil || (m.Status != StatusDone && m.Status != StatusFailed) {
				q.log.Error("invalid result discarded", "error", err)
				_ = d.Reject(false)
				continue
			}
			if err := q.handleResult(ctx, m); err != nil {
				q.log.Error("failed to store the result", "task", m.TaskID, "error", err)
				_ = d.Nack(false, true)
				continue
			}
			_ = d.Ack(false)
		}
	}
}

// handleResult is idempotent: only a task that is processing changes state,
// and a repeated result does nothing (E4.6). Then it releases the account's
// next task.
func (q *Rabbit) handleResult(ctx context.Context, m ResultMessage) error {
	tx, err := q.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no effect after the commit
	qtx := q.q.WithTx(tx)

	owner, err := qtx.TaskOwner(ctx, m.TaskID)
	if errors.Is(err, pgx.ErrNoRows) {
		q.log.Warn("result for an unknown task", "task", m.TaskID)
		return nil
	}
	if err != nil {
		return err
	}
	if err := qtx.LockOwner(ctx, owner.String()); err != nil {
		return err
	}

	finish := queuedb.FinishTaskParams{ID: m.TaskID, Status: m.Status}
	if m.Status == StatusDone {
		finish.VolumeMm3, finish.BboxMm = &m.VolumeMM3, m.BBoxMM[:]
	}
	changed, err := qtx.FinishTask(ctx, finish)
	if err != nil {
		return err
	}
	if changed == 0 {
		return tx.Commit(ctx)
	}

	next, err := qtx.OldestWaiting(ctx, owner)
	if errors.Is(err, pgx.ErrNoRows) {
		return tx.Commit(ctx)
	}
	if err != nil {
		return err
	}
	if err := qtx.StartTask(ctx, next.ID); err != nil {
		return err
	}
	if err := q.publish(ctx, task{id: next.ID, file: next.FileID, sha256: next.Sha256}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (q *Rabbit) publish(ctx context.Context, t task) error {
	url, err := q.urls.PresignInternal(t.file.String(), workerURLTTL)
	if err != nil {
		return err
	}
	return q.broker.Publish(ctx, ExchangeTasks, TaskMessage{
		TaskID: t.id,
		FileID: t.file,
		SHA256: hex.EncodeToString(t.sha256),
		URL:    url,
	})
}
