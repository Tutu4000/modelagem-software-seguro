//go:build e2e

// Package e2e tests the compose environment from the outside, through nginx,
// as a customer or an attacker would. It needs `just up`. Some tests stop the
// worker and query the databases through `docker compose`.
package e2e

import (
	"bytes"
	"context"
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

	"nuggetmachine/internal/devlogin"
	"nuggetmachine/internal/fakestl"
)

const (
	base     = "https://localhost"
	idp      = "https://localhost:8444"
	issuer   = idp + "/realms/nugget"
	secrets  = "../../secrets/"
	compose  = "../../deploy/compose.yaml"
	httpWait = time.Minute
)

var (
	keycloak = newLogin()
	client   = keycloak.HTTP()
)

func newLogin() *devlogin.Client {
	c, err := devlogin.New(issuer, secrets)
	if err != nil {
		panic("run `just secrets` and `just up` first: " + err.Error())
	}
	return c
}

// login gets a real access token from Keycloak. Customer C is reserved for the
// quota test, so that its count does not depend on the other tests.
func login(t *testing.T, a devlogin.Account) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), httpWait)
	defer cancel()
	token, err := keycloak.Token(ctx, a)
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
		t.Fatalf("%s %s: status %d, want %d (%s)", resp.Request.Method, resp.Request.URL.Path, resp.StatusCode, want, body)
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
	resp := do(t, http.MethodPost, base+"/api/v1/files", token, bytes.NewReader(data), int64(len(data)))
	wantStatus(t, resp, http.StatusAccepted)
	var body struct {
		ID string `json:"id"`
	}
	decode(t, resp, &body)
	return body.ID
}

type state struct {
	Status string `json:"status"`
	Result *struct {
		Volume float64    `json:"volume_mm3"`
		BBox   [3]float64 `json:"bbox_mm"`
	} `json:"result"`
}

func status(t *testing.T, token, id string) state {
	t.Helper()
	resp := do(t, http.MethodGet, base+"/api/v1/files/"+id, token, nil, -1)
	wantStatus(t, resp, http.StatusOK)
	var st state
	decode(t, resp, &st)
	return st
}

// waitFor polls slowly, so as not to hit the rate limit.
func waitFor(t *testing.T, token, id, want string) state {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		st := status(t, token, id)
		if st.Status == want {
			return st
		}
		if time.Now().After(deadline) {
			t.Fatalf("file %s stayed %q, want %q", id, st.Status, want)
		}
		time.Sleep(300 * time.Millisecond)
	}
}

func downloadURL(t *testing.T, token, id string) string {
	t.Helper()
	resp := do(t, http.MethodGet, base+"/api/v1/files/"+id+"/download", token, nil, -1)
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

// TS01 with the whole system: upload, processing by the worker and download
// through Garage.
func TestFullFlowFromUploadToDownload(t *testing.T) {
	token := login(t, devlogin.CustomerA)
	data := fakestl.Box(10, 20, 30)
	id := upload(t, token, data)

	st := waitFor(t, token, id, "done")
	if st.Result == nil || st.Result.Volume != 6000 || st.Result.BBox != [3]float64{10, 20, 30} {
		t.Errorf("unexpected result: %+v", st.Result)
	}

	resp := do(t, http.MethodGet, downloadURL(t, token, id), "", nil, -1)
	wantStatus(t, resp, http.StatusOK)
	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, data) {
		t.Error("Garage did not deliver the uploaded file")
	}
}

// TS02: another customer gets 403 on the status and on the download.
func TestAnotherCustomerGets403(t *testing.T) {
	owner, other := login(t, devlogin.CustomerA), login(t, devlogin.CustomerB)
	id := upload(t, owner, fakestl.Box(1, 1, 1))

	for _, path := range []string{"/api/v1/files/" + id, "/api/v1/files/" + id + "/download"} {
		wantStatus(t, do(t, http.MethodGet, base+path, other, nil, -1), http.StatusForbidden)
	}
}

// TS05 and C03: the bucket only delivers with a valid signature and only
// accepts reads through the public address.
func TestGarageOnlyDeliversWithValidSignature(t *testing.T) {
	token := login(t, devlogin.CustomerA)
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
		"altered signature":    {http.MethodGet, tampered.String()},
		"no signature":         {http.MethodGet, unsigned.String()},
		"bucket listing":       {http.MethodGet, u.Scheme + "://" + u.Host + "/files"},
		"write through public": {http.MethodPut, unsigned.String()},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			wantStatus(t, do(t, c.method, c.url, "", strings.NewReader("x"), 1), http.StatusForbidden)
		})
	}
}

// TS07 at the edge: nginx rejects the body by its Content-Length, before the
// API.
func TestFileOf150MBGets413(t *testing.T) {
	const size = 150 << 20
	resp := do(t, http.MethodPost, base+"/api/v1/files", login(t, devlogin.CustomerA), io.LimitReader(zeros{}, size), size)
	wantStatus(t, resp, http.StatusRequestEntityTooLarge)
}

type zeros struct{}

func (zeros) Read(p []byte) (int, error) {
	clear(p)
	return len(p), nil
}

// E5.3 and D02: a staff account logs in only with TOTP, and its valid token
// still gets 403 on the customer API.
func TestStaffNeedsTOTPAndCannotUseCustomerAPI(t *testing.T) {
	form := url.Values{
		"grant_type":    {"password"},
		"client_id":     {"e2e"},
		"client_secret": {readSecret(t, "keycloak_e2e_client_secret")},
		"username":      {devlogin.Support.Username},
		"password":      {readSecret(t, "keycloak_demo_password")},
	}
	resp, err := client.PostForm(issuer+"/protocol/openid-connect/token", form)
	if err != nil {
		t.Fatal(err)
	}
	var body struct {
		Error       string `json:"error"`
		AccessToken string `json:"access_token"`
	}
	err = json.NewDecoder(resp.Body).Decode(&body)
	resp.Body.Close()
	// OAuth 2.0 answers rejected credentials with 400 invalid_grant.
	if err != nil || resp.StatusCode != http.StatusBadRequest || body.Error != "invalid_grant" || body.AccessToken != "" {
		t.Errorf("login without TOTP got %d %q, want 400 \"invalid_grant\"", resp.StatusCode, body.Error)
	}

	staff := login(t, devlogin.Support)
	wantStatus(t, do(t, http.MethodGet, base+"/api/v1/files/"+uuid.NewString(), staff, nil, -1), http.StatusForbidden)
}

// The admin console and the master realm are not published (E5.3).
func TestKeycloakAdminIsNotPublic(t *testing.T) {
	for _, path := range []string{"/admin/", "/admin/master/console/", "/realms/master/protocol/openid-connect/token"} {
		wantStatus(t, do(t, http.MethodGet, idp+path, "", nil, -1), http.StatusNotFound)
	}
}

func readSecret(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(secrets + name)
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(data))
}

// E4.3 and E4.5: without a worker nothing is released, the account's fourth
// task waits, and all of them finish once the worker is back.
func TestQuotaAndFailSafeWithoutWorker(t *testing.T) {
	token := login(t, devlogin.CustomerC)
	dockerCompose(t, "stop", "worker")
	t.Cleanup(func() { dockerCompose(t, "start", "worker") })

	var ids []string
	for i := range 4 {
		ids = append(ids, upload(t, token, fakestl.Box(float32(i+1), 1, 1)))
	}
	time.Sleep(time.Second)
	for i, id := range ids {
		want := "processing"
		if i == 3 {
			want = "waiting"
		}
		if st := status(t, token, id); st.Status != want {
			t.Errorf("task %d is %q, want %q", i+1, st.Status, want)
		}
	}

	dockerCompose(t, "start", "worker")
	for _, id := range ids {
		waitFor(t, token, id, "done")
	}
}

// E3.1 and E7.2: each database role only reaches what belongs to it.
func TestDatabaseRolesAreRestricted(t *testing.T) {
	cases := []struct {
		name, service, db, role, secret, sql string
	}{
		{"files cannot read tasks", "postgres", "nugget", "files", "db_files_password", "SELECT 1 FROM tasks LIMIT 1"},
		{"queue cannot read files", "postgres", "nugget", "queue", "db_queue_password", "SELECT 1 FROM files LIMIT 1"},
		{"audit cannot update events", "postgres-audit", "audit", "audit_app", "db_audit_password", "UPDATE events SET result = 'x'"},
		{"audit cannot delete events", "postgres-audit", "audit", "audit_app", "db_audit_password", "DELETE FROM events"},
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
				t.Fatalf("want permission denied, got: %v\n%s", err, out)
			}
		})
	}
}

// C02: files keep UUIDv4, which the customer sees. Tasks and events are
// internal and use UUIDv7, which keeps the index ordered.
func TestIDVersions(t *testing.T) {
	token := login(t, devlogin.CustomerA)
	waitFor(t, token, upload(t, token, fakestl.Box(3, 3, 3)), "done")
	wantStatus(t, do(t, http.MethodGet, base+"/api/v1/files/"+uuid.NewString(), token, nil, -1), http.StatusForbidden)

	cases := []struct {
		service, db, table, want string
	}{
		{"postgres", "nugget", "files", "4"},
		{"postgres", "nugget", "tasks", "7"},
		{"postgres-audit", "audit", "events", "7"},
	}
	for _, c := range cases {
		t.Run(c.table, func(t *testing.T) {
			out := dockerCompose(t, "exec", "-T", c.service, "psql", "-U", "postgres", "-d", c.db, "-tAc",
				"SELECT string_agg(DISTINCT uuid_extract_version(id)::text, ',') FROM "+c.table)
			if got := strings.TrimSpace(out); got != c.want {
				t.Errorf("%s IDs are version %q, want %q", c.table, got, c.want)
			}
		})
	}
}

// RS03 and E2.3: a burst from one IP gets 429. It runs last because it
// exhausts the IP's limit for a few seconds.
func TestBurstGets429(t *testing.T) {
	limited := 0
	for range 60 {
		resp := do(t, http.MethodGet, base+"/api/v1/files/"+uuid.NewString(), "", nil, -1)
		if resp.StatusCode == http.StatusTooManyRequests {
			limited++
		}
	}
	if limited == 0 {
		t.Fatal("none of the 60 requests got 429")
	}
}
