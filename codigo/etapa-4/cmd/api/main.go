// Command api is the public API: customer routes only (backlog section 1.1).
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"nuggetmachine/internal/audit"
	"nuggetmachine/internal/auth"
	"nuggetmachine/internal/config"
	"nuggetmachine/internal/files"
	"nuggetmachine/internal/orders"
	"nuggetmachine/internal/partners"
	"nuggetmachine/internal/pg"
	"nuggetmachine/internal/pricing"
	"nuggetmachine/internal/queue"
	"nuggetmachine/internal/server/customer"
	"nuggetmachine/internal/session"
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
	internalAddr := env.String("INTERNAL_ADDR", ":8090")
	trusted := env.List("TRUSTED_PROXIES")
	issuer := env.Required("OIDC_ISSUER")
	jwksURL := env.Required("OIDC_JWKS_URL")
	audience := env.String("OIDC_AUDIENCE", "api")
	mainHost, mainName := env.Required("DB_HOST"), env.String("DB_NAME", "nugget")
	auditHost, auditName := env.Required("AUDIT_DB_HOST"), env.String("AUDIT_DB_NAME", "audit")
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
	ordersPassword := env.Secret("DB_ORDERS_PASSWORD")
	auditPassword := env.Secret("DB_AUDIT_PASSWORD")
	buildVolume, err := pricing.ParseBuildVolume(env.String("BUILD_VOLUME_MM", "250,250,250"))
	if err != nil {
		return err
	}
	amqpHost := env.String("AMQP_HOST", "rabbitmq:5672")
	amqpUser := env.String("AMQP_USER", "api")
	amqpPassword := env.Secret("AMQP_PASSWORD")
	paymentsURL, carrierURL := env.Required("PAYMENTS_URL"), env.Required("CARRIER_URL")
	paymentsKey, carrierKey := env.Secret("PAYMENTS_API_KEY"), env.Secret("CARRIER_API_KEY")
	paymentsSecret, shippingSecret := env.Secret("PAYMENTS_WEBHOOK_SECRET"), env.Secret("SHIPPING_WEBHOOK_SECRET")
	valkeyAddr := env.Required("VALKEY_ADDR")
	valkeyUser := env.String("VALKEY_USER", "api")
	valkeyPassword := env.Secret("VALKEY_PASSWORD")
	ratePerMinute, err := strconv.Atoi(env.String("RATE_LIMIT_PER_MINUTE", "60"))
	if err != nil || ratePerMinute < 1 {
		return fmt.Errorf("RATE_LIMIT_PER_MINUTE: want a positive integer")
	}
	if err := env.Err(); err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	keys, err := auth.JWKS(ctx, jwksURL)
	if err != nil {
		return err
	}

	filesPool, err := pg.Open(ctx, mainHost, mainName, "files", filesPassword)
	if err != nil {
		return err
	}
	defer filesPool.Close()
	queuePool, err := pg.Open(ctx, mainHost, mainName, "queue", queuePassword)
	if err != nil {
		return err
	}
	defer queuePool.Close()
	ordersPool, err := pg.Open(ctx, mainHost, mainName, "orders", ordersPassword)
	if err != nil {
		return err
	}
	defer ordersPool.Close()
	auditPool, err := pg.Open(ctx, auditHost, auditName, "audit_app", auditPassword)
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

	sessions, err := session.NewValkey(valkeyAddr, valkeyUser, valkeyPassword)
	if err != nil {
		return fmt.Errorf("Valkey at %s: %w", valkeyAddr, err)
	}
	defer sessions.Close()

	store := storage.NewS3(s3cfg)
	auditLog := audit.NewWithStore(os.Stdout, audit.NewPostgres(auditPool))
	tasks := queue.NewRabbit(queuePool, broker, store, auditLog, log)
	verifier := auth.NewVerifier(auth.VerifierConfig{Keys: keys, Issuer: issuer, Audience: audience, Now: time.Now})

	gin.SetMode(gin.ReleaseMode)
	router, err := customer.New(customer.Deps{
		Verifier:              verifier,
		Files:                 files.NewPostgresRepo(filesPool),
		Store:                 store,
		Queue:                 tasks,
		Tasks:                 tasks,
		Audit:                 auditLog,
		Sessions:              sessions,
		Limiter:               sessions,
		RatePerMinute:         ratePerMinute,
		Orders:                orders.NewStore(ordersPool),
		BuildVolume:           buildVolume,
		Now:                   time.Now,
		Gateway:               partners.NewClient(paymentsURL, paymentsKey),
		Carrier:               partners.NewClient(carrierURL, carrierKey),
		PaymentsWebhookSecret: paymentsSecret,
		ShippingWebhookSecret: shippingSecret,
		Log:                   log,
		TrustedProxies:        trusted,
	})
	if err != nil {
		return err
	}

	// The internal listener is reachable only on the internal network: nginx
	// never forwards to it. Keycloak calls it for the back-channel logout.
	internal := http.NewServeMux()
	internal.Handle("POST /oidc/backchannel-logout", session.LogoutHandler{
		Verifier: verifier,
		Clients:  []string{"web", "panel-web", "e2e"},
		Store:    sessions,
		Audit:    auditLog,
		Log:      log,
	})
	internalSrv := &http.Server{
		Addr:              internalAddr,
		Handler:           internal,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		MaxHeaderBytes:    16 << 10,
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
	errc := make(chan error, 3)
	go func() { errc <- srv.ListenAndServe() }()
	go func() { errc <- internalSrv.ListenAndServe() }()
	go func() { errc <- tasks.ConsumeResults(ctx) }()
	log.Info("API up", "addr", addr, "internal_addr", internalAddr)

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
	if err := internalSrv.Shutdown(shutdown); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
