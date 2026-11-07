package audit

import (
	"context"

	"nuggetmachine/internal/audit/auditdb"
)

// Postgres grava no banco de auditoria separado, onde o papel da aplicação só
// tem INSERT (E7.2).
type Postgres struct {
	q *auditdb.Queries
}

func NewPostgres(db auditdb.DBTX) *Postgres { return &Postgres{q: auditdb.New(db)} }

func (p *Postgres) Insert(ctx context.Context, e Event) error {
	return p.q.InsertEvent(ctx, auditdb.InsertEventParams{
		Horario:   e.Time,
		Evento:    e.Name,
		Autor:     e.Actor,
		Operacao:  e.Action,
		Alvo:      e.Target,
		Resultado: e.Result,
		Motivo:    e.Reason,
		Origem:    e.Origin,
	})
}
