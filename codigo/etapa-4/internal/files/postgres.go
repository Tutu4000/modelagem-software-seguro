package files

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"nuggetmachine/internal/auth"
	"nuggetmachine/internal/authz"
)

// PostgresRepo usa o papel "arquivos", que só tem SELECT e INSERT na tabela
// arquivos (E3.1).
type PostgresRepo struct {
	db *pgxpool.Pool
}

func NewPostgresRepo(db *pgxpool.Pool) *PostgresRepo { return &PostgresRepo{db: db} }

func (r *PostgresRepo) Save(ctx context.Context, f File) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO arquivos (id, dono, tamanho, sha256) VALUES ($1, $2, $3, $4)`,
		uuid.UUID(f.ID), uuid.UUID(f.Owner), f.Size, f.SHA256[:])
	return err
}

// FindForUser filtra pelo dono na própria consulta (D01) e ainda passa pela
// autorização central, que também confere o perfil.
func (r *PostgresRepo) FindForUser(ctx context.Context, u auth.User, id FileID) (File, error) {
	var (
		owner uuid.UUID
		f     = File{ID: id}
		sum   []byte
	)
	err := r.db.QueryRow(ctx,
		`SELECT dono, tamanho, sha256 FROM arquivos WHERE id = $1 AND dono = $2`,
		uuid.UUID(id), uuid.UUID(u.ID())).Scan(&owner, &f.Size, &sum)
	if errors.Is(err, pgx.ErrNoRows) {
		return File{}, ErrNotFound
	}
	if err != nil {
		return File{}, err
	}
	f.Owner = auth.UserID(owner)
	if authz.CanReadFile(u, f.Owner) != nil {
		return File{}, ErrNotFound
	}
	if len(sum) != len(f.SHA256) {
		return File{}, fmt.Errorf("arquivo %s com hash de %d bytes", id, len(sum))
	}
	copy(f.SHA256[:], sum)
	return f, nil
}
