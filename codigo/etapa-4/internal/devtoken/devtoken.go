// Package devtoken emite tokens de teste no lugar do IdP. Só é usado pelas
// ferramentas de desenvolvimento, nunca pela API.
package devtoken

import (
	"crypto/ed25519"
	"fmt"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// Clientes fictícios com IDs fixos, para que o seed, os testes e o contexto do
// ZAP falem dos mesmos usuários.
var (
	ClienteA = uuid.MustParse("0b7e3a52-5f1d-4c3e-9a51-7f6a1c2d0a01")
	ClienteB = uuid.MustParse("0b7e3a52-5f1d-4c3e-9a51-7f6a1c2d0b02")
)

// Resolve aceita "a", "b" ou um UUID.
func Resolve(sub string) (uuid.UUID, error) {
	switch sub {
	case "a":
		return ClienteA, nil
	case "b":
		return ClienteB, nil
	}
	return uuid.Parse(sub)
}

func LoadKey(path string) (ed25519.PrivateKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	key, err := jwt.ParseEdPrivateKeyFromPEM(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return key.(ed25519.PrivateKey), nil
}

// Sign tem as mesmas claims que o auth.Verifier exige.
func Sign(key ed25519.PrivateKey, sub uuid.UUID, ttl time.Duration) (string, error) {
	now := time.Now()
	return jwt.NewWithClaims(jwt.SigningMethodEdDSA, jwt.MapClaims{
		"sub":  sub.String(),
		"role": "cliente",
		"iat":  now.Unix(),
		"exp":  now.Add(ttl).Unix(),
	}).SignedString(key)
}
