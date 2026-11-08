// Package authz centralizes authorization and denies by default (D01).
package authz

import (
	"errors"

	"nuggetmachine/internal/auth"
)

var ErrDenied = errors.New("access denied")

func CanReadFile(u auth.User, owner auth.UserID) error {
	if u.Role() == auth.RoleCustomer && u.ID() == owner {
		return nil
	}
	return ErrDenied
}
