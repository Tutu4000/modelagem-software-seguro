package audit

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Postgres grava no banco de auditoria separado, onde o papel da aplicação só
// tem INSERT (E7.2).
type Postgres struct {
	db *pgxpool.Pool
}

func NewPostgres(db *pgxpool.Pool) *Postgres { return &Postgres{db: db} }

func (p *Postgres) Insert(ctx context.Context, e Event) error {
	_, err := p.db.Exec(ctx,
		`INSERT INTO eventos (horario, evento, autor, operacao, alvo, resultado, motivo, origem)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		e.Time, e.Name, e.Actor, e.Action, e.Target, e.Result, e.Reason, e.Origin)
	return err
}
