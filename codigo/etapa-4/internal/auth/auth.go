// Package auth tira a identidade só do token, nunca da requisição (RS01).
package auth

import (
	"crypto/ed25519"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type UserID uuid.UUID

func (id UserID) String() string { return uuid.UUID(id).String() }

type Role string

const RoleCliente Role = "cliente"

// Verifier guarda só a chave pública: quem assina os tokens é o IdP.
type Verifier struct {
	pub ed25519.PublicKey
	now func() time.Time
}

func NewVerifier(pub ed25519.PublicKey, now func() time.Time) *Verifier {
	return &Verifier{pub: pub, now: now}
}

func RequireUser(v *Verifier) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.AbortWithStatus(http.StatusNotImplemented)
	}
}
