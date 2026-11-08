// Command panel is the internal panel API: staff routes only, behind MFA, on
// an address of its own (D02, backlog section 1.1).
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
	"nuggetmachine/internal/idpadmin"
	"nuggetmachine/internal/notify"
	"nuggetmachine/internal/orders"
	"nuggetmachine/internal/partners"
	"nuggetmachine/internal/pg"
	"nuggetmachine/internal/queue"
	"nuggetmachine/internal/server/panel"
	"nuggetmachine/internal/session"
	"nuggetmachine/internal/storage"
	"nuggetmachine/internal/support"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(log); err != nil {
		log.Error("the panel stopped", "error", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	var env config.Env
	addr := env.String("PANEL_ADDR", ":8080")
	internalAddr := env.String("INTERNAL_ADDR", ":8090")
	trusted := env.List("TRUSTED_PROXIES")
	issuer := env.Required("OIDC_ISSUER")
	jwksURL := env.Required("OIDC_JWKS_URL")
	audience := env.String("OIDC_AUDIENCE", "panel")
	auditHost, auditName := env.Required("AUDIT_DB_HOST"), env.String("AUDIT_DB_NAME", "audit")
	auditPassword := env.Secret("DB_PANEL_AUDIT_PASSWORD")
	mainHost, mainName := env.Required("DB_HOST"), env.String("DB_NAME", "nugget")
	ordersPassword := env.Secret("DB_PANEL_ORDERS_PASSWORD")
	supportPassword := env.Secret("DB_PANEL_SUPPORT_PASSWORD")
	amqpHost := env.String("AMQP_HOST", "rabbitmq:5672")
	amqpUser := env.String("AMQP_USER", "panel")
	amqpPassword := env.Secret("AMQP_PASSWORD")
	// A read-only storage key: the panel only opens designs (D02).
	s3cfg := storage.S3Config{
		Bucket:         env.String("S3_BUCKET", "files"),
		Region:         env.String("S3_REGION", "garage"),
		Endpoint:       env.Required("S3_ENDPOINT"),
		PublicEndpoint: env.Required("S3_PUBLIC_ENDPOINT"),
		AccessKey:      string(env.Secret("S3_ACCESS_KEY_ID")),
		SecretKey:      string(env.Secret("S3_SECRET_ACCESS_KEY")),
	}
	valkeyAddr := env.Required("VALKEY_ADDR")
	valkeyUser := env.String("VALKEY_USER", "panel")
	valkeyPassword := env.Secret("VALKEY_PASSWORD")
	carrierURL, carrierKey := env.Required("CARRIER_URL"), env.Secret("CARRIER_API_KEY")
	keycloakURL := env.Required("KEYCLOAK_ADMIN_URL")
	realm := env.String("KEYCLOAK_REALM", "nugget")
	adminSecret := env.Secret("KEYCLOAK_PANEL_ADMIN_SECRET")
	ratePerMinute, err := strconv.Atoi(env.String("RATE_LIMIT_PER_MINUTE", "60"))
	if err != nil || ratePerMinute < 1 {
		return fmt.Errorf("RATE_LIMIT_PER_MINUTE: want a positive integer")
	}
	stepUp, err := time.ParseDuration(env.String("PANEL_STEP_UP_MAX_AGE", "5m"))
	if err != nil || stepUp <= 0 {
		return fmt.Errorf("PANEL_STEP_UP_MAX_AGE: want a positive duration")
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
	auditPool, err := pg.Open(ctx, auditHost, auditName, "panel_audit", auditPassword)
	if err != nil {
		return err
	}
	defer auditPool.Close()
	ordersPool, err := pg.Open(ctx, mainHost, mainName, "panel_orders", ordersPassword)
	if err != nil {
		return err
	}
	defer ordersPool.Close()
	supportPool, err := pg.Open(ctx, mainHost, mainName, "panel_support", supportPassword)
	if err != nil {
		return err
	}
	defer supportPool.Close()
	// The panel only publishes notifications: its RabbitMQ user reaches
	// nothing else.
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

	auditLog := audit.NewWithStore(os.Stdout, audit.NewPostgres(auditPool))
	verifier := auth.NewVerifier(auth.VerifierConfig{Keys: keys, Issuer: issuer, Audience: audience, Now: time.Now})

	gin.SetMode(gin.ReleaseMode)
	router, err := panel.New(panel.Deps{
		Verifier:       verifier,
		Sessions:       sessions,
		Limiter:        sessions,
		RatePerMinute:  ratePerMinute,
		Audit:          auditLog,
		IdP:            idpadmin.New(keycloakURL, realm, "panel-admin", adminSecret),
		Orders:         orders.NewStore(ordersPool),
		Files:          storage.NewS3(s3cfg),
		Carrier:        partners.NewClient(carrierURL, carrierKey),
		Support:        support.NewStore(supportPool),
		Notify:         notify.NewRabbit(broker),
		StepUpMaxAge:   stepUp,
		Now:            time.Now,
		Log:            log,
		TrustedProxies: trusted,
	})
	if err != nil {
		return err
	}

	srv := &http.Server{
		Addr:              addr,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       time.Minute,
		MaxHeaderBytes:    16 << 10,
	}
	// Keycloak calls the back-channel logout of the panel-web sessions here.
	// nginx never forwards to this listener.
	internal := http.NewServeMux()
	internal.Handle("POST /oidc/backchannel-logout", session.LogoutHandler{
		Verifier: verifier,
		Clients:  []string{"panel-web"},
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

	errc := make(chan error, 2)
	go func() { errc <- srv.ListenAndServe() }()
	go func() { errc <- internalSrv.ListenAndServe() }()
	log.Info("panel up", "addr", addr, "internal_addr", internalAddr)

	select {
	case err := <-errc:
		return err
	case err := <-closed:
		return fmt.Errorf("connection to RabbitMQ dropped: %v", err)
	case <-ctx.Done():
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	for _, s := range []*http.Server{srv, internalSrv} {
		if err := s.Shutdown(shutdown); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	}
	return nil
}
