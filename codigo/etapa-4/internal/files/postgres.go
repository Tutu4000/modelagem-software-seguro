package files

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"nuggetmachine/internal/auth"
	"nuggetmachine/internal/authz"
	"nuggetmachine/internal/files/filesdb"
)

// PostgresRepo uses the "files" role, which only has SELECT and INSERT on the
// files table (E3.1). The queries live in queries.sql.
type PostgresRepo struct {
	q *filesdb.Queries
}

func NewPostgresRepo(db filesdb.DBTX) *PostgresRepo { return &PostgresRepo{q: filesdb.New(db)} }

func (r *PostgresRepo) Save(ctx context.Context, f File) error {
	return r.q.InsertFile(ctx, filesdb.InsertFileParams{
		ID:        uuid.UUID(f.ID),
		OwnerID:   uuid.UUID(f.Owner),
		SizeBytes: f.Size,
		Sha256:    f.SHA256[:],
	})
}

// FindForUser filters by owner in the query itself (D01) and still goes
// through the central authorization, which also checks the role.
func (r *PostgresRepo) FindForUser(ctx context.Context, u auth.User, id FileID) (File, error) {
	row, err := r.q.FindFileForOwner(ctx, filesdb.FindFileForOwnerParams{
		ID:      uuid.UUID(id),
		OwnerID: uuid.UUID(u.ID()),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return File{}, ErrNotFound
	}
	if err != nil {
		return File{}, err
	}
	f := File{ID: id, Owner: auth.UserID(row.OwnerID), Size: row.SizeBytes}
	if authz.CanReadFile(u, f.Owner) != nil {
		return File{}, ErrNotFound
	}
	if len(row.Sha256) != len(f.SHA256) {
		return File{}, fmt.Errorf("file %s has a %d-byte hash", id, len(row.Sha256))
	}
	copy(f.SHA256[:], row.Sha256)
	return f, nil
}
