// Package pg opens the Postgres pools, always with the role of the module that
// uses them (E3.1).
package pg

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Open connects to one database with one role. The internal network has no
// TLS in the local environment (backlog section 5).
func Open(ctx context.Context, host, database, user string, password []byte) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(fmt.Sprintf("host=%s dbname=%s user=%s sslmode=disable", host, database, user))
	if err != nil {
		return nil, err
	}
	cfg.ConnConfig.Password = string(password)
	cfg.MaxConns = 10
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("database %s: %w", database, err)
	}
	pingCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("database %s with role %s: %w", database, user, err)
	}
	return pool, nil
}
