// Command api is the public API: customer routes only (backlog section 1.1).
package main

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"nuggetmachine/internal/audit"
	"nuggetmachine/internal/auth"
	"nuggetmachine/internal/config"
	"nuggetmachine/internal/files"
	"nuggetmachine/internal/queue"
	"nuggetmachine/internal/server/customer"
	"nuggetmachine/internal/storage"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(log); err != nil {
		log.Error("the API stopped", "error", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	var env config.Env
	addr := env.String("API_ADDR", ":8080")
	trusted := env.List("TRUSTED_PROXIES")
	pubPEM := env.Secret("AUTH_PUBLIC_KEY")
	mainDB := dbConfig{host: env.Required("DB_HOST"), name: env.String("DB_NAME", "nugget")}
	auditDB := dbConfig{host: env.Required("AUDIT_DB_HOST"), name: env.String("AUDIT_DB_NAME", "audit")}
	s3cfg := storage.S3Config{
		Bucket:         env.String("S3_BUCKET", "files"),
		Region:         env.String("S3_REGION", "garage"),
		Endpoint:       env.Required("S3_ENDPOINT"),
		PublicEndpoint: env.Required("S3_PUBLIC_ENDPOINT"),
		AccessKey:      string(env.Secret("S3_ACCESS_KEY_ID")),
		SecretKey:      string(env.Secret("S3_SECRET_ACCESS_KEY")),
	}
	filesPassword := env.Secret("DB_FILES_PASSWORD")
	queuePassword := env.Secret("DB_QUEUE_PASSWORD")
	auditPassword := env.Secret("DB_AUDIT_PASSWORD")
	amqpHost := env.String("AMQP_HOST", "rabbitmq:5672")
	amqpUser := env.String("AMQP_USER", "api")
	amqpPassword := env.Secret("AMQP_PASSWORD")
	if err := env.Err(); err != nil {
		return err
	}

	pub, err := jwt.ParseEdPublicKeyFromPEM(pubPEM)
	if err != nil {
		return fmt.Errorf("AUTH_PUBLIC_KEY: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	filesPool, err := mainDB.open(ctx, "files", filesPassword)
	if err != nil {
		return err
	}
	defer filesPool.Close()
	queuePool, err := mainDB.open(ctx, "queue", queuePassword)
	if err != nil {
		return err
	}
	defer queuePool.Close()
	auditPool, err := auditDB.open(ctx, "audit_app", auditPassword)
	if err != nil {
		return err
	}
	defer auditPool.Close()

	broker, err := queue.Dial(ctx, amqpHost, amqpUser, amqpPassword)
	if err != nil {
		return err
	}
	defer broker.Close()
	closed := broker.Closed()

	store := storage.NewS3(s3cfg)
	auditLog := audit.NewWithStore(os.Stdout, audit.NewPostgres(auditPool))
	tasks := queue.NewRabbit(queuePool, broker, store, auditLog, log)

	gin.SetMode(gin.ReleaseMode)
	router, err := customer.New(customer.Deps{
		Verifier:       auth.NewVerifier(pub.(ed25519.PublicKey), time.Now),
		Files:          files.NewPostgresRepo(filesPool),
		Store:          store,
		Queue:          tasks,
		Tasks:          tasks,
		Audit:          auditLog,
		Log:            log,
		TrustedProxies: trusted,
	})
	if err != nil {
		return err
	}

	// The timeouts cover a 100 MB upload over a slow connection without leaving
	// connections stuck forever.
	srv := &http.Server{
		Addr:              addr,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       2 * time.Minute,
		WriteTimeout:      2 * time.Minute,
		IdleTimeout:       time.Minute,
		MaxHeaderBytes:    16 << 10,
	}
	errc := make(chan error, 2)
	go func() { errc <- srv.ListenAndServe() }()
	go func() { errc <- tasks.ConsumeResults(ctx) }()
	log.Info("API up", "addr", addr)

	// Without RabbitMQ the API exits, and Docker restarts it once it is back.
	select {
	case err := <-errc:
		return err
	case err := <-closed:
		return fmt.Errorf("connection to RabbitMQ dropped: %v", err)
	case <-ctx.Done():
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdown); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

type dbConfig struct {
	host, name string
}

// open connects with the module's role. The internal network has no TLS in
// the local environment (backlog section 5).
func (d dbConfig) open(ctx context.Context, user string, password []byte) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(fmt.Sprintf("host=%s dbname=%s user=%s sslmode=disable", d.host, d.name, user))
	if err != nil {
		return nil, err
	}
	cfg.ConnConfig.Password = string(password)
	cfg.MaxConns = 10
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("database %s: %w", d.name, err)
	}
	pingCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("database %s with role %s: %w", d.name, user, err)
	}
	return pool, nil
}
