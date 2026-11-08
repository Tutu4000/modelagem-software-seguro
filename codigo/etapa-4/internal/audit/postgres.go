package audit

import (
	"context"

	"nuggetmachine/internal/audit/auditdb"
)

// Postgres writes to the separate audit database, where the application role
// can only INSERT (E7.2).
type Postgres struct {
	q *auditdb.Queries
}

func NewPostgres(db auditdb.DBTX) *Postgres { return &Postgres{q: auditdb.New(db)} }

func (p *Postgres) Insert(ctx context.Context, e Event) error {
	return p.q.InsertEvent(ctx, auditdb.InsertEventParams{
		OccurredAt: e.Time,
		Name:       e.Name,
		Actor:      e.Actor,
		Action:     e.Action,
		Target:     e.Target,
		Result:     e.Result,
		Reason:     e.Reason,
		Origin:     e.Origin,
	})
}
