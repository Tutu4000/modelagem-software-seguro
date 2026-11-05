// Package cliente é o roteador da API pública. As rotas do painel não existem
// aqui.
package cliente

import (
	"github.com/gin-gonic/gin"

	"nuggetmachine/internal/audit"
	"nuggetmachine/internal/auth"
	"nuggetmachine/internal/files"
)

type Deps struct {
	Verifier *auth.Verifier
	Files    files.Repo
	Store    files.ObjectStore
	Audit    *audit.Logger
}

func New(d Deps) *gin.Engine {
	r := gin.New()
	h := &files.Handler{Repo: d.Files, Store: d.Store, Audit: d.Audit}

	api := r.Group("/api/v1", auth.RequireUser(d.Verifier))
	api.GET("/arquivos/:id/download", h.Download)
	return r
}
