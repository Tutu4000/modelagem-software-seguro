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

// PostgresRepo usa o papel "arquivos", que só tem SELECT e INSERT na tabela
// arquivos (E3.1). As queries ficam em queries.sql.
type PostgresRepo struct {
	q *filesdb.Queries
}

func NewPostgresRepo(db filesdb.DBTX) *PostgresRepo { return &PostgresRepo{q: filesdb.New(db)} }

func (r *PostgresRepo) Save(ctx context.Context, f File) error {
	return r.q.InsertFile(ctx, filesdb.InsertFileParams{
		ID:      uuid.UUID(f.ID),
		Dono:    uuid.UUID(f.Owner),
		Tamanho: f.Size,
		Sha256:  f.SHA256[:],
	})
}

// FindForUser filtra pelo dono na própria consulta (D01) e ainda passa pela
// autorização central, que também confere o perfil.
func (r *PostgresRepo) FindForUser(ctx context.Context, u auth.User, id FileID) (File, error) {
	row, err := r.q.FindFileForOwner(ctx, filesdb.FindFileForOwnerParams{
		ID:   uuid.UUID(id),
		Dono: uuid.UUID(u.ID()),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return File{}, ErrNotFound
	}
	if err != nil {
		return File{}, err
	}
	f := File{ID: id, Owner: auth.UserID(row.Dono), Size: row.Tamanho}
	if authz.CanReadFile(u, f.Owner) != nil {
		return File{}, ErrNotFound
	}
	if len(row.Sha256) != len(f.SHA256) {
		return File{}, fmt.Errorf("arquivo %s com hash de %d bytes", id, len(row.Sha256))
	}
	copy(f.SHA256[:], row.Sha256)
	return f, nil
}
