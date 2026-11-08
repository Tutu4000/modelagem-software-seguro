// Package retention deletes what the platform no longer needs (E8.5, LGPD):
// the design some time after the delivery, the design of a file that never
// became a live order, and the delivery address once the order is closed. It
// runs as its own binary, with its own database role and storage key.
package retention

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"nuggetmachine/internal/audit"
	"nuggetmachine/internal/retention/retentiondb"
)

type Rules struct {
	// AfterDelivery: the design and the address are kept this long after the
	// order is delivered (90 days).
	AfterDelivery time.Duration
	// Unordered: a design without a live order is kept this long after the
	// upload (30 days).
	Unordered time.Duration
}

type Deleter interface {
	Delete(ctx context.Context, key string) error
}

type Job struct {
	Rules   Rules
	DB      *pgxpool.Pool
	Objects Deleter
	Audit   *audit.Logger
	Log     *slog.Logger
	Now     func() time.Time
}

// Result counts what one pass deleted.
type Result struct {
	Files     int
	Addresses int
}

// Run makes one pass. A file is marked purged only after the object is gone,
// so a failure leaves it to the next pass.
func (j Job) Run(ctx context.Context) (Result, error) {
	q := retentiondb.New(j.DB)
	now := j.Now()
	var res Result
	ids, err := q.PurgeableFiles(ctx, retentiondb.PurgeableFilesParams{
		DeliveredBefore: now.Add(-j.Rules.AfterDelivery),
		UnorderedBefore: now.Add(-j.Rules.Unordered),
	})
	if err != nil {
		return res, err
	}
	var errs []error
	for _, id := range ids {
		if err := j.Objects.Delete(ctx, id.String()); err != nil {
			errs = append(errs, err)
			continue
		}
		if err := q.MarkFilePurged(ctx, id); err != nil {
			errs = append(errs, err)
			continue
		}
		j.record("file.purged", id.String())
		res.Files++
	}

	orders, err := q.AddressesToPurge(ctx, now.Add(-j.Rules.AfterDelivery))
	if err != nil {
		return res, errors.Join(append(errs, err)...)
	}
	for _, id := range orders {
		if err := q.PurgeAddress(ctx, id); err != nil {
			errs = append(errs, err)
			continue
		}
		j.record("address.purged", id.String())
		res.Addresses++
	}
	return res, errors.Join(errs...)
}

func (j Job) record(name, target string) {
	if err := j.Audit.Record(audit.Event{
		Name: name, Actor: "system:retention", Action: name, Target: target, Result: "ok", Origin: "retention",
	}); err != nil {
		j.Log.Error("audit failed", "event", name, "target", target, "error", err)
	}
}
