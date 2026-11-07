//go:build e2e

// Package e2e testa o ambiente do compose de fora, pelo nginx, como um cliente
// ou um atacante faria. Precisa de `just up`. Alguns testes param o worker e
// consultam os bancos pelo `docker compose`.
package e2e

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"nuggetmachine/internal/devtoken"
	"nuggetmachine/internal/fakestl"
)

const (
	base    = "https://localhost"
	secrets = "../../secrets/"
	compose = "../../deploy/compose.yaml"
)

var client = newClient()

func newClient() *http.Client {
	pem, err := os.ReadFile(secrets + "tls_ca.pem")
	if err != nil {
		panic("rode `just secrets` e `just up` antes: " + err.Error())
	}
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(pem)
	return &http.Client{
		Timeout:   time.Minute,
		Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}},
	}
}

// newCustomer cria um cliente sem histórico, para a cota não depender do seed
// nem de outros testes.
func newCustomer(t *testing.T) string {
	t.Helper()
	key, err := devtoken.LoadKey(secrets + "devtoken_ed25519.pem")
	if err != nil {
		t.Fatal(err)
	}
	token, err := devtoken.Sign(key, uuid.New(), 15*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func do(t *testing.T, method, rawURL, token string, body io.Reader, size int64) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, rawURL, body)
	if err != nil {
		t.Fatal(err)
	}
	if size >= 0 {
		req.ContentLength = size
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Content-Type", "model/stl")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func wantStatus(t *testing.T, resp *http.Response, want int) {
	t.Helper()
	if resp.StatusCode != want {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("%s %s: status %d, esperado %d (%s)", resp.Request.Method, resp.Request.URL.Path, resp.StatusCode, want, body)
	}
}

func decode(t *testing.T, resp *http.Response, v any) {
	t.Helper()
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		t.Fatal(err)
	}
}

func upload(t *testing.T, token string, data []byte) string {
	t.Helper()
	resp := do(t, http.MethodPost, base+"/api/v1/arquivos", token, bytes.NewReader(data), int64(len(data)))
	wantStatus(t, resp, http.StatusAccepted)
	var body struct {
		ID string `json:"id"`
	}
	decode(t, resp, &body)
	return body.ID
}

type state struct {
	Status    string `json:"status"`
	Resultado *struct {
		Volume float64    `json:"volume_mm3"`
		Caixa  [3]float64 `json:"caixa_mm"`
	} `json:"resultado"`
}

func status(t *testing.T, token, id string) state {
	t.Helper()
	resp := do(t, http.MethodGet, base+"/api/v1/arquivos/"+id, token, nil, -1)
	wantStatus(t, resp, http.StatusOK)
	var st state
	decode(t, resp, &st)
	return st
}

// waitFor consulta devagar, para não esbarrar no limite de requisições.
func waitFor(t *testing.T, token, id, want string) state {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		st := status(t, token, id)
		if st.Status == want {
			return st
		}
		if time.Now().After(deadline) {
			t.Fatalf("arquivo %s ficou em %q, esperado %q", id, st.Status, want)
		}
		time.Sleep(300 * time.Millisecond)
	}
}

func downloadURL(t *testing.T, token, id string) string {
	t.Helper()
	resp := do(t, http.MethodGet, base+"/api/v1/arquivos/"+id+"/download", token, nil, -1)
	wantStatus(t, resp, http.StatusOK)
	var body struct {
		URL string `json:"url"`
	}
	decode(t, resp, &body)
	return body.URL
}

func dockerCompose(t *testing.T, args ...string) string {
	t.Helper()
	out, err := exec.Command("docker", append([]string{"compose", "-f", compose}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("docker compose %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

// TS01 com o sistema inteiro: upload, processamento pelo worker e download
// pelo Garage.
func TestFluxoCompletoDoUploadAoDownload(t *testing.T) {
	token := newCustomer(t)
	data := fakestl.Box(10, 20, 30)
	id := upload(t, token, data)

	st := waitFor(t, token, id, "concluida")
	if st.Resultado == nil || st.Resultado.Volume != 6000 || st.Resultado.Caixa != [3]float64{10, 20, 30} {
		t.Errorf("resultado inesperado: %+v", st.Resultado)
	}

	resp := do(t, http.MethodGet, downloadURL(t, token, id), "", nil, -1)
	wantStatus(t, resp, http.StatusOK)
	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, data) {
		t.Error("o Garage não entregou o arquivo enviado")
	}
}

// TS02: o outro cliente recebe 403 no estado e no download.
func TestOutroClienteRecebe403(t *testing.T) {
	owner, other := newCustomer(t), newCustomer(t)
	id := upload(t, owner, fakestl.Box(1, 1, 1))

	for _, path := range []string{"/api/v1/arquivos/" + id, "/api/v1/arquivos/" + id + "/download"} {
		wantStatus(t, do(t, http.MethodGet, base+path, other, nil, -1), http.StatusForbidden)
	}
}

// TS05 e C03: o bucket só entrega com assinatura válida e só aceita leitura
// pelo endereço público.
func TestGarageSoEntregaComAssinaturaValida(t *testing.T) {
	token := newCustomer(t)
	signed := downloadURL(t, token, upload(t, token, fakestl.Box(2, 2, 2)))

	u, err := url.Parse(signed)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	sig := q.Get("X-Amz-Signature")
	q.Set("X-Amz-Signature", strings.Repeat("0", len(sig)))
	tampered := *u
	tampered.RawQuery = q.Encode()
	unsigned := *u
	unsigned.RawQuery = ""

	cases := map[string]struct {
		method, url string
	}{
		"assinatura trocada":    {http.MethodGet, tampered.String()},
		"sem assinatura":        {http.MethodGet, unsigned.String()},
		"listagem do bucket":    {http.MethodGet, u.Scheme + "://" + u.Host + "/arquivos"},
		"gravação pelo público": {http.MethodPut, unsigned.String()},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			wantStatus(t, do(t, c.method, c.url, "", strings.NewReader("x"), 1), http.StatusForbidden)
		})
	}
}

// TS07 na borda: o nginx recusa o corpo pelo Content-Length, antes da API.
func TestArquivoDe150MBRecebe413(t *testing.T) {
	const size = 150 << 20
	resp := do(t, http.MethodPost, base+"/api/v1/arquivos", newCustomer(t), io.LimitReader(zeros{}, size), size)
	wantStatus(t, resp, http.StatusRequestEntityTooLarge)
}

type zeros struct{}

func (zeros) Read(p []byte) (int, error) {
	clear(p)
	return len(p), nil
}

// E4.3 e E4.5: sem worker nada é liberado, a quarta tarefa da conta espera, e
// todas concluem quando o worker volta.
func TestCotaEFalhaSeguraSemWorker(t *testing.T) {
	token := newCustomer(t)
	dockerCompose(t, "stop", "worker")
	t.Cleanup(func() { dockerCompose(t, "start", "worker") })

	var ids []string
	for i := range 4 {
		ids = append(ids, upload(t, token, fakestl.Box(float32(i+1), 1, 1)))
	}
	time.Sleep(time.Second)
	for i, id := range ids {
		want := "processando"
		if i == 3 {
			want = "aguardando"
		}
		if st := status(t, token, id); st.Status != want {
			t.Errorf("tarefa %d em %q, esperado %q", i+1, st.Status, want)
		}
	}

	dockerCompose(t, "start", "worker")
	for _, id := range ids {
		waitFor(t, token, id, "concluida")
	}
}

// E3.1 e E7.2: cada papel do banco só alcança o que é dele.
func TestPapeisDoBancoSaoRestritos(t *testing.T) {
	cases := []struct {
		name, service, db, role, secret, sql string
	}{
		{"arquivos não lê tarefas", "postgres", "nugget", "arquivos", "db_arquivos_password", "SELECT 1 FROM tarefas LIMIT 1"},
		{"fila não lê arquivos", "postgres", "nugget", "fila", "db_fila_password", "SELECT 1 FROM arquivos LIMIT 1"},
		{"auditoria não altera eventos", "postgres-auditoria", "auditoria", "auditoria_app", "db_auditoria_password", "UPDATE eventos SET resultado = 'x'"},
		{"auditoria não apaga eventos", "postgres-auditoria", "auditoria", "auditoria_app", "db_auditoria_password", "DELETE FROM eventos"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			password, err := os.ReadFile(secrets + c.secret)
			if err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command("docker", "compose", "-f", compose, "exec", "-T", "-e", "PGPASSWORD",
				c.service, "psql", "-h", "localhost", "-U", c.role, "-d", c.db, "-c", c.sql)
			cmd.Env = append(os.Environ(), "PGPASSWORD="+strings.TrimSpace(string(password)))
			out, err := cmd.CombinedOutput()
			if err == nil || !strings.Contains(string(out), "permission denied") {
				t.Fatalf("esperado permission denied, veio: %v\n%s", err, out)
			}
		})
	}
}

// RS03 e E2.3: uma rajada de um mesmo IP recebe 429. Fica por último porque
// esgota o limite do IP por alguns segundos.
func TestRajadaRecebe429(t *testing.T) {
	limited := 0
	for range 60 {
		resp := do(t, http.MethodGet, base+"/api/v1/arquivos/"+uuid.NewString(), "", nil, -1)
		if resp.StatusCode == http.StatusTooManyRequests {
			limited++
		}
	}
	if limited == 0 {
		t.Fatal("nenhuma das 60 requisições recebeu 429")
	}
}
