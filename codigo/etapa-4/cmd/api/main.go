// Comando api é a API pública: só as rotas do cliente (seção 1.1 do backlog).
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
	"nuggetmachine/internal/server/cliente"
	"nuggetmachine/internal/storage"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(log); err != nil {
		log.Error("a API parou", "error", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	var env config.Env
	addr := env.String("API_ADDR", ":8080")
	trusted := env.List("TRUSTED_PROXIES")
	pubPEM := env.Secret("AUTH_PUBLIC_KEY")
	mainDB := dbConfig{host: env.Required("DB_HOST"), name: env.String("DB_NAME", "nugget")}
	auditDB := dbConfig{host: env.Required("AUDIT_DB_HOST"), name: env.String("AUDIT_DB_NAME", "auditoria")}
	s3cfg := storage.S3Config{
		Bucket:         env.String("S3_BUCKET", "arquivos"),
		Region:         env.String("S3_REGION", "garage"),
		Endpoint:       env.Required("S3_ENDPOINT"),
		PublicEndpoint: env.Required("S3_PUBLIC_ENDPOINT"),
		AccessKey:      string(env.Secret("S3_ACCESS_KEY_ID")),
		SecretKey:      string(env.Secret("S3_SECRET_ACCESS_KEY")),
	}
	filesPassword := env.Secret("DB_ARQUIVOS_PASSWORD")
	auditPassword := env.Secret("DB_AUDITORIA_PASSWORD")
	if err := env.Err(); err != nil {
		return err
	}

	pub, err := jwt.ParseEdPublicKeyFromPEM(pubPEM)
	if err != nil {
		return fmt.Errorf("AUTH_PUBLIC_KEY: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	filesPool, err := mainDB.open(ctx, "arquivos", filesPassword)
	if err != nil {
		return err
	}
	defer filesPool.Close()
	auditPool, err := auditDB.open(ctx, "auditoria_app", auditPassword)
	if err != nil {
		return err
	}
	defer auditPool.Close()

	store := storage.NewS3(s3cfg)
	tasks := queue.NewMemory()

	gin.SetMode(gin.ReleaseMode)
	router, err := cliente.New(cliente.Deps{
		Verifier:       auth.NewVerifier(pub.(ed25519.PublicKey), time.Now),
		Files:          files.NewPostgresRepo(filesPool),
		Store:          store,
		Queue:          tasks,
		Tasks:          tasks,
		Audit:          audit.NewWithStore(os.Stdout, audit.NewPostgres(auditPool)),
		Log:            log,
		TrustedProxies: trusted,
	})
	if err != nil {
		return err
	}

	// Os tempos cobrem um upload de 100 MB numa conexão lenta, sem deixar
	// conexões presas para sempre.
	srv := &http.Server{
		Addr:              addr,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       2 * time.Minute,
		WriteTimeout:      2 * time.Minute,
		IdleTimeout:       time.Minute,
		MaxHeaderBytes:    16 << 10,
	}
	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServe() }()
	log.Info("API no ar", "addr", addr)

	select {
	case err := <-errc:
		return err
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

// open conecta com o papel do módulo. A rede interna não tem TLS no ambiente
// local (seção 5 do backlog).
func (d dbConfig) open(ctx context.Context, user string, password []byte) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(fmt.Sprintf("host=%s dbname=%s user=%s sslmode=disable", d.host, d.name, user))
	if err != nil {
		return nil, err
	}
	cfg.ConnConfig.Password = string(password)
	cfg.MaxConns = 10
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("banco %s: %w", d.name, err)
	}
	pingCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("banco %s com o papel %s: %w", d.name, user, err)
	}
	return pool, nil
}
