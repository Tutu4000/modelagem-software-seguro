package server

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"nuggetmachine/internal/auth"
	"nuggetmachine/internal/problem"
)

// Route is one registered route and the roles it accepts.
type Route struct {
	Method string
	Path   string
	Roles  []auth.Role
}

// Router is a gin engine that only gets routes through Routes, so every route
// declares its roles (E6.2). The tests walk Engine.Routes() and fail on any
// route missing from Registered.
type Router struct {
	Engine     *gin.Engine
	Registered []Route
}

// NewRouter sets what both routers share: the proxies allowed to report the
// client IP, the request log (nil turns it off) and a recovery that answers a
// panic with a Problem Details 500.
func NewRouter(trustedProxies []string, log *slog.Logger) (*Router, error) {
	e := gin.New()
	if err := e.SetTrustedProxies(trustedProxies); err != nil {
		return nil, fmt.Errorf("trusted proxies: %w", err)
	}
	if log != nil {
		e.Use(RequestLog(log))
	}
	e.Use(gin.CustomRecovery(func(c *gin.Context, recovered any) {
		problem.Abort(c, fmt.Errorf("panic: %v", recovered))
	}))
	return &Router{Engine: e}, nil
}

func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) { r.Engine.ServeHTTP(w, req) }

// Routes registers the routes of one group. Every route gets
// auth.Require(roles...) first, then the shared middlewares, then its own
// handlers.
type Routes struct {
	router   *Router
	group    *gin.RouterGroup
	verifier *auth.Verifier
	shared   []gin.HandlerFunc
}

func (r *Router) Group(path string, v *auth.Verifier, shared ...gin.HandlerFunc) *Routes {
	return &Routes{router: r, group: r.Engine.Group(path), verifier: v, shared: shared}
}

// Handle refuses a route without roles at startup: a route nobody declared a
// role for must not exist.
func (r *Routes) Handle(method, path string, roles []auth.Role, handlers ...gin.HandlerFunc) {
	if len(roles) == 0 {
		panic("route " + method + " " + r.group.BasePath() + path + " has no roles")
	}
	chain := append([]gin.HandlerFunc{auth.Require(r.verifier, roles...)}, r.shared...)
	r.group.Handle(method, path, append(chain, handlers...)...)
	r.router.Registered = append(r.router.Registered, Route{
		Method: method,
		Path:   joinPath(r.group.BasePath(), path),
		Roles:  roles,
	})
}

func joinPath(base, path string) string {
	if path == "" {
		return base
	}
	if base == "/" {
		return path
	}
	return base + path
}
