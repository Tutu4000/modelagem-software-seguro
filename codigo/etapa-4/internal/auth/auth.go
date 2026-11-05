// Package auth tira a identidade só do token, nunca da requisição (RS01).
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

const RoleCliente Role = "cliente"

// User só é criado por Verify, a partir de um token válido.
type User struct {
	id   UserID
	role Role
}

func (u User) ID() UserID { return u.id }

func (u User) Role() Role { return u.role }

// Verifier guarda só a chave pública: quem assina os tokens é o IdP.
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

// Verify aceita só EdDSA, o que barra o alg none e a troca de algoritmo
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
		return User{}, fmt.Errorf("token inválido: %w", err)
	}
	id, err := uuid.Parse(c.Subject)
	if err != nil {
		return User{}, fmt.Errorf("sub não é um UUID: %w", err)
	}
	if c.Role != RoleCliente {
		return User{}, fmt.Errorf("perfil %q não aceito", c.Role)
	}
	return User{id: UserID(id), role: c.Role}, nil
}

func RequireUser(v *Verifier) gin.HandlerFunc {
	return func(c *gin.Context) {
		token, ok := bearerToken(c.GetHeader("Authorization"))
		if !ok {
			problem.Abort(c, problem.New(problem.Unauthenticated, errors.New("sem token Bearer")))
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

// MustUser só pode ser chamado em rotas atrás do RequireUser.
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
