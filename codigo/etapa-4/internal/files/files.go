// Package files é o módulo Arquivos.
package files

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
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

// TaskStatus é o que o módulo fila conta sobre o processamento de um arquivo.
// Arquivos não lê a tabela da fila (E3.1).
type TaskStatus struct {
	State     string      `json:"status"`
	VolumeMM3 *float64    `json:"-"`
	BoxMM     *[3]float64 `json:"-"`
}

type TaskReader interface {
	StatusOf(ctx context.Context, id FileID) (TaskStatus, error)
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
	Tasks TaskReader
	Audit *audit.Logger
}

// Status mostra o andamento só ao dono, com a mesma negação do Download.
func (h *Handler) Status(c *gin.Context) {
	user := auth.MustUser(c)
	target := c.Param("id")

	f, err := h.findForUser(c, user, target)
	if errors.Is(err, ErrNotFound) {
		h.recordDenied(c, user, "file.status", target)
		problem.Abort(c, problem.New(problem.Forbidden, err))
		return
	}
	if err != nil {
		problem.Abort(c, err)
		return
	}

	st, err := h.Tasks.StatusOf(c.Request.Context(), f.ID)
	if err != nil {
		problem.Abort(c, err)
		return
	}
	body := gin.H{"id": f.ID.String(), "status": st.State}
	if st.VolumeMM3 != nil && st.BoxMM != nil {
		body["resultado"] = gin.H{"volume_mm3": *st.VolumeMM3, "caixa_mm": st.BoxMM[:]}
	}
	c.JSON(http.StatusOK, body)
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

// Upload só grava o arquivo e cria a tarefa depois do limite de tamanho e do
// stl.Parse (RS03).
func (h *Handler) Upload(c *gin.Context) {
	user := auth.MustUser(c)

	data, err := readBody(c)
	if err != nil {
		h.recordRejected(c, user, err)
		problem.Abort(c, err)
		return
	}
	model, err := stl.Parse(data)
	if err != nil {
		err = stlProblem(err)
		h.recordRejected(c, user, err)
		problem.Abort(c, err)
		return
	}

	f := File{ID: NewFileID(), Owner: user.ID(), Size: int64(len(data)), SHA256: sha256.Sum256(data)}
	ctx := c.Request.Context()
	if err := h.Store.Put(ctx, f.ID.String(), data); err != nil {
		problem.Abort(c, err)
		return
	}
	if err := h.Repo.Save(ctx, f); err != nil {
		problem.Abort(c, err)
		return
	}
	if err := h.Queue.Enqueue(ctx, f, model); err != nil {
		problem.Abort(c, err)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"id": f.ID.String(), "status": "processing"})
}

// readBody recusa um Content-Length acima do limite sem ler o corpo, e corta a
// leitura no limite quando o tamanho não é declarado.
func readBody(c *gin.Context) ([]byte, error) {
	tooLarge := problem.New(problem.FileTooLarge, fmt.Errorf("upload acima de %d bytes", stl.MaxSize))
	if c.Request.ContentLength > stl.MaxSize {
		return nil, tooLarge
	}
	data, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, stl.MaxSize))
	var maxErr *http.MaxBytesError
	if errors.As(err, &maxErr) {
		return nil, tooLarge
	}
	return data, err
}

func stlProblem(err error) error {
	var tooMany *stl.TooManyTrianglesError
	if errors.As(err, &tooMany) {
		return problem.New(problem.TooManyTriangles, err)
	}
	var format *stl.FormatError
	if errors.As(err, &format) {
		return problem.New(problem.InvalidSTL, err)
	}
	return err
}

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

// recordRejected só registra recusas da validação (RS03), com o código do erro
// como motivo. Falhas internas ficam no log de requisições.
func (h *Handler) recordRejected(c *gin.Context, u auth.User, err error) {
	var pe *problem.Error
	if !errors.As(err, &pe) {
		return
	}
	rerr := h.Audit.Record(audit.Event{
		Name:   "upload.rejected",
		Actor:  u.ID().String(),
		Action: "file.upload",
		Result: "rejected",
		Reason: string(pe.Code),
		Origin: c.ClientIP(),
	})
	if rerr != nil {
		_ = c.Error(rerr)
	}
}
