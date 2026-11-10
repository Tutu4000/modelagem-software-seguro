// Package files is the Files module.
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
	"golang.org/x/sync/semaphore"

	"nuggetmachine/internal/audit"
	"nuggetmachine/internal/auth"
	"nuggetmachine/internal/authz"
	"nuggetmachine/internal/problem"
	"nuggetmachine/internal/stl"
)

// ErrNotFound covers both a missing file and someone else's file, so that the
// response does not reveal which IDs exist.
var ErrNotFound = errors.New("file not found")

const downloadTTL = 5 * time.Minute // RS01

// FileID is also the object key in the storage. It is a UUIDv4 because the
// customer sees it: a v7 would reveal when each design was uploaded (C02).
// StatusPurged replaces the processing status once the retention deleted the
// file.
const StatusPurged = "purged"

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
	// Purged means the retention deleted the object (E8.5).
	Purged bool
}

// Repo has no lookup by ID alone: every read goes through the central
// authorization (D01).
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

// TaskStatus is what the queue module reports about the processing of a file.
// The files module never reads the queue's table (E3.1).
type TaskStatus struct {
	State     string
	VolumeMM3 *float64
	BBoxMM    *[3]float64
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

	budgetOnce sync.Once
	budget     *semaphore.Weighted
}

// Status shows the progress to the owner only, with the same denial as
// Download.
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

	if f.Purged {
		c.JSON(http.StatusOK, gin.H{"id": f.ID.String(), "status": StatusPurged})
		return
	}
	st, err := h.Tasks.StatusOf(c.Request.Context(), f.ID)
	if err != nil {
		problem.Abort(c, err)
		return
	}
	body := gin.H{"id": f.ID.String(), "status": st.State}
	if st.VolumeMM3 != nil && st.BBoxMM != nil {
		body["result"] = gin.H{"volume_mm3": *st.VolumeMM3, "bbox_mm": st.BBoxMM[:]}
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

	if f.Purged {
		problem.Abort(c, problem.New(problem.FilePurged, fmt.Errorf("file %s was purged", f.ID)))
		return
	}
	url, err := h.Store.PresignGet(f.ID.String(), downloadTTL)
	if err != nil {
		problem.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"url": url})
}

// Upload only stores the file and creates the task after the size limit and
// stl.Parse (RS03).
func (h *Handler) Upload(c *gin.Context) {
	user := auth.MustUser(c)

	data, release, err := h.readBody(c)
	if err != nil {
		h.recordRejected(c, user, err)
		problem.Abort(c, err)
		return
	}
	defer release()
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

// readBody never holds more than the size the STL header declares. Junk is
// refused from the Content-Length or discarded without being buffered, and
// uploadBudget bounds what all the uploads together keep in memory: a few
// parallel 100 MiB bodies would otherwise kill the container. The caller
// calls release when it is done with data.
func (h *Handler) readBody(c *gin.Context) (data []byte, release func(), err error) {
	tooLarge := problem.New(problem.FileTooLarge, fmt.Errorf("upload above %d bytes", stl.MaxSize))
	declared := c.Request.ContentLength
	if declared > stl.MaxSize {
		return nil, nil, tooLarge
	}
	body := http.MaxBytesReader(c.Writer, c.Request.Body, stl.MaxSize)

	header := make([]byte, stl.HeaderSize)
	if n, err := io.ReadFull(body, header); err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, nil, stlProblem(&stl.FormatError{Size: n})
		}
		return nil, nil, err
	}
	triangles, size := stl.Declared(header)
	if declared >= 0 && declared != size {
		return nil, nil, stlProblem(&stl.FormatError{Size: int(declared), Triangles: triangles})
	}
	if triangles > stl.MaxTriangles {
		// Without a Content-Length, only the byte count tells a real STL from
		// a file that is not one.
		if declared < 0 {
			rest, err := discard(body, tooLarge)
			if err != nil {
				return nil, nil, err
			}
			if stl.HeaderSize+rest != size {
				return nil, nil, stlProblem(&stl.FormatError{Size: int(stl.HeaderSize + rest), Triangles: triangles})
			}
		}
		return nil, nil, stlProblem(&stl.TooManyTrianglesError{Triangles: triangles})
	}

	if !h.uploadBudget().TryAcquire(size) {
		return nil, nil, problem.New(problem.Unavailable, errors.New("too many uploads in memory"))
	}
	release = func() { h.uploadBudget().Release(size) }
	data = make([]byte, size)
	copy(data, header)
	n, err := io.ReadFull(body, data[stl.HeaderSize:])
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		release()
		return nil, nil, stlProblem(&stl.FormatError{Size: stl.HeaderSize + n, Triangles: triangles})
	}
	if err != nil {
		release()
		return nil, nil, err
	}
	extra, err := discard(body, tooLarge)
	if err != nil {
		release()
		return nil, nil, err
	}
	if extra > 0 {
		release()
		return nil, nil, stlProblem(&stl.FormatError{Size: int(size + extra), Triangles: triangles})
	}
	return data, release, nil
}

// discard counts what is left of the body without keeping it.
func discard(body io.Reader, tooLarge error) (int64, error) {
	n, err := io.Copy(io.Discard, body)
	var maxErr *http.MaxBytesError
	if errors.As(err, &maxErr) {
		return n, tooLarge
	}
	return n, err
}

// maxUploadBytes fits the API's 512 MB container with room for the rest.
const maxUploadBytes = 256 << 20

func (h *Handler) uploadBudget() *semaphore.Weighted {
	h.budgetOnce.Do(func() { h.budget = semaphore.NewWeighted(maxUploadBytes) })
	return h.budget
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

// recordRejected only records validation rejections (RS03), with the error code
// as the reason. Internal failures go to the request log.
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
