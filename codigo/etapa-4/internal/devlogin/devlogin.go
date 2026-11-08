// Package devlogin gets real Keycloak tokens for the tests and the local tools
// (seed, ZAP), through the password grant of the "e2e" client. That client
// only exists in the local realm; people log in through the browser.
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
	CustomerA  = Account{"customer-a", uuid.MustParse("0b7e3a52-5f1d-4c3e-9a51-7f6a1c2d0a01"), false}
	CustomerB  = Account{"customer-b", uuid.MustParse("0b7e3a52-5f1d-4c3e-9a51-7f6a1c2d0b02"), false}
	CustomerC  = Account{"customer-c", uuid.MustParse("0b7e3a52-5f1d-4c3e-9a51-7f6a1c2d0c03"), false}
	Technician = Account{"technician", uuid.MustParse("0b7e3a52-5f1d-4c3e-9a51-7f6a1c2d1a01"), true}
	Support    = Account{"support", uuid.MustParse("0b7e3a52-5f1d-4c3e-9a51-7f6a1c2d1b02"), true}
	Admin      = Account{"admin", uuid.MustParse("0b7e3a52-5f1d-4c3e-9a51-7f6a1c2d1c03"), true}
)

// Resolve accepts "a", "b", "c" or a username.
func Resolve(name string) (Account, error) {
	for _, a := range []Account{CustomerA, CustomerB, CustomerC, Technician, Support, Admin} {
		if name == a.Username || "customer-"+name == a.Username {
			return a, nil
		}
	}
	return Account{}, fmt.Errorf("unknown account %q", name)
}

type Client struct {
	issuer  string
	secrets string
	http    *http.Client
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
	return &Client{
		issuer:  strings.TrimSuffix(issuer, "/"),
		secrets: secretsDir,
		http: &http.Client{
			Timeout:   30 * time.Second,
			Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}},
		},
	}, nil
}

// HTTP is a client that trusts the local CA, for calls to the API.
func (c *Client) HTTP() *http.Client { return c.http }

// Token logs the account in and returns its access token.
func (c *Client) Token(ctx context.Context, a Account) (string, error) {
	form := url.Values{
		"grant_type":    {"password"},
		"client_id":     {"e2e"},
		"client_secret": {c.secret("keycloak_e2e_client_secret")},
		"username":      {a.Username},
		"password":      {c.secret("keycloak_demo_password")},
		"scope":         {"openid"},
	}
	if a.Staff {
		form.Set("otp", TOTP([]byte(c.secret("keycloak_totp_secret")), time.Now()))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.issuer+"/protocol/openid-connect/token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("login of %s: %s: %s", a.Username, resp.Status, bytes.TrimSpace(body))
	}
	var tok struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &tok); err != nil || tok.AccessToken == "" {
		return "", fmt.Errorf("login of %s: no access token in the response", a.Username)
	}
	return tok.AccessToken, nil
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
