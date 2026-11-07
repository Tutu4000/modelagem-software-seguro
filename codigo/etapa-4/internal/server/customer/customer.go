// Package customer is the public API router. The panel routes do not exist
// here.
package customer

import (
	"fmt"
	"log/slog"

	"github.com/gin-gonic/gin"

	"nuggetmachine/internal/audit"
	"nuggetmachine/internal/auth"
	"nuggetmachine/internal/files"
	"nuggetmachine/internal/problem"
	"nuggetmachine/internal/server"
)

type Deps struct {
	Verifier *auth.Verifier
	Files    files.Repo
	Store    files.ObjectStore
	Queue    files.Enqueuer
	Tasks    files.TaskReader
	Audit    *audit.Logger
	Log      *slog.Logger // nil turns off the request log
	// TrustedProxies are the IPs allowed to report the origin through
	// X-Forwarded-For. When empty, the origin of audit events comes from the
	// connection, not from a forged header. In the compose, only nginx (E2.3).
	TrustedProxies []string
}

func New(d Deps) (*gin.Engine, error) {
	r := gin.New()
	if err := r.SetTrustedProxies(d.TrustedProxies); err != nil {
		return nil, fmt.Errorf("trusted proxies: %w", err)
	}
	if d.Log != nil {
		r.Use(server.RequestLog(d.Log))
	}
	r.Use(gin.CustomRecovery(func(c *gin.Context, recovered any) {
		problem.Abort(c, fmt.Errorf("panic: %v", recovered))
	}))

	h := &files.Handler{Repo: d.Files, Store: d.Store, Queue: d.Queue, Tasks: d.Tasks, Audit: d.Audit}
	api := r.Group("/api/v1", auth.RequireUser(d.Verifier))
	api.GET("/files/:id", h.Status)
	api.GET("/files/:id/download", h.Download)
	api.POST("/files", h.Upload)
	return r, nil
}
