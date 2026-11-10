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

// CanReadOrder: the customer who owns it; support and admin, any order; a
// technician, only the orders assigned to them (D02).
func CanReadOrder(u auth.User, owner auth.UserID, assigned *auth.UserID) error {
	switch u.Role() {
	case auth.RoleCustomer:
		if u.ID() == owner {
			return nil
		}
	case auth.RoleSupport, auth.RoleAdmin:
		return nil
	case auth.RoleTechnician:
		if assigned != nil && *assigned == u.ID() {
			return nil
		}
	}
	return ErrDenied
}

// CanWorkOnOrder covers the review and the download of the customer's file:
// only the technician assigned to the order (E6.3, CA05, CA08). Not even an
// admin may approve or open a design.
func CanWorkOnOrder(u auth.User, assigned *auth.UserID) error {
	if u.Role() == auth.RoleTechnician && assigned != nil && *assigned == u.ID() {
		return nil
	}
	return ErrDenied
}
