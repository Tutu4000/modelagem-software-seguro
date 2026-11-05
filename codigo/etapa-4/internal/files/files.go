// Package files é o módulo Arquivos.
package files

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"nuggetmachine/internal/audit"
	"nuggetmachine/internal/auth"
	"nuggetmachine/internal/authz"
	"nuggetmachine/internal/problem"
	"nuggetmachine/internal/stl"
)

// ErrNotFound vale para arquivo inexistente e para arquivo alheio, para não
// revelar quais IDs existem.
var ErrNotFound = errors.New("arquivo não encontrado")

const downloadTTL = 5 * time.Minute // RS01

// FileID também é a chave do objeto no armazenamento.
type FileID uuid.UUID

func NewFileID() FileID { return FileID(uuid.New()) }

func ParseFileID(s string) (FileID, error) {
	id, err := uuid.Parse(s)
	return FileID(id), err
}

func (id FileID) String() string { return uuid.UUID(id).String() }

type File struct {
	ID     FileID
	Owner  auth.UserID
	Size   int64
	SHA256 [32]byte
}

// Repo não tem busca só por ID: toda leitura passa pela autorização central
// (D01).
type Repo interface {
	Save(ctx context.Context, f File) error
	FindForUser(ctx context.Context, u auth.User, id FileID) (File, error)
}

type ObjectStore interface {
	Put(ctx context.Context, key string, data []byte) error
	PresignGet(key string, ttl time.Duration) (string, error)
}

type Enqueuer interface {
	Enqueue(ctx context.Context, f File, model stl.Validated) error
}

type MemoryRepo struct {
	mu    sync.RWMutex
	files map[FileID]File
}

func NewMemoryRepo() *MemoryRepo { return &MemoryRepo{files: map[FileID]File{}} }

func (r *MemoryRepo) Save(_ context.Context, f File) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.files[f.ID] = f
	return nil
}

func (r *MemoryRepo) FindForUser(_ context.Context, u auth.User, id FileID) (File, error) {
	r.mu.RLock()
	f, ok := r.files[id]
	r.mu.RUnlock()
	if !ok || authz.CanReadFile(u, f.Owner) != nil {
		return File{}, ErrNotFound
	}
	return f, nil
}

func (r *MemoryRepo) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.files)
}

type Handler struct {
	Repo  Repo
	Store ObjectStore
	Queue Enqueuer
	Audit *audit.Logger
}

func (h *Handler) Download(c *gin.Context) {
	user := auth.MustUser(c)
	target := c.Param("id")

	f, err := h.findForUser(c, user, target)
	if errors.Is(err, ErrNotFound) {
		h.recordDenied(c, user, "file.download", target)
		problem.Abort(c, problem.New(problem.Forbidden, err))
		return
	}
	if err != nil {
		problem.Abort(c, err)
		return
	}

	url, err := h.Store.PresignGet(f.ID.String(), downloadTTL)
	if err != nil {
		problem.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"url": url})
}

func (h *Handler) Upload(c *gin.Context) { c.AbortWithStatus(http.StatusNotImplemented) }

func (h *Handler) findForUser(c *gin.Context, u auth.User, rawID string) (File, error) {
	id, err := ParseFileID(rawID)
	if err != nil {
		return File{}, ErrNotFound
	}
	return h.Repo.FindForUser(c.Request.Context(), u, id)
}

func (h *Handler) recordDenied(c *gin.Context, u auth.User, action, target string) {
	err := h.Audit.Record(audit.Event{
		Name:   "authz.denied",
		Actor:  u.ID().String(),
		Action: action,
		Target: target,
		Result: "denied",
		Origin: c.ClientIP(),
	})
	if err != nil {
		_ = c.Error(err)
	}
}
