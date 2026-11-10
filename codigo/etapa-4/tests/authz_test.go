package tests

// Authorization of orders (D01, D02): who reads, reviews and opens a design.

import (
	"testing"
	"time"

	"nuggetmachine/internal/auth"
	"nuggetmachine/internal/authz"
)

// user builds an auth.User the only way there is: from a verified token.
func (e *env) user(id auth.UserID, role auth.Role) auth.User {
	e.t.Helper()
	token := sign(e.t, e.key, with(e.claims(id, 15*time.Minute), "realm_access", map[string]any{"roles": []string{string(role)}}))
	u, err := e.verifier.Verify(token)
	if err != nil {
		e.t.Fatal(err)
	}
	return u
}

func TestOrderAuthorization(t *testing.T) {
	e := newEnv(t)
	owner, other := newUser(), newUser()
	tech, otherTech := newUser(), newUser()

	customer := e.user(owner, auth.RoleCustomer)
	stranger := e.user(other, auth.RoleCustomer)
	assigned := e.user(tech, auth.RoleTechnician)
	notAssigned := e.user(otherTech, auth.RoleTechnician)
	support := e.user(newUser(), auth.RoleSupport)
	admin := e.user(newUser(), auth.RoleAdmin)

	read := []struct {
		name string
		u    auth.User
		want bool
	}{
		{"owner", customer, true},
		{"another customer", stranger, false},
		{"assigned technician", assigned, true},
		{"technician not assigned", notAssigned, false},
		{"support", support, true},
		{"admin", admin, true},
	}
	for _, tc := range read {
		if got := authz.CanReadOrder(tc.u, owner, &tech) == nil; got != tc.want {
			t.Errorf("read by %s: %v, want %v", tc.name, got, tc.want)
		}
	}
	if authz.CanReadOrder(assigned, owner, nil) == nil {
		t.Error("a technician read an order assigned to nobody")
	}

	// Review and the design itself: only the assigned technician (CA05,
	// CA08). Not the owner, not support, not even an admin.
	work := []struct {
		name string
		u    auth.User
		want bool
	}{
		{"assigned technician", assigned, true},
		{"technician not assigned", notAssigned, false},
		{"owner", customer, false},
		{"support", support, false},
		{"admin", admin, false},
	}
	for _, tc := range work {
		if got := authz.CanWorkOnOrder(tc.u, &tech) == nil; got != tc.want {
			t.Errorf("work by %s: %v, want %v", tc.name, got, tc.want)
		}
	}
	if authz.CanWorkOnOrder(assigned, nil) == nil {
		t.Error("a technician worked on an order assigned to nobody")
	}
}
