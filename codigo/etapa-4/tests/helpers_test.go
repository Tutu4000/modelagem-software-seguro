// Package tests uses the API the way a customer or an attacker would: only
// through what the packages export.
package tests

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"nuggetmachine/internal/audit"
	"nuggetmachine/internal/auth"
	"nuggetmachine/internal/files"
	"nuggetmachine/internal/queue"
	"nuggetmachine/internal/server/customer"
	"nuggetmachine/internal/storage"
)

func init() { gin.SetMode(gin.TestMode) }

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *clock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

type env struct {
	t      *testing.T
	router http.Handler
	store  *storage.Memory
	repo   *files.MemoryRepo
	queue  *queue.Memory
	audit  *bytes.Buffer // audit log
	app    *bytes.Buffer // request log
	key    ed25519.PrivateKey
	clock  *clock
}

func newEnv(t *testing.T) *env {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	clk := &clock{t: time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)}
	e := &env{
		t:     t,
		store: storage.NewMemory([]byte("storage-test-secret"), "https://storage.local", clk.Now),
		repo:  files.NewMemoryRepo(),
		queue: queue.NewMemory(),
		audit: &bytes.Buffer{},
		app:   &bytes.Buffer{},
		key:   priv,
		clock: clk,
	}
	router, err := customer.New(customer.Deps{
		Verifier: auth.NewVerifier(auth.VerifierConfig{
			Keys:     auth.StaticKey(pub),
			Issuer:   testIssuer,
			Audience: "api",
			Now:      clk.Now,
		}),
		Files: e.repo,
		Store: e.store,
		Queue: e.queue,
		Tasks: e.queue,
		Audit: audit.New(e.audit),
		Log:   slog.New(slog.NewJSONHandler(e.app, nil)),
	})
	if err != nil {
		t.Fatal(err)
	}
	e.router = router
	return e
}

const testIssuer = "https://idp.test/realms/nugget"

func newUser() auth.UserID { return auth.UserID(uuid.New()) }

// claims has the shape of a Keycloak access token for the API.
func (e *env) claims(user auth.UserID, validity time.Duration) jwt.MapClaims {
	now := e.clock.Now()
	return jwt.MapClaims{
		"iss":          testIssuer,
		"aud":          []string{"api"},
		"typ":          "Bearer",
		"sub":          user.String(),
		"realm_access": map[string]any{"roles": []string{string(auth.RoleCustomer)}},
		"iat":          now.Unix(),
		"exp":          now.Add(validity).Unix(),
	}
}

// with returns a copy of the claims with one field replaced.
func with(c jwt.MapClaims, key string, value any) jwt.MapClaims {
	out := jwt.MapClaims{}
	for k, v := range c {
		out[k] = v
	}
	out[key] = value
	return out
}

func sign(t *testing.T, key ed25519.PrivateKey, claims jwt.MapClaims) string {
	t.Helper()
	token, err := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims).SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func (e *env) token(user auth.UserID) string {
	return sign(e.t, e.key, e.claims(user, 15*time.Minute))
}

// swapSubject replaces the sub and keeps the original signature.
func swapSubject(t *testing.T, token string, user auth.UserID) string {
	t.Helper()
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("token with %d parts", len(parts))
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatal(err)
	}
	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		t.Fatal(err)
	}
	claims["sub"] = user.String()
	changed, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	parts[1] = base64.RawURLEncoding.EncodeToString(changed)
	return strings.Join(parts, ".")
}

func (e *env) seedFile(owner auth.UserID, content []byte) files.File {
	e.t.Helper()
	f := files.File{
		ID:     files.NewFileID(),
		Owner:  owner,
		Size:   int64(len(content)),
		SHA256: sha256.Sum256(content),
	}
	if err := e.repo.Save(context.Background(), f); err != nil {
		e.t.Fatalf("store the metadata: %v", err)
	}
	if err := e.store.Put(context.Background(), f.ID.String(), content); err != nil {
		e.t.Fatalf("store the content: %v", err)
	}
	return f
}

func downloadPath(id files.FileID) string {
	return "/api/v1/files/" + id.String() + "/download"
}

func (e *env) do(req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	e.router.ServeHTTP(rec, req)
	return rec
}

func (e *env) get(path, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return e.do(req)
}

func (e *env) downloadURL(owner auth.UserID, f files.File) string {
	e.t.Helper()
	rec := e.get(downloadPath(f.ID), e.token(owner))
	wantStatus(e.t, rec, http.StatusOK)
	var body struct {
		URL string `json:"url"`
	}
	decode(e.t, rec, &body)
	if body.URL == "" {
		e.t.Fatal("the response had no URL")
	}
	return body.URL
}

// A negative size simulates an upload without Content-Length.
func (e *env) upload(token string, body io.Reader, size int64) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/files", body)
	req.ContentLength = size
	req.Header.Set("Content-Type", "model/stl")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return e.do(req)
}

func (e *env) nothingStored() {
	e.t.Helper()
	if n := e.repo.Count(); n != 0 {
		e.t.Errorf("%d files stored in the repository, want 0", n)
	}
	if n := e.store.Count(); n != 0 {
		e.t.Errorf("%d objects stored in the storage, want 0", n)
	}
	if n := len(e.queue.Tasks()); n != 0 {
		e.t.Errorf("%d tasks in the queue, want 0", n)
	}
}

// zeros generates large files without allocating memory.
type zeros struct{}

func (zeros) Read(p []byte) (int, error) {
	clear(p)
	return len(p), nil
}

func binarySTL(n uint32) []byte {
	b := make([]byte, 84+50*int(n))
	copy(b, "STL generated by the test")
	binary.LittleEndian.PutUint32(b[80:84], n)
	return b
}

func binarySTLStream(n uint32) (io.Reader, int64) {
	header := make([]byte, 84)
	binary.LittleEndian.PutUint32(header[80:84], n)
	body := 50 * int64(n)
	return io.MultiReader(bytes.NewReader(header), io.LimitReader(zeros{}, body)), 84 + body
}

func (e *env) fetch(rawURL string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	e.store.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, rawURL, nil))
	return rec
}

func (e *env) events(name string) []map[string]any {
	e.t.Helper()
	var found []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(e.audit.String()), "\n") {
		if line == "" {
			continue
		}
		var ev map[string]any
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			e.t.Fatalf("log line is not JSON: %q", line)
		}
		if ev["event"] == name {
			found = append(found, ev)
		}
	}
	return found
}

func wantStatus(t *testing.T, rec *httptest.ResponseRecorder, want int) {
	t.Helper()
	if rec.Code != want {
		t.Fatalf("status %d, want %d (body: %s)", rec.Code, want, rec.Body.String())
	}
}

func decode(t *testing.T, rec *httptest.ResponseRecorder, v any) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), v); err != nil {
		t.Fatalf("the response is not JSON: %q", rec.Body.String())
	}
}

func problemCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/problem+json") {
		t.Fatalf("Content-Type %q, want application/problem+json (RFC 9457)", ct)
	}
	var body struct {
		Type   string `json:"type"`
		Title  string `json:"title"`
		Status int    `json:"status"`
		Code   string `json:"code"`
	}
	decode(t, rec, &body)
	if body.Type == "" || body.Title == "" || body.Status != rec.Code {
		t.Errorf("incomplete Problem Details: %s", rec.Body.String())
	}
	return body.Code
}
