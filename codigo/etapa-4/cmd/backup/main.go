// Command backup copies the databases and the files to the backup bucket,
// encrypted with age, once a day, or once with -once (`just backup`). With
// -restore-test it restores the latest backup into a temporary server and
// checks it (`just restore-test`, E3.6).
package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"filippo.io/age"

	"nuggetmachine/internal/backup"
	"nuggetmachine/internal/config"
	"nuggetmachine/internal/storage"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	once := flag.Bool("once", false, "make one backup and exit")
	restoreTest := flag.Bool("restore-test", false, "restore the latest backup into the temporary server and check it")
	flag.Parse()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	var err error
	if *restoreTest {
		err = runRestore(ctx, log)
	} else {
		err = runBackup(ctx, log, *once)
	}
	if err != nil {
		log.Error("backup stopped", "error", err)
		os.Exit(1)
	}
}

func bucket(env *config.Env, name string) *storage.S3 {
	return storage.NewS3(storage.S3Config{
		Bucket:    name,
		Region:    env.String("S3_REGION", "garage"),
		Endpoint:  env.String("S3_ENDPOINT", "http://garage:3900"),
		AccessKey: string(env.Secret("S3_ACCESS_KEY_ID")),
		SecretKey: string(env.Secret("S3_SECRET_ACCESS_KEY")),
	})
}

func runBackup(ctx context.Context, log *slog.Logger, once bool) error {
	var env config.Env
	b := backup.Backup{
		Main: backup.Database{Host: env.Required("DB_HOST"), Name: env.String("DB_NAME", "nugget"),
			User: "backup", Password: env.Secret("DB_BACKUP_PASSWORD")},
		Audit: backup.Database{Host: env.Required("AUDIT_DB_HOST"), Name: env.String("AUDIT_DB_NAME", "audit"),
			User: "backup", Password: env.Secret("DB_AUDIT_BACKUP_PASSWORD")},
		Files:   bucket(&env, env.String("FILES_BUCKET", "files")),
		Backups: bucket(&env, env.String("BACKUP_BUCKET", "backups")),
		Log:     log,
		Now:     time.Now,
	}
	recipient := env.Secret("AGE_RECIPIENT")
	interval, err := time.ParseDuration(env.String("BACKUP_INTERVAL", "24h"))
	if err != nil || interval <= 0 {
		return fmt.Errorf("BACKUP_INTERVAL: want a positive duration")
	}
	if err := env.Err(); err != nil {
		return err
	}
	if b.Recipient, err = age.ParseX25519Recipient(string(bytes.TrimSpace(recipient))); err != nil {
		return fmt.Errorf("AGE_RECIPIENT: %w", err)
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		prefix, m, err := b.Run(ctx)
		if err != nil {
			log.Error("backup failed", "error", err)
		} else {
			log.Info("backup done", "prefix", prefix, "counts", m.Counts, "objects", m.Objects)
		}
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

func runRestore(ctx context.Context, log *slog.Logger) error {
	var env config.Env
	r := backup.Restore{
		Backups: bucket(&env, env.String("BACKUP_BUCKET", "backups")),
		Host:    env.Required("RESTORE_DB_HOST"),
		Log:     log,
	}
	identity := env.Secret("AGE_IDENTITY")
	if err := env.Err(); err != nil {
		return err
	}
	id, err := age.ParseX25519Identity(string(bytes.TrimSpace(identity)))
	if err != nil {
		return fmt.Errorf("AGE_IDENTITY: %w", err)
	}
	r.Identity = id
	res, err := r.Run(ctx)
	if err != nil {
		return err
	}
	log.Info("restore-test ok", "backup", res.Prefix, "counts", res.Counts, "objects", res.Objects, "seconds", res.Duration.Seconds())
	return nil
}
