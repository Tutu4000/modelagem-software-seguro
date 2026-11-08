package session

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"nuggetmachine/internal/audit"
	"nuggetmachine/internal/auth"
	"nuggetmachine/internal/problem"
)

// storeTimeout keeps a slow Valkey from holding requests: past it, the
// request fails closed with 503.
const storeTimeout = time.Second

// Active refuses tokens whose session or user was revoked. It runs after
// auth.Require.
func Active(s Store, a *audit.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		u := auth.MustUser(c)
		ctx, cancel := context.WithTimeout(c.Request.Context(), storeTimeout)
		defer cancel()
		revoked, err := s.Revoked(ctx, u.SessionID(), u.ID().String(), u.IssuedAt())
		if err != nil {
			problem.Abort(c, problem.New(problem.Unavailable, fmt.Errorf("session store: %w", err)))
			return
		}
		if revoked {
			record(c, a, audit.Event{
				Name:   "auth.revoked_token_used",
				Actor:  u.ID().String(),
				Action: c.Request.Method + " " + c.FullPath(),
				Target: u.SessionID(),
				Result: "denied",
			})
			problem.Abort(c, problem.New(problem.Unauthenticated, errors.New("revoked session")))
			return
		}
		c.Next()
	}
}

// RateLimit caps the requests of each user across all replicas (E5.2), on top
// of the per-IP limit of nginx.
func RateLimit(l Limiter, a *audit.Logger, perMinute int) gin.HandlerFunc {
	return func(c *gin.Context) {
		u := auth.MustUser(c)
		ctx, cancel := context.WithTimeout(c.Request.Context(), storeTimeout)
		defer cancel()
		d, err := l.Allow(ctx, u.ID().String(), perMinute, time.Minute)
		if err != nil {
			problem.Abort(c, problem.New(problem.Unavailable, fmt.Errorf("rate limiter: %w", err)))
			return
		}
		if !d.Allowed {
			if d.FirstRejection {
				record(c, a, audit.Event{
					Name:   "rate.limited",
					Actor:  u.ID().String(),
					Action: c.Request.Method + " " + c.FullPath(),
					Result: "denied",
					Reason: fmt.Sprintf("more than %d requests per minute", perMinute),
				})
			}
			c.Header("Retry-After", strconv.Itoa(int(math.Ceil(d.RetryAfter.Seconds()))))
			problem.Abort(c, problem.New(problem.RateLimited, fmt.Errorf("user above %d requests per minute", perMinute)))
			return
		}
		c.Next()
	}
}

func record(c *gin.Context, a *audit.Logger, e audit.Event) {
	e.Origin = c.ClientIP()
	if err := a.Record(e); err != nil {
		_ = c.Error(err)
	}
}
