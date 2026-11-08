// Package devlogin gets real Keycloak tokens for the tests and the local tools
// (seed, ZAP). Customers log in through the password grant of the "e2e"
// client, which only exists in the local realm. The panel needs proof of MFA,
// which only the browser flow gives, so BrowserLogin goes through the login
// pages like a person would.
package devlogin

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha1" // #nosec G505 -- TOTP (RFC 6238) is defined over HMAC-SHA1, as configured in the realm
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Account is one of the fictitious users of deploy/keycloak/realm.json.
type Account struct {
	Username string
	ID       uuid.UUID
	// Staff accounts have TOTP configured and need a code to log in.
	Staff bool
}

var (
	CustomerA = Account{"customer-a", uuid.MustParse("0b7e3a52-5f1d-4c3e-9a51-7f6a1c2d0a01"), false}
	CustomerB = Account{"customer-b", uuid.MustParse("0b7e3a52-5f1d-4c3e-9a51-7f6a1c2d0b02"), false}
	CustomerC = Account{"customer-c", uuid.MustParse("0b7e3a52-5f1d-4c3e-9a51-7f6a1c2d0c03"), false}
	// CustomerD is revoked by the panel e2e test, so no other test uses it.
	CustomerD  = Account{"customer-d", uuid.MustParse("0b7e3a52-5f1d-4c3e-9a51-7f6a1c2d0d04"), false}
	Technician = Account{"technician", uuid.MustParse("0b7e3a52-5f1d-4c3e-9a51-7f6a1c2d1a01"), true}
	// Technician2 is never assigned in the tests: it checks that only the
	// assigned technician works on an order.
	Technician2 = Account{"technician-2", uuid.MustParse("0b7e3a52-5f1d-4c3e-9a51-7f6a1c2d1a02"), true}
	Support     = Account{"support", uuid.MustParse("0b7e3a52-5f1d-4c3e-9a51-7f6a1c2d1b02"), true}
	Admin       = Account{"admin", uuid.MustParse("0b7e3a52-5f1d-4c3e-9a51-7f6a1c2d1c03"), true}
	// Admin2 approves the price tables that Admin proposes (two admins, D02).
	Admin2 = Account{"admin-2", uuid.MustParse("0b7e3a52-5f1d-4c3e-9a51-7f6a1c2d1c04"), true}
)

// Resolve accepts "a" to "d" or a username.
func Resolve(name string) (Account, error) {
	for _, a := range []Account{CustomerA, CustomerB, CustomerC, CustomerD, Technician, Technician2, Support, Admin, Admin2} {
		if name == a.Username || "customer-"+name == a.Username {
			return a, nil
		}
	}
	return Account{}, fmt.Errorf("unknown account %q", name)
}

type Client struct {
	issuer  string
	secrets string
	tls     *tls.Config
	http    *http.Client

	mu       sync.Mutex
	lastStep int64 // TOTP time step of the last code used
}

// New reads the client secret, the demo password and the TOTP secret from the
// secrets directory, and trusts the local CA.
func New(issuer, secretsDir string) (*Client, error) {
	pem, err := os.ReadFile(filepath.Join(secretsDir, "tls_ca.pem")) // #nosec G304 -- fixed name inside the secrets directory
	if err != nil {
		return nil, fmt.Errorf("run `just secrets` first: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		return nil, fmt.Errorf("tls_ca.pem: no certificate")
	}
	tlsConfig := &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}
	return &Client{
		issuer:  strings.TrimSuffix(issuer, "/"),
		secrets: secretsDir,
		tls:     tlsConfig,
		http: &http.Client{
			Timeout:   30 * time.Second,
			Transport: &http.Transport{TLSClientConfig: tlsConfig},
		},
	}, nil
}

// otp returns a TOTP code that was not used yet. Keycloak refuses a code used
// twice in the same 30-second step, so a second login in the same step waits
// for the next one.
func (c *Client) otp(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	if step := now.Unix() / 30; step <= c.lastStep {
		wait := time.Unix((c.lastStep+1)*30, 0).Sub(now)
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(wait):
		}
		now = time.Now()
	}
	c.lastStep = now.Unix() / 30
	return TOTP([]byte(c.secret("keycloak_totp_secret")), now), nil
}

// HTTP is a client that trusts the local CA, for calls to the API.
func (c *Client) HTTP() *http.Client { return c.http }

// Tokens is the result of a login. The refresh token is what ends the
// Keycloak session in Logout.
type Tokens struct {
	Access  string `json:"access_token"`
	Refresh string `json:"refresh_token"`
}

// Token logs the account in and returns its access token.
func (c *Client) Token(ctx context.Context, a Account) (string, error) {
	t, err := c.Login(ctx, a)
	return t.Access, err
}

func (c *Client) Login(ctx context.Context, a Account) (Tokens, error) {
	form := url.Values{
		"grant_type": {"password"},
		"username":   {a.Username},
		"password":   {c.secret("keycloak_demo_password")},
		"scope":      {"openid"},
	}
	if a.Staff {
		code, err := c.otp(ctx)
		if err != nil {
			return Tokens{}, err
		}
		form.Set("otp", code)
	}
	body, err := c.post(ctx, "/protocol/openid-connect/token", form)
	if err != nil {
		return Tokens{}, fmt.Errorf("login of %s: %w", a.Username, err)
	}
	var t Tokens
	if err := json.Unmarshal(body, &t); err != nil || t.Access == "" {
		return Tokens{}, fmt.Errorf("login of %s: no access token in the response", a.Username)
	}
	return t, nil
}

// Logout ends the Keycloak session. Keycloak then calls the back-channel
// logout of the API, which revokes the session's access tokens.
func (c *Client) Logout(ctx context.Context, refresh string) error {
	_, err := c.post(ctx, "/protocol/openid-connect/logout", url.Values{"refresh_token": {refresh}})
	return err
}

// post authenticates as the e2e client and expects a 2xx answer.
func (c *Client) post(ctx context.Context, path string, form url.Values) ([]byte, error) {
	form.Set("client_id", "e2e")
	form.Set("client_secret", c.secret("keycloak_e2e_client_secret"))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.issuer+path, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("%s: %s", resp.Status, bytes.TrimSpace(body))
	}
	return body, nil
}

func (c *Client) secret(name string) string {
	data, err := os.ReadFile(filepath.Join(c.secrets, name)) // #nosec G304 -- fixed names inside the secrets directory
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// TOTP computes the 6-digit RFC 6238 code with a 30-second step, the policy of
// the realm. Keycloak uses the secret's bytes as the HMAC key.
func TOTP(secret []byte, t time.Time) string {
	var counter [8]byte
	binary.BigEndian.PutUint64(counter[:], uint64(t.Unix()/30)) // #nosec G115 -- Unix time is positive
	mac := hmac.New(sha1.New, secret)
	mac.Write(counter[:])
	sum := mac.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	code := binary.BigEndian.Uint32(sum[off:off+4]) & 0x7fffffff
	return fmt.Sprintf("%06d", code%1_000_000)
}
