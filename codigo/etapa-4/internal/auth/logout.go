package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// backchannelLogoutEvent identifies a logout token (OpenID Connect
// Back-Channel Logout 1.0, section 2.4).
const backchannelLogoutEvent = "http://schemas.openid.net/event/backchannel-logout"

// logoutSkew tolerates clock differences between Keycloak and the API.
const logoutSkew = 30 * time.Second

// LogoutToken is what a valid back-channel logout tells the API.
type LogoutToken struct {
	ID        string // jti, used once
	Subject   string
	SessionID string
	IssuedAt  time.Time
}

type logoutClaims struct {
	SessionID string                     `json:"sid"`
	Events    map[string]json.RawMessage `json:"events"`
	Nonce     *string                    `json:"nonce"`
	jwt.RegisteredClaims
}

// VerifyLogout checks a logout token sent by Keycloak: same keys and issuer
// as the access tokens, issued for one of the clients that may ask for a
// logout, recent, with the logout event and a session, and without a nonce,
// which would mean an ID token passed off as a logout token.
func (v *Verifier) VerifyLogout(token string, clients []string, maxAge time.Duration) (LogoutToken, error) {
	var c logoutClaims
	_, err := jwt.ParseWithClaims(token, &c, v.keys,
		jwt.WithValidMethods([]string{jwt.SigningMethodEdDSA.Alg()}),
		jwt.WithIssuer(v.issuer),
		jwt.WithIssuedAt(),
		jwt.WithLeeway(logoutSkew),
		jwt.WithTimeFunc(v.now),
	)
	if err != nil {
		return LogoutToken{}, fmt.Errorf("invalid logout token: %w", err)
	}
	if !slices.ContainsFunc(c.Audience, func(a string) bool { return slices.Contains(clients, a) }) {
		return LogoutToken{}, fmt.Errorf("logout token for audience %v", c.Audience)
	}
	if c.IssuedAt == nil || v.now().Sub(c.IssuedAt.Time) > maxAge {
		return LogoutToken{}, errors.New("logout token without iat or too old")
	}
	if _, ok := c.Events[backchannelLogoutEvent]; !ok {
		return LogoutToken{}, errors.New("logout token without the back-channel logout event")
	}
	if c.Nonce != nil {
		return LogoutToken{}, errors.New("logout token must not carry a nonce")
	}
	if c.SessionID == "" || c.ID == "" {
		return LogoutToken{}, errors.New("logout token without sid or jti")
	}
	return LogoutToken{ID: c.ID, Subject: c.Subject, SessionID: c.SessionID, IssuedAt: c.IssuedAt.Time}, nil
}
