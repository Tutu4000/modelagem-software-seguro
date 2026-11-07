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
// "fila", e as publica no RabbitMQ.
type Rabbit struct {
	db     *pgxpool.Pool
	broker *Broker
	urls   Presigner
	audit  *audit.Logger
	log    *slog.Logger
}

func NewRabbit(db *pgxpool.Pool, b *Broker, urls Presigner, a *audit.Logger, l *slog.Logger) *Rabbit {
	return &Rabbit{db: db, broker: b, urls: urls, audit: a, log: l}
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

	owner := uuid.UUID(f.Owner)
	if err := lockOwner(ctx, tx, owner); err != nil {
		return err
	}
	var active int
	err = tx.QueryRow(ctx,
		`SELECT count(*) FROM tarefas WHERE dono = $1 AND status = $2`, owner, StatusProcessing).Scan(&active)
	if err != nil {
		return err
	}
	t := task{id: uuid.New(), file: uuid.UUID(f.ID), sha256: f.SHA256[:]}
	status := StatusProcessing
	if active >= MaxActive {
		status = StatusWaiting
	}
	_, err = tx.Exec(ctx,
		`INSERT INTO tarefas (id, arquivo_id, dono, sha256, triangulos, status) VALUES ($1, $2, $3, $4, $5, $6)`,
		t.id, t.file, owner, t.sha256, int64(model.Triangles()), status)
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
	var (
		st     files.TaskStatus
		volume *float64
		box    []float64
	)
	err := q.db.QueryRow(ctx,
		`SELECT status, volume_mm3, caixa_mm FROM tarefas WHERE arquivo_id = $1`, uuid.UUID(id)).
		Scan(&st.State, &volume, &box)
	if err != nil {
		return files.TaskStatus{}, fmt.Errorf("tarefa do arquivo %s: %w", id, err)
	}
	if volume != nil && len(box) == 3 {
		st.VolumeMM3 = volume
		st.BoxMM = &[3]float64{box[0], box[1], box[2]}
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

	var owner uuid.UUID
	err = tx.QueryRow(ctx, `SELECT dono FROM tarefas WHERE id = $1`, m.TaskID).Scan(&owner)
	if errors.Is(err, pgx.ErrNoRows) {
		q.log.Warn("resultado de tarefa desconhecida", "tarefa", m.TaskID)
		return nil
	}
	if err != nil {
		return err
	}
	if err := lockOwner(ctx, tx, owner); err != nil {
		return err
	}

	var volume *float64
	var box []float64
	if m.Status == StatusDone {
		volume, box = &m.VolumeMM3, m.BoxMM[:]
	}
	tag, err := tx.Exec(ctx,
		`UPDATE tarefas SET status = $2, volume_mm3 = $3, caixa_mm = $4, atualizado_em = now()
		 WHERE id = $1 AND status = $5`,
		m.TaskID, m.Status, volume, box, StatusProcessing)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return tx.Commit(ctx)
	}

	var next task
	err = tx.QueryRow(ctx,
		`SELECT id, arquivo_id, sha256 FROM tarefas WHERE dono = $1 AND status = $2 ORDER BY criado_em LIMIT 1`,
		owner, StatusWaiting).Scan(&next.id, &next.file, &next.sha256)
	if errors.Is(err, pgx.ErrNoRows) {
		return tx.Commit(ctx)
	}
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx,
		`UPDATE tarefas SET status = $2, atualizado_em = now() WHERE id = $1`, next.id, StatusProcessing)
	if err != nil {
		return err
	}
	if err := q.publish(ctx, next); err != nil {
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

// lockOwner serializa as decisões de cota de uma conta entre todas as
// réplicas da API, até o fim da transação.
func lockOwner(ctx context.Context, tx pgx.Tx, owner uuid.UUID) error {
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1::text, 0))`, owner.String())
	return err
}
