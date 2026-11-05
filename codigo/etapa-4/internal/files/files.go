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

type MemoryRepo struct{}

func NewMemoryRepo() *MemoryRepo { return &MemoryRepo{} }

func (r *MemoryRepo) Save(ctx context.Context, f File) error { return errNotImplemented }

type Handler struct {
	Repo  Repo
	Store ObjectStore
	Audit *audit.Logger
}

func (h *Handler) Download(c *gin.Context) { c.AbortWithStatus(http.StatusNotImplemented) }
