// Package authz centraliza a autorização e nega por padrão (D01).
package authz

import (
	"errors"

	"nuggetmachine/internal/auth"
)

var ErrDenied = errors.New("acesso negado")

func CanReadFile(u auth.User, owner auth.UserID) error {
	if u.Role() == auth.RoleCliente && u.ID() == owner {
		return nil
	}
	return ErrDenied
}
