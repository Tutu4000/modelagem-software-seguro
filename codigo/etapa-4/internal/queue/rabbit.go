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
	// MaxActive é a cota de tarefas em processamento por conta. As outras
	// esperam a vez (E4.3, C10).
	MaxActive = 3
	// workerURLTTL cobre o tempo na fila. Uma URL vencida faz a tarefa falhar,
	// e não abre o arquivo para mais ninguém.
	workerURLTTL = 30 * time.Minute
)

type Presigner interface {
	PresignInternal(key string, ttl time.Duration) (string, error)
}

// Rabbit é o módulo fila: guarda o estado das tarefas no Postgres, com o papel
// "fila", e as publica no RabbitMQ. As queries ficam em queries.sql.
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

// Enqueue cria a tarefa. Dentro da cota ela é publicada antes do commit: se o
// commit falhar, o worker processa uma tarefa que não existe e o resultado é
// descartado, mas nenhuma tarefa gravada fica sem mensagem.
func (q *Rabbit) Enqueue(ctx context.Context, f files.File, model stl.Validated) error {
	tx, err := q.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // sem efeito depois do commit
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
		ID:         t.id,
		ArquivoID:  t.file,
		Dono:       owner,
		Sha256:     t.sha256,
		Triangulos: int64(model.Triangles()),
		Status:     status,
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
		// A tarefa já existe: uma falha da auditoria vai para o log, sem
		// recusar o upload.
		err := q.audit.Record(audit.Event{
			Name:   "quota.waiting",
			Actor:  owner.String(),
			Action: "file.process",
			Target: t.file.String(),
			Result: "waiting",
			Reason: fmt.Sprintf("%d tarefas em processamento", active),
		})
		if err != nil {
			q.log.Error("falha ao auditar a espera", "error", err)
		}
	}
	return nil
}

func (q *Rabbit) StatusOf(ctx context.Context, id files.FileID) (files.TaskStatus, error) {
	row, err := q.q.TaskStatusByFile(ctx, uuid.UUID(id))
	if err != nil {
		return files.TaskStatus{}, fmt.Errorf("tarefa do arquivo %s: %w", id, err)
	}
	st := files.TaskStatus{State: row.Status}
	if row.VolumeMm3 != nil && len(row.CaixaMm) == 3 {
		st.VolumeMM3 = row.VolumeMm3
		st.BoxMM = &[3]float64{row.CaixaMm[0], row.CaixaMm[1], row.CaixaMm[2]}
	}
	return st, nil
}

// ConsumeResults roda até o contexto acabar ou a conexão cair.
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
				return errors.New("o canal de resultados fechou")
			}
			var m ResultMessage
			if err := json.Unmarshal(d.Body, &m); err != nil || (m.Status != StatusDone && m.Status != StatusFailed) {
				q.log.Error("resultado inválido descartado", "error", err)
				_ = d.Reject(false)
				continue
			}
			if err := q.handleResult(ctx, m); err != nil {
				q.log.Error("falha ao gravar o resultado", "tarefa", m.TaskID, "error", err)
				_ = d.Nack(false, true)
				continue
			}
			_ = d.Ack(false)
		}
	}
}

// handleResult é idempotente: só uma tarefa em processamento muda de estado, e
// um resultado repetido não faz nada (E4.6). Depois libera a próxima tarefa da
// mesma conta.
func (q *Rabbit) handleResult(ctx context.Context, m ResultMessage) error {
	tx, err := q.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // sem efeito depois do commit
	qtx := q.q.WithTx(tx)

	owner, err := qtx.TaskOwner(ctx, m.TaskID)
	if errors.Is(err, pgx.ErrNoRows) {
		q.log.Warn("resultado de tarefa desconhecida", "tarefa", m.TaskID)
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
		finish.VolumeMm3, finish.CaixaMm = &m.VolumeMM3, m.BoxMM[:]
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
	if err := q.publish(ctx, task{id: next.ID, file: next.ArquivoID, sha256: next.Sha256}); err != nil {
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
