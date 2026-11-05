// Package tests usa a API como um cliente ou um atacante usaria: só pelo que
// os pacotes exportam.
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
	"nuggetmachine/internal/server/cliente"
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
	logs   *bytes.Buffer
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
		store: storage.NewMemory([]byte("segredo-de-teste-do-armazenamento"), "https://armazenamento.local", clk.Now),
		repo:  files.NewMemoryRepo(),
		queue: queue.NewMemory(),
		logs:  &bytes.Buffer{},
		key:   priv,
		clock: clk,
	}
	e.router = cliente.New(cliente.Deps{
		Verifier: auth.NewVerifier(pub, clk.Now),
		Files:    e.repo,
		Store:    e.store,
		Queue:    e.queue,
		Audit:    audit.New(e.logs),
	})
	return e
}

func newUser() auth.UserID { return auth.UserID(uuid.New()) }

func (e *env) claims(user auth.UserID, validity time.Duration) jwt.MapClaims {
	now := e.clock.Now()
	return jwt.MapClaims{
		"sub":  user.String(),
		"role": string(auth.RoleCliente),
		"iat":  now.Unix(),
		"exp":  now.Add(validity).Unix(),
	}
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

// swapSubject troca o sub e mantém a assinatura original.
func swapSubject(t *testing.T, token string, user auth.UserID) string {
	t.Helper()
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("token com %d partes", len(parts))
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
		e.t.Fatalf("gravar os metadados: %v", err)
	}
	if err := e.store.Put(context.Background(), f.ID.String(), content); err != nil {
		e.t.Fatalf("gravar o conteúdo: %v", err)
	}
	return f
}

func downloadPath(id files.FileID) string {
	return "/api/v1/arquivos/" + id.String() + "/download"
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
		e.t.Fatal("a resposta não trouxe a URL")
	}
	return body.URL
}

// size negativo simula um envio sem Content-Length.
func (e *env) upload(token string, body io.Reader, size int64) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/arquivos", body)
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
		e.t.Errorf("%d arquivos gravados no repositório, esperado 0", n)
	}
	if n := e.store.Count(); n != 0 {
		e.t.Errorf("%d objetos gravados no armazenamento, esperado 0", n)
	}
	if n := len(e.queue.Tasks()); n != 0 {
		e.t.Errorf("%d tarefas na fila, esperado 0", n)
	}
}

// zeros gera arquivos grandes sem alocar memória.
type zeros struct{}

func (zeros) Read(p []byte) (int, error) {
	clear(p)
	return len(p), nil
}

func binarySTL(n uint32) []byte {
	b := make([]byte, 84+50*int(n))
	copy(b, "STL gerado pelo teste")
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
	for _, line := range strings.Split(strings.TrimSpace(e.logs.String()), "\n") {
		if line == "" {
			continue
		}
		var ev map[string]any
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			e.t.Fatalf("linha do log não é JSON: %q", line)
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
		t.Fatalf("status %d, esperado %d (corpo: %s)", rec.Code, want, rec.Body.String())
	}
}

func decode(t *testing.T, rec *httptest.ResponseRecorder, v any) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), v); err != nil {
		t.Fatalf("a resposta não é JSON: %q", rec.Body.String())
	}
}

func problemCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/problem+json") {
		t.Fatalf("Content-Type %q, esperado application/problem+json (RFC 9457)", ct)
	}
	var body struct {
		Type   string `json:"type"`
		Title  string `json:"title"`
		Status int    `json:"status"`
		Code   string `json:"code"`
	}
	decode(t, rec, &body)
	if body.Type == "" || body.Title == "" || body.Status != rec.Code {
		t.Errorf("Problem Details incompleto: %s", rec.Body.String())
	}
	return body.Code
}
