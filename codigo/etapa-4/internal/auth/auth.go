// Package auth takes the identity from the token only, never from the request
// (RS01).
package auth

import (
	"crypto/ed25519"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"nuggetmachine/internal/problem"
)

const userKey = "nuggetmachine/auth.user"

type UserID uuid.UUID

func (id UserID) String() string { return uuid.UUID(id).String() }

type Role string

const RoleCustomer Role = "customer"

// User is only created by Verify, from a valid token.
type User struct {
	id   UserID
	role Role
}

func (u User) ID() UserID { return u.id }

func (u User) Role() Role { return u.role }

// Verifier holds only the public key: the IdP signs the tokens.
type Verifier struct {
	pub ed25519.PublicKey
	now func() time.Time
}

func NewVerifier(pub ed25519.PublicKey, now func() time.Time) *Verifier {
	return &Verifier{pub: pub, now: now}
}

type claims struct {
	Role Role `json:"role"`
	jwt.RegisteredClaims
}

// Verify accepts only EdDSA, which blocks alg none and algorithm confusion
// (ASVS 9.1.2).
func (v *Verifier) Verify(token string) (User, error) {
	var c claims
	_, err := jwt.ParseWithClaims(token, &c,
		func(*jwt.Token) (any, error) { return v.pub, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodEdDSA.Alg()}),
		jwt.WithExpirationRequired(),
		jwt.WithTimeFunc(v.now),
	)
	if err != nil {
		return User{}, fmt.Errorf("invalid token: %w", err)
	}
	id, err := uuid.Parse(c.Subject)
	if err != nil {
		return User{}, fmt.Errorf("sub is not a UUID: %w", err)
	}
	if c.Role != RoleCustomer {
		return User{}, fmt.Errorf("role %q not accepted", c.Role)
	}
	return User{id: UserID(id), role: c.Role}, nil
}

func RequireUser(v *Verifier) gin.HandlerFunc {
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
		c.Set(userKey, u)
		c.Next()
	}
}

// MustUser may only be called on routes behind RequireUser.
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
