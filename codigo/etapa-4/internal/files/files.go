// Package files é o módulo Arquivos.
package files

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"nuggetmachine/internal/audit"
	"nuggetmachine/internal/auth"
	"nuggetmachine/internal/stl"
)

var errNotImplemented = errors.New("não implementado")

// FileID também é a chave do objeto no armazenamento.
type FileID uuid.UUID

func NewFileID() FileID { return FileID(uuid.New()) }

func (id FileID) String() string { return uuid.UUID(id).String() }

type File struct {
	ID     FileID
	Owner  auth.UserID
	Size   int64
	SHA256 [32]byte
}

type Repo interface {
	Save(ctx context.Context, f File) error
}

type ObjectStore interface {
	Put(ctx context.Context, key string, data []byte) error
}

type Enqueuer interface {
	Enqueue(ctx context.Context, f File, model stl.Validated) error
}

type MemoryRepo struct{}

func NewMemoryRepo() *MemoryRepo { return &MemoryRepo{} }

func (r *MemoryRepo) Save(ctx context.Context, f File) error { return errNotImplemented }

func (r *MemoryRepo) Count() int { return 0 }

type Handler struct {
	Repo  Repo
	Store ObjectStore
	Queue Enqueuer
	Audit *audit.Logger
}

func (h *Handler) Download(c *gin.Context) { c.AbortWithStatus(http.StatusNotImplemented) }

func (h *Handler) Upload(c *gin.Context) { c.AbortWithStatus(http.StatusNotImplemented) }
