// Package auth takes the identity from the token only, never from the request
// (RS01). Tokens are issued by Keycloak (E5.3).
package auth

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"nuggetmachine/internal/problem"
)

const userKey = "nuggetmachine/auth.user"

type UserID uuid.UUID

func (id UserID) String() string { return uuid.UUID(id).String() }

type Role string

const (
	RoleCustomer   Role = "customer"
	RoleTechnician Role = "technician"
	RoleSupport    Role = "support"
	RoleAdmin      Role = "admin"
)

var knownRoles = []Role{RoleCustomer, RoleTechnician, RoleSupport, RoleAdmin}

// User is only created by Verify, from a valid token.
type User struct {
	id   UserID
	role Role
}

func (u User) ID() UserID { return u.id }

func (u User) Role() Role { return u.role }

// StaticKey verifies every token with one public key. The tests use it in
// place of the IdP.
func StaticKey(pub ed25519.PublicKey) jwt.Keyfunc {
	return func(*jwt.Token) (any, error) { return pub, nil }
}

// JWKS fetches the IdP's public keys over the internal network and refreshes
// them in the background, so a key rotation in Keycloak needs no restart.
func JWKS(ctx context.Context, url string) (jwt.Keyfunc, error) {
	k, err := keyfunc.NewDefaultCtx(ctx, []string{url})
	if err != nil {
		return nil, fmt.Errorf("JWKS at %s: %w", url, err)
	}
	return k.Keyfunc, nil
}

type VerifierConfig struct {
	Keys jwt.Keyfunc
	// Issuer is the public URL of the realm, the one the tokens carry.
	Issuer string
	// Audience is this API's client in Keycloak: a token issued for another
	// client is rejected.
	Audience string
	Now      func() time.Time
}

// Verifier holds only public keys: the IdP signs the tokens.
type Verifier struct {
	keys     jwt.Keyfunc
	issuer   string
	audience string
	now      func() time.Time
}

func NewVerifier(c VerifierConfig) *Verifier {
	return &Verifier{keys: c.Keys, issuer: c.Issuer, audience: c.Audience, now: c.Now}
}

type claims struct {
	Type        string `json:"typ"`
	RealmAccess struct {
		Roles []string `json:"roles"`
	} `json:"realm_access"`
	jwt.RegisteredClaims
}

// Verify accepts only EdDSA, which blocks alg none and algorithm confusion
// (ASVS 9.1.2), and only access tokens from this realm, for this API.
func (v *Verifier) Verify(token string) (User, error) {
	var c claims
	_, err := jwt.ParseWithClaims(token, &c, v.keys,
		jwt.WithValidMethods([]string{jwt.SigningMethodEdDSA.Alg()}),
		jwt.WithExpirationRequired(),
		jwt.WithIssuer(v.issuer),
		jwt.WithAudience(v.audience),
		jwt.WithTimeFunc(v.now),
	)
	if err != nil {
		return User{}, fmt.Errorf("invalid token: %w", err)
	}
	// An ID token has the same issuer and signature: only an access token
	// grants access.
	if c.Type != "Bearer" {
		return User{}, fmt.Errorf("token of type %q is not an access token", c.Type)
	}
	id, err := uuid.Parse(c.Subject)
	if err != nil {
		return User{}, fmt.Errorf("sub is not a UUID: %w", err)
	}
	role, err := appRole(c.RealmAccess.Roles)
	if err != nil {
		return User{}, err
	}
	return User{id: UserID(id), role: role}, nil
}

// appRole requires exactly one application role: an account that is both a
// customer and staff would mix two permission sets (separation of duties).
func appRole(roles []string) (Role, error) {
	var found []Role
	for _, r := range knownRoles {
		if slices.Contains(roles, string(r)) {
			found = append(found, r)
		}
	}
	if len(found) != 1 {
		return "", fmt.Errorf("token has %d application roles, want 1", len(found))
	}
	return found[0], nil
}

// Require authenticates the request and lets through only the given roles.
// No valid token is a 401; a valid token with another role is a 403.
func Require(v *Verifier, roles ...Role) gin.HandlerFunc {
	return func(c *gin.Context) {
		token, ok := bearerToken(c.GetHeader("Authorization"))
		if !ok {
			problem.Abort(c, problem.New(problem.Unauthenticated, errors.New("no Bearer token")))
			return
		}
		u, err := v.Verify(token)
		if err != nil {
			problem.Abort(c, problem.New(problem.Unauthenticated, err))
			return
		}
		if !slices.Contains(roles, u.role) {
			problem.Abort(c, problem.New(problem.Forbidden, fmt.Errorf("role %q not allowed on this route", u.role)))
			return
		}
		c.Set(userKey, u)
		c.Next()
	}
}

// MustUser may only be called on routes behind Require.
func MustUser(c *gin.Context) User {
	return c.MustGet(userKey).(User)
}

func bearerToken(header string) (string, bool) {
	scheme, token, ok := strings.Cut(header, " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") || token == "" {
		return "", false
	}
	return token, true
}
