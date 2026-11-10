// Command retention applies the retention rules (E8.5): it deletes the
// designs and the delivery addresses the platform no longer needs. It runs
// every RETENTION_INTERVAL, or once with -once (tests and `just retention`).
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"nuggetmachine/internal/audit"
	"nuggetmachine/internal/config"
	"nuggetmachine/internal/pg"
	"nuggetmachine/internal/retention"
	"nuggetmachine/internal/storage"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	once := flag.Bool("once", false, "make one pass and exit")
	flag.Parse()
	if err := run(log, *once); err != nil {
		log.Error("retention stopped", "error", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger, once bool) error {
	var env config.Env
	mainHost, mainName := env.Required("DB_HOST"), env.String("DB_NAME", "nugget")
	auditHost, auditName := env.Required("AUDIT_DB_HOST"), env.String("AUDIT_DB_NAME", "audit")
	dbPassword := env.Secret("DB_RETENTION_PASSWORD")
	auditPassword := env.Secret("DB_RETENTION_AUDIT_PASSWORD")
	s3cfg := storage.S3Config{
		Bucket:    env.String("S3_BUCKET", "files"),
		Region:    env.String("S3_REGION", "garage"),
		Endpoint:  env.Required("S3_ENDPOINT"),
		AccessKey: string(env.Secret("S3_ACCESS_KEY_ID")),
		SecretKey: string(env.Secret("S3_SECRET_ACCESS_KEY")),
	}
	durations := map[string]*time.Duration{}
	for name, def := range map[string]string{
		"RETENTION_AFTER_DELIVERY": "2160h",
		"RETENTION_UNORDERED":      "720h",
		"RETENTION_INTERVAL":       "1h",
	} {
		d, err := time.ParseDuration(env.String(name, def))
		if err != nil || d <= 0 {
			return fmt.Errorf("%s: want a positive duration", name)
		}
		durations[name] = &d
	}
	if err := env.Err(); err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := pg.Open(ctx, mainHost, mainName, "retention", dbPassword)
	if err != nil {
		return err
	}
	defer db.Close()
	auditPool, err := pg.Open(ctx, auditHost, auditName, "retention_audit", auditPassword)
	if err != nil {
		return err
	}
	defer auditPool.Close()

	job := retention.Job{
		Rules: retention.Rules{
			AfterDelivery: *durations["RETENTION_AFTER_DELIVERY"],
			Unordered:     *durations["RETENTION_UNORDERED"],
		},
		DB:      db,
		Objects: storage.NewS3(s3cfg),
		Audit:   audit.NewWithStore(os.Stdout, audit.NewPostgres(auditPool)),
		Log:     log,
		Now:     time.Now,
	}
	ticker := time.NewTicker(*durations["RETENTION_INTERVAL"])
	defer ticker.Stop()
	for {
		res, err := job.Run(ctx)
		if err != nil {
			log.Error("retention pass", "error", err)
		}
		log.Info("retention pass done", "files", res.Files, "addresses", res.Addresses)
		if once {
			return err
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}
