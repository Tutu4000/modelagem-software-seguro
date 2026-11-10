//go:build e2e

// Package e2e tests the compose environment from the outside, through nginx,
// as a customer or an attacker would. It needs `just up`. Some tests stop the
// worker and query the databases through `docker compose`.
package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"

	"nuggetmachine/internal/devlogin"
	"nuggetmachine/internal/fakestl"
	"nuggetmachine/internal/partners"
	"nuggetmachine/internal/webhook"
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

// waitFor polls slowly, so as not to hit the per-IP or the per-user rate
// limit.
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
		time.Sleep(500 * time.Millisecond)
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
		{"orders cannot change a price", "postgres", "nugget", "orders", "db_orders_password", "UPDATE orders SET total_cents = 0"},
		{"panel_orders cannot change a price", "postgres", "nugget", "panel_orders", "db_panel_orders_password", "UPDATE orders SET total_cents = 0"},
		{"orders cannot read tasks", "postgres", "nugget", "orders", "db_orders_password", "SELECT 1 FROM tasks LIMIT 1"},
		{"panel_orders cannot read file owners", "postgres", "nugget", "panel_orders", "db_panel_orders_password", "SELECT owner_id, sha256 FROM files LIMIT 1"},
		{"panel_orders cannot mark a payment", "postgres", "nugget", "panel_orders", "db_panel_orders_password", "UPDATE payments SET status = 'succeeded'"},
		{"orders cannot change a payment amount", "postgres", "nugget", "orders", "db_orders_password", "UPDATE payments SET amount_cents = 1"},
		{"support cannot read order prices", "postgres", "nugget", "support", "db_support_password", "SELECT total_cents FROM orders LIMIT 1"},
		{"panel_support cannot delete messages", "postgres", "nugget", "panel_support", "db_panel_support_password", "DELETE FROM ticket_messages"},
		{"orders cannot change a confirmation code", "postgres", "nugget", "orders", "db_orders_password", "UPDATE address_changes SET code_hash = code_hash"},
		{"retention cannot read tickets", "postgres", "nugget", "retention", "db_retention_password", "SELECT 1 FROM tickets LIMIT 1"},
		{"retention cannot read prices", "postgres", "nugget", "retention", "db_retention_password", "SELECT total_cents FROM orders LIMIT 1"},
		{"retention audit cannot read events", "postgres-audit", "audit", "retention_audit", "db_retention_audit_password", "SELECT 1 FROM events LIMIT 1"},
		{"grafana_reader cannot write the trail", "postgres-audit", "audit", "grafana_reader", "db_grafana_reader_password", "INSERT INTO events (occurred_at, name, actor, action, target, result, reason, origin) VALUES (now(), 'x', 'x', 'x', 'x', 'x', 'x', 'x')"},
		{"grafana_main cannot read addresses", "postgres", "nugget", "grafana_main", "db_grafana_main_password", "SELECT ship_street FROM orders LIMIT 1"},
		{"grafana_main cannot read e-mails", "postgres", "nugget", "grafana_main", "db_grafana_main_password", "SELECT contact_email FROM orders LIMIT 1"},
		{"grafana_main cannot read tickets", "postgres", "nugget", "grafana_main", "db_grafana_main_password", "SELECT body FROM ticket_messages LIMIT 1"},
		{"backup cannot write", "postgres", "nugget", "backup", "db_backup_password", "UPDATE orders SET status = status"},
		{"audit backup cannot write", "postgres-audit", "audit", "backup", "db_audit_backup_password", "DELETE FROM events"},
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

const panelBase = "https://localhost:9445"

// panelLogin logs a staff account in through the panel-web browser flow, with
// password and TOTP. maxAge 0 forces a new login (step-up).
func panelLogin(t *testing.T, a devlogin.Account, maxAge int) string {
	t.Helper()
	browser, err := keycloak.NewBrowser()
	if err != nil {
		t.Fatal(err)
	}
	opts := devlogin.PanelLogin
	opts.MaxAge = maxAge
	ctx, cancel := context.WithTimeout(context.Background(), httpWait)
	defer cancel()
	tokens, err := browser.Login(ctx, a, opts)
	if err != nil {
		t.Fatal(err)
	}
	return tokens.Access
}

// D02: the panel routes do not exist on the public port.
func TestPanelIsNotOnThePublicPort(t *testing.T) {
	wantStatus(t, do(t, http.MethodGet, base+"/panel/v1/me", "", nil, -1), http.StatusNotFound)
}

// D02 and E6.4: only a browser login with TOTP reaches the panel. A password
// grant token, even of staff with TOTP, carries acr 1 and no panel audience.
func TestPanelNeedsBrowserLoginWithMFA(t *testing.T) {
	admin := panelLogin(t, devlogin.Admin, -1)
	resp := do(t, http.MethodGet, panelBase+"/panel/v1/me", admin, nil, -1)
	wantStatus(t, resp, http.StatusOK)
	var me struct {
		Role string `json:"role"`
	}
	decode(t, resp, &me)
	if me.Role != "admin" {
		t.Errorf("role %q, want admin", me.Role)
	}

	for name, token := range map[string]string{
		"staff password grant": login(t, devlogin.Admin),
		"customer token":       login(t, devlogin.CustomerA),
	} {
		t.Run(name, func(t *testing.T) {
			wantStatus(t, do(t, http.MethodGet, panelBase+"/panel/v1/me", token, nil, -1), http.StatusUnauthorized)
		})
	}
}

func TestOnlyAdminsRevokeUsers(t *testing.T) {
	support := panelLogin(t, devlogin.Support, -1)
	body := strings.NewReader(`{"justification":"support trying an admin operation"}`)
	resp := do(t, http.MethodPost, panelBase+"/panel/v1/users/"+devlogin.CustomerD.ID.String()+"/revoke", support, body, -1)
	wantStatus(t, resp, http.StatusForbidden)
}

// E6.4 from end to end: the admin logs in again, revokes customer D with a
// justification, D's existing token stops working on the public API, and D
// can no longer log in.
func TestPanelRevokesUser(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), httpWait)
	defer cancel()
	victim, err := keycloak.Login(ctx, devlogin.CustomerD)
	if err != nil {
		t.Fatal(err)
	}
	path := base + "/api/v1/files/" + uuid.NewString()
	wantStatus(t, do(t, http.MethodGet, path, victim.Access, nil, -1), http.StatusForbidden)
	t.Cleanup(func() { setEnabled(t, devlogin.CustomerD.ID, true) })

	admin := panelLogin(t, devlogin.Admin, 0)
	revoke := panelBase + "/panel/v1/users/" + devlogin.CustomerD.ID.String() + "/revoke"
	wantStatus(t, do(t, http.MethodPost, revoke, admin, strings.NewReader(`{}`), -1), http.StatusUnprocessableEntity)
	resp := do(t, http.MethodPost, revoke, admin, strings.NewReader(`{"justification":"e2e: account seen in a credential leak"}`), -1)
	wantStatus(t, resp, http.StatusOK)

	wantStatus(t, do(t, http.MethodGet, path, victim.Access, nil, -1), http.StatusUnauthorized)
	if _, err := keycloak.Login(ctx, devlogin.CustomerD); err == nil {
		t.Error("the revoked user could still log in")
	}
	out := dockerCompose(t, "exec", "-T", "postgres-audit", "psql", "-U", "postgres", "-d", "audit", "-tAc",
		"SELECT name FROM events WHERE target = '"+devlogin.CustomerD.ID.String()+"' AND name IN ('user.revoked', 'panel.critical') ORDER BY name")
	if got := strings.Fields(out); !slices.Contains(got, "user.revoked") || !slices.Contains(got, "panel.critical") {
		t.Errorf("audit events for the revocation: %v", got)
	}
}

// setEnabled turns an account back on through the Keycloak Admin API, from a
// container on the internal network, since the Admin API is not published.
func setEnabled(t *testing.T, id uuid.UUID, enabled bool) {
	t.Helper()
	script := `T=$(curl -sf -X POST http://keycloak:8080/realms/nugget/protocol/openid-connect/token ` +
		`-d grant_type=client_credentials -d client_id=panel-admin -d client_secret="$S" | sed -n 's/.*"access_token":"\([^"]*\)".*/\1/p') && ` +
		`curl -sf -X PUT -H "Authorization: Bearer $T" -H 'Content-Type: application/json' ` +
		`-d '{"enabled":` + strconv.FormatBool(enabled) + `}' http://keycloak:8080/admin/realms/nugget/users/` + id.String()
	out, err := exec.Command("docker", "run", "--rm", "--network", "nuggetmachine_internal",
		"-e", "S="+readSecret(t, "keycloak_panel_admin_secret"), "curlimages/curl:8.17.0", "sh", "-c", script).CombinedOutput()
	if err != nil {
		t.Fatalf("set enabled=%v for %s: %v\n%s", enabled, id, err, out)
	}
}

// FC2: nginx serves the panel only to the gateway of the public network, where
// loopback and SSH tunnel connections arrive. Another container on that
// network is refused.
func TestPanelRefusesOtherAddresses(t *testing.T) {
	out, err := exec.Command("docker", "run", "--rm", "--network", "nuggetmachine_public", "curlimages/curl:8.17.0",
		"-sk", "-o", "/dev/null", "-w", "%{http_code}", "https://nginx:11443/panel/v1/me").CombinedOutput()
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if code := strings.TrimSpace(string(out)); code != "403" {
		t.Errorf("panel from another container got %s, want 403", code)
	}
}

type cachedToken struct {
	token string
	at    time.Time
}

var staffTokens = map[string]cachedToken{}

// staff returns a panel token of the account, reused for 4 minutes: every new
// staff login waits for a fresh TOTP code. Critical operations accept a login
// up to 5 minutes old.
func staff(t *testing.T, a devlogin.Account) string {
	t.Helper()
	if c, ok := staffTokens[a.Username]; ok && time.Since(c.at) < 4*time.Minute {
		return c.token
	}
	token := panelLogin(t, a, 0)
	staffTokens[a.Username] = cachedToken{token: token, at: time.Now()}
	return token
}

func panelDo(t *testing.T, method, path, token, body string) *http.Response {
	t.Helper()
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	return do(t, method, panelBase+path, token, r, -1)
}

func apiJSON(t *testing.T, method, path, token, body string) *http.Response {
	t.Helper()
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	return do(t, method, base+path, token, r, -1)
}

type orderView struct {
	ID                string `json:"id"`
	FileID            string `json:"file_id"`
	Status            string `json:"status"`
	UnitCents         int64  `json:"unit_cents"`
	ShippingCents     int64  `json:"shipping_cents"`
	TotalCents        int64  `json:"total_cents"`
	PriceTableVersion int64  `json:"price_table_version"`
	TrackingCode      string `json:"tracking_code"`
	Shipping          *struct {
		PostalCode string `json:"postal_code"`
	} `json:"shipping"`
	Payment *struct {
		ChargeID string `json:"charge_id"`
		Status   string `json:"status"`
	} `json:"payment"`
	History []struct {
		To string `json:"to"`
	} `json:"history"`
	Review *struct {
		Decision string `json:"decision"`
		Report   string `json:"report"`
	} `json:"review"`
	DFMFlags []string `json:"dfm_flags"`
}

type catalog struct {
	Version int64 `json:"version"`
	Rules   struct {
		Materials map[string]struct {
			CentsPerCM3 int64 `json:"cents_per_cm3"`
		} `json:"materials"`
		Qualities  map[string]int64 `json:"qualities"`
		Finishings map[string]int64 `json:"finishings"`
		SetupCents int64            `json:"setup_cents"`
	} `json:"rules"`
}

func getCatalog(t *testing.T, token string) catalog {
	t.Helper()
	resp := apiJSON(t, http.MethodGet, "/api/v1/catalog", token, "")
	wantStatus(t, resp, http.StatusOK)
	var c catalog
	decode(t, resp, &c)
	return c
}

// processedFile uploads a box and waits for the worker.
func processedFile(t *testing.T, token string, x, y, z float32) string {
	t.Helper()
	id := upload(t, token, fakestl.Box(x, y, z))
	waitFor(t, token, id, "done")
	return id
}

const plaOrder = `{"file_id":%q,"material":"pla","color":"black","quality":"standard","finishing":"none","quantity":2,` +
	`"shipping":{"recipient":"Customer Test","street":"Avenida Paulista","number":"1000","city":"São Paulo","state":"SP","postal_code":"01310-100"}}`

func createOrder(t *testing.T, token, fileID string) orderView {
	t.Helper()
	resp := apiJSON(t, http.MethodPost, "/api/v1/orders", token, fmt.Sprintf(plaOrder, fileID))
	wantStatus(t, resp, http.StatusCreated)
	var o orderView
	decode(t, resp, &o)
	return o
}

func submittedOrder(t *testing.T, token, fileID string) orderView {
	t.Helper()
	o := createOrder(t, token, fileID)
	resp := apiJSON(t, http.MethodPost, "/api/v1/orders/"+o.ID+"/submit", token, "")
	wantStatus(t, resp, http.StatusOK)
	decode(t, resp, &o)
	return o
}

// E8.1: the quote is computed on the server from the active price table. The
// test recomputes it from the published catalog: a 10 x 20 x 30 mm box is 6
// cm3, two units of black standard PLA without finishing.
func TestOrderQuoteFollowsTheCatalog(t *testing.T) {
	token := login(t, devlogin.CustomerA)
	cat := getCatalog(t, token)
	file := processedFile(t, token, 10, 20, 30)

	o := createOrder(t, token, file)

	unit := (6*cat.Rules.Materials["pla"].CentsPerCM3*cat.Rules.Qualities["standard"]+999)/1000 + cat.Rules.Finishings["none"]
	want := 2*unit + cat.Rules.SetupCents + o.ShippingCents
	if o.Status != "quoted" || o.UnitCents != unit || o.ShippingCents <= 0 || o.TotalCents != want || o.PriceTableVersion != cat.Version {
		t.Errorf("order %+v, want unit %d, shipping quoted, total %d and table %d", o, unit, want, cat.Version)
	}
	resp := apiJSON(t, http.MethodPost, "/api/v1/orders/"+o.ID+"/submit", token, "")
	wantStatus(t, resp, http.StatusOK)
	wantStatus(t, apiJSON(t, http.MethodPost, "/api/v1/orders/"+o.ID+"/submit", token, ""), http.StatusConflict)
}

// CA03 and D01: the customer cannot set the price, use someone else's file,
// order an unprocessed file or read someone else's order.
func TestOrderAttacks(t *testing.T) {
	a, b := login(t, devlogin.CustomerA), login(t, devlogin.CustomerB)
	fileA := processedFile(t, a, 5, 5, 5)

	invalid := func(body string) {
		t.Helper()
		resp := apiJSON(t, http.MethodPost, "/api/v1/orders", a, body)
		wantStatus(t, resp, http.StatusBadRequest)
		var p struct {
			Code string `json:"code"`
		}
		decode(t, resp, &p)
		if p.Code != "invalid_request" {
			t.Errorf("code %q, want invalid_request", p.Code)
		}
	}
	for _, extra := range []string{`"total_cents":1`, `"status":"paid"`, `"price_table_version":1`, `"unit_cents":0`} {
		invalid(strings.TrimSuffix(fmt.Sprintf(plaOrder, fileA), "}") + "," + extra + "}")
	}
	invalid(strings.Replace(fmt.Sprintf(plaOrder, fileA), `"pla"`, `"gold"`, 1))

	wantStatus(t, apiJSON(t, http.MethodPost, "/api/v1/orders", b, fmt.Sprintf(plaOrder, fileA)), http.StatusForbidden)

	orderA := createOrder(t, a, fileA)
	for _, path := range []string{"/api/v1/orders/" + orderA.ID, "/api/v1/orders/" + uuid.NewString()} {
		wantStatus(t, apiJSON(t, http.MethodGet, path, b, ""), http.StatusForbidden)
	}
	wantStatus(t, apiJSON(t, http.MethodPost, "/api/v1/orders/"+orderA.ID+"/submit", b, ""), http.StatusForbidden)

	dockerCompose(t, "stop", "worker")
	t.Cleanup(func() { dockerCompose(t, "start", "worker") })
	pending := upload(t, b, fakestl.Box(4, 4, 4))
	wantStatus(t, apiJSON(t, http.MethodPost, "/api/v1/orders", b, fmt.Sprintf(plaOrder, pending)), http.StatusConflict)
}

// E6.3, CA05 and CA08: only the assigned technician opens the design and
// reviews it, once; the customer then sees the decision and the report.
func TestTechnicalReview(t *testing.T) {
	customer := login(t, devlogin.CustomerA)
	data := fakestl.Box(12, 12, 12)
	fileID := upload(t, customer, data)
	waitFor(t, customer, fileID, "done")
	o := submittedOrder(t, customer, fileID)
	orderPath := "/panel/v1/orders/" + o.ID

	admin := staff(t, devlogin.Admin)
	wantStatus(t, panelDo(t, http.MethodPost, orderPath+"/assign", admin,
		`{"technician_id":"`+devlogin.Support.ID.String()+`"}`), http.StatusBadRequest)
	wantStatus(t, panelDo(t, http.MethodPost, orderPath+"/assign", admin,
		`{"technician_id":"`+devlogin.Technician.ID.String()+`"}`), http.StatusOK)

	const justification = `{"justification":"e2e: DFM review of the order"}`
	const approval = `{"decision":"approved","report":"e2e: walls thick enough, fits the printer"}`
	other := staff(t, devlogin.Technician2)
	wantStatus(t, panelDo(t, http.MethodPost, orderPath+"/file", other, justification), http.StatusForbidden)
	wantStatus(t, panelDo(t, http.MethodPost, orderPath+"/review", other, approval), http.StatusForbidden)
	wantStatus(t, panelDo(t, http.MethodGet, orderPath, other, ""), http.StatusForbidden)
	support := staff(t, devlogin.Support)
	wantStatus(t, panelDo(t, http.MethodPost, orderPath+"/file", support, justification), http.StatusForbidden)
	wantStatus(t, panelDo(t, http.MethodPost, orderPath+"/review", admin, approval), http.StatusForbidden)

	tech := staff(t, devlogin.Technician)
	resp := panelDo(t, http.MethodPost, orderPath+"/file", tech, justification)
	wantStatus(t, resp, http.StatusOK)
	var file struct {
		URL string `json:"url"`
	}
	decode(t, resp, &file)
	got := do(t, http.MethodGet, file.URL, "", nil, -1)
	wantStatus(t, got, http.StatusOK)
	if body, _ := io.ReadAll(got.Body); !bytes.Equal(body, data) {
		t.Error("the technician's URL did not deliver the design")
	}

	wantStatus(t, panelDo(t, http.MethodPost, orderPath+"/review", tech, approval), http.StatusOK)
	wantStatus(t, panelDo(t, http.MethodPost, orderPath+"/review", tech, approval), http.StatusConflict)

	resp = apiJSON(t, http.MethodGet, "/api/v1/orders/"+o.ID, customer, "")
	wantStatus(t, resp, http.StatusOK)
	var seen orderView
	decode(t, resp, &seen)
	if seen.Status != "awaiting_payment" || seen.Review == nil || seen.Review.Decision != "approved" || len(seen.History) != 3 {
		t.Errorf("the customer sees %+v", seen)
	}

	out := dockerCompose(t, "exec", "-T", "postgres-audit", "psql", "-U", "postgres", "-d", "audit", "-tAc",
		"SELECT DISTINCT name FROM events WHERE target = '"+o.ID+"' ORDER BY name")
	for _, want := range []string{"file.staff_download", "order.assigned", "order.created", "order.submitted", "panel.critical", "review.approved"} {
		if !slices.Contains(strings.Fields(out), want) {
			t.Errorf("audit event %s missing for the order: %v", want, strings.Fields(out))
		}
	}
}

// R14: the assignment is never cleared, so the technician could keep opening
// the design of an order that is over. The status bounds the access.
func TestTechnicianCannotOpenTheDesignOfAFinishedOrder(t *testing.T) {
	customer := login(t, devlogin.CustomerB)
	o := submittedOrder(t, customer, processedFile(t, customer, 9, 9, 9))
	orderPath := "/panel/v1/orders/" + o.ID
	wantStatus(t, panelDo(t, http.MethodPost, orderPath+"/assign", staff(t, devlogin.Admin),
		`{"technician_id":"`+devlogin.Technician.ID.String()+`"}`), http.StatusOK)

	const justification = `{"justification":"e2e: DFM review of the order"}`
	tech := staff(t, devlogin.Technician)
	wantStatus(t, panelDo(t, http.MethodPost, orderPath+"/file", tech, justification), http.StatusOK)

	cancel(t, customer, o.ID, http.StatusOK)
	wantStatus(t, panelDo(t, http.MethodPost, orderPath+"/file", tech, justification), http.StatusConflict)
}

// D02: a new price table only applies after another admin approves it, and
// orders keep the price they were quoted with.
func TestPriceTableNeedsTwoAdmins(t *testing.T) {
	customer := login(t, devlogin.CustomerA)
	before := getCatalog(t, customer)
	fileID := processedFile(t, customer, 10, 20, 30)
	old := createOrder(t, customer, fileID)

	admin := staff(t, devlogin.Admin)
	resp := panelDo(t, http.MethodGet, "/panel/v1/price-tables", admin, "")
	wantStatus(t, resp, http.StatusOK)
	var tables struct {
		PriceTables []struct {
			Version int64           `json:"version"`
			Status  string          `json:"status"`
			Rules   json.RawMessage `json:"rules"`
		} `json:"price_tables"`
	}
	decode(t, resp, &tables)
	var rules map[string]any
	for _, pt := range tables.PriceTables {
		if pt.Status == "active" {
			if err := json.Unmarshal(pt.Rules, &rules); err != nil {
				t.Fatal(err)
			}
		}
	}
	pla := rules["materials"].(map[string]any)["pla"].(map[string]any)
	pla["cents_per_cm3"] = pla["cents_per_cm3"].(float64) + 5
	proposal, _ := json.Marshal(map[string]any{"rules": rules})
	resp = panelDo(t, http.MethodPost, "/panel/v1/price-tables", admin, string(proposal))
	wantStatus(t, resp, http.StatusCreated)
	var draft struct {
		Version int64 `json:"version"`
	}
	decode(t, resp, &draft)

	if c := getCatalog(t, customer); c.Version != before.Version {
		t.Errorf("the draft changed the active table to %d", c.Version)
	}
	approve := fmt.Sprintf("/panel/v1/price-tables/%d/approve", draft.Version)
	const justification = `{"justification":"e2e: PLA supplier raised the price"}`
	wantStatus(t, panelDo(t, http.MethodPost, approve, admin, justification), http.StatusForbidden)
	wantStatus(t, panelDo(t, http.MethodPost, approve, staff(t, devlogin.Admin2), justification), http.StatusOK)

	if c := getCatalog(t, customer); c.Version != draft.Version {
		t.Errorf("active table %d, want %d", c.Version, draft.Version)
	}
	fresh := createOrder(t, customer, fileID)
	if fresh.PriceTableVersion != draft.Version || fresh.UnitCents <= old.UnitCents {
		t.Errorf("new quote %+v should use table %d with a higher price than %d", fresh, draft.Version, old.UnitCents)
	}
	resp = apiJSON(t, http.MethodGet, "/api/v1/orders/"+old.ID, customer, "")
	wantStatus(t, resp, http.StatusOK)
	var kept orderView
	decode(t, resp, &kept)
	if kept.TotalCents != old.TotalCents || kept.PriceTableVersion != old.PriceTableVersion {
		t.Errorf("the old order changed: %+v, was %+v", kept, old)
	}
}

// approvedOrder takes a new order of the customer to awaiting_payment: quote,
// submission, assignment and approval by the technician.
func approvedOrder(t *testing.T, customer string) orderView {
	t.Helper()
	o := submittedOrder(t, customer, processedFile(t, customer, 10, 10, 10))
	wantStatus(t, panelDo(t, http.MethodPost, "/panel/v1/orders/"+o.ID+"/assign", staff(t, devlogin.Admin),
		`{"technician_id":"`+devlogin.Technician.ID.String()+`"}`), http.StatusOK)
	wantStatus(t, panelDo(t, http.MethodPost, "/panel/v1/orders/"+o.ID+"/review", staff(t, devlogin.Technician),
		`{"decision":"approved","report":"e2e: approved for the payment tests"}`), http.StatusOK)
	o.Status = "awaiting_payment"
	return o
}

func orderOf(t *testing.T, customer, id string) orderView {
	t.Helper()
	resp := apiJSON(t, http.MethodGet, "/api/v1/orders/"+id, customer, "")
	wantStatus(t, resp, http.StatusOK)
	var o orderView
	decode(t, resp, &o)
	return o
}

// waitOrder polls once a second: the per-user limit (60 per minute) is shared
// with the rest of the test.
func waitOrder(t *testing.T, customer, id, want string) orderView {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		o := orderOf(t, customer, id)
		if o.Status == want {
			return o
		}
		if time.Now().After(deadline) {
			t.Fatalf("order %s stayed %q, want %q", id, o.Status, want)
		}
		time.Sleep(time.Second)
	}
}

func pay(t *testing.T, customer, id string) string {
	t.Helper()
	resp := apiJSON(t, http.MethodPost, "/api/v1/orders/"+id+"/payment", customer, "")
	wantStatus(t, resp, http.StatusAccepted)
	var p struct {
		ChargeID string `json:"charge_id"`
	}
	decode(t, resp, &p)
	return p.ChargeID
}

// E8.2 and E8.3 from end to end: the gateway's signed notification pays the
// order, the technician starts production and books the pickup, and the
// carrier's notifications ship and deliver it.
func TestOrderFromPaymentToDelivery(t *testing.T) {
	customer := login(t, devlogin.CustomerA)
	o := approvedOrder(t, customer)

	pay(t, customer, o.ID)
	paid := waitOrder(t, customer, o.ID, "paid")
	if paid.Payment == nil || paid.Payment.Status != "succeeded" {
		t.Errorf("payment %+v, want succeeded", paid.Payment)
	}

	tech := staff(t, devlogin.Technician)
	orderPath := "/panel/v1/orders/" + o.ID
	wantStatus(t, panelDo(t, http.MethodPost, orderPath+"/ship", tech, ""), http.StatusConflict)
	wantStatus(t, panelDo(t, http.MethodPost, orderPath+"/production", staff(t, devlogin.Technician2), ""), http.StatusForbidden)
	wantStatus(t, panelDo(t, http.MethodPost, orderPath+"/production", tech, ""), http.StatusOK)
	wantStatus(t, panelDo(t, http.MethodPost, orderPath+"/ship", tech, ""), http.StatusAccepted)
	wantStatus(t, panelDo(t, http.MethodPost, orderPath+"/ship", tech, ""), http.StatusConflict)

	// RF15: a problem on the way, signed by the carrier, is told to the
	// customer and does not move the order.
	waitOrder(t, customer, o.ID, "shipped")
	shipment := strings.TrimSpace(dockerCompose(t, "exec", "-T", "postgres", "psql", "-U", "postgres", "-d", "nugget", "-tAc",
		"SELECT shipment_id FROM orders WHERE id = '"+o.ID+"'"))
	problemEvent, _ := json.Marshal(partners.ShippingEvent{EventID: "evt_e2e_" + uuid.NewString(), Type: partners.ShipmentException,
		ShipmentID: shipment, OrderID: uuid.MustParse(o.ID)})
	wantStatus(t, signedPost(t, "/api/v1/webhooks/shipping", []byte(readSecret(t, "partner_shipping_webhook_secret")), time.Now(), problemEvent), http.StatusOK)

	done := waitOrder(t, customer, o.ID, "delivered")
	var steps []string
	for _, h := range done.History {
		steps = append(steps, h.To)
	}
	want := []string{"quoted", "awaiting_review", "awaiting_payment", "paid", "in_production", "shipped", "delivered"}
	if !slices.Equal(steps, want) || done.TrackingCode == "" {
		t.Errorf("history %v and tracking %q, want %v and a tracking code", steps, done.TrackingCode, want)
	}

	// The audit record is written right after the commit that the status
	// poll already sees, so it is polled too.
	want = []string{"order.in_production", "payment.requested", "payment.succeeded", "shipment.delivered", "shipment.exception", "shipment.picked_up", "shipment.requested"}
	deadline := time.Now().Add(10 * time.Second)
	for {
		out := dockerCompose(t, "exec", "-T", "postgres-audit", "psql", "-U", "postgres", "-d", "audit", "-tAc",
			"SELECT DISTINCT name FROM events WHERE target = '"+o.ID+"' AND name LIKE ANY (ARRAY['payment.%', 'shipment.%', 'order.in_production']) ORDER BY name")
		got := strings.Fields(out)
		if slices.Equal(got, want) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("audit events %v, want %v", got, want)
		}
		time.Sleep(500 * time.Millisecond)
	}

	// E8.4: each step reached the registered e-mail, and none of the notices
	// carries the address.
	short := o.ID[:8]
	for _, subject := range []string{"Pedido " + short + " aprovado", "Pagamento do pedido " + short + " confirmado",
		"Pedido " + short + " enviado", "Problema na entrega do pedido " + short, "Pedido " + short + " entregue"} {
		text := mailTo(t, devlogin.CustomerA, subject)
		if strings.Contains(text, "Paulista") || strings.Contains(text, "01310") {
			t.Errorf("%q carries the address: %q", subject, text)
		}
	}
	if text := mailTo(t, devlogin.CustomerA, "Pedido "+short+" enviado"); !strings.Contains(text, done.TrackingCode) {
		t.Errorf("the shipping notice has no tracking code: %q", text)
	}

	// C21: no address change after the dispatch.
	wantStatus(t, panelDo(t, http.MethodPost, orderPath+"/address-change", staff(t, devlogin.Support), newAddressBody), http.StatusConflict)
}

const mailpit = "http://127.0.0.1:8026"

// mailTo waits for the newest message to the account with the subject and
// returns its text, read from the Mailpit API as the customer's inbox.
func mailTo(t *testing.T, a devlogin.Account, subject string) string {
	t.Helper()
	q := url.Values{"query": {fmt.Sprintf(`to:"%s@nugget.local" subject:"%s"`, a.Username, subject)}}
	deadline := time.Now().Add(20 * time.Second)
	for {
		var found struct {
			Messages []struct {
				ID      string `json:"ID"`
				Subject string `json:"Subject"`
			} `json:"messages"`
		}
		getJSON(t, mailpit+"/api/v1/search?"+q.Encode(), &found)
		for _, m := range found.Messages {
			if m.Subject == subject {
				var msg struct {
					Text string `json:"Text"`
				}
				getJSON(t, mailpit+"/api/v1/message/"+m.ID, &msg)
				return msg.Text
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("no e-mail %q to %s", subject, a.Username)
		}
		time.Sleep(time.Second)
	}
}

func getJSON(t *testing.T, rawURL string, v any) {
	t.Helper()
	resp, err := http.Get(rawURL) // #nosec G107 -- the local Mailpit
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: %s", rawURL, resp.Status)
	}
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		t.Fatal(err)
	}
}

const newAddressBody = `{"address":{"recipient":"Customer Test","street":"Rua Augusta","number":"500","city":"São Paulo",` +
	`"state":"SP","postal_code":"01304-000"},"justification":"e2e: customer asked for a new delivery address"}`

// signedPost sends a webhook to the public address as the partner would, or
// as an attacker who does not have the secret. It keeps to the nginx rate per
// IP (10 r/s), so a burst of attacks is not cut by the limiter instead of by
// the signature check.
func signedPost(t *testing.T, path string, secret []byte, at time.Time, body []byte) *http.Response {
	t.Helper()
	time.Sleep(150 * time.Millisecond)
	req, err := http.NewRequest(http.MethodPost, base+path, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if secret != nil {
		req.Header.Set(webhook.Header, webhook.Sign(secret, at, body))
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

// C14: the payment webhook is public, so the signature, the timestamp, the
// event ID and the amount are what protect it. The test plays the gateway
// with the real secret, and an attacker without it, before the simulator's
// own notification arrives.
func TestPaymentWebhookAttacks(t *testing.T) {
	customer := login(t, devlogin.CustomerB)
	o := approvedOrder(t, customer)
	charge := pay(t, customer, o.ID)
	secret := []byte(readSecret(t, "partner_payments_webhook_secret"))
	const path = "/api/v1/webhooks/payments"
	event := func(id string, amount int64) []byte {
		b, _ := json.Marshal(partners.PaymentEvent{EventID: id, Type: partners.PaymentSucceeded, ChargeID: charge,
			OrderID: uuid.MustParse(o.ID), AmountCents: amount})
		return b
	}
	now := time.Now()

	good := event("evt_e2e_"+uuid.NewString(), o.TotalCents)
	altered := bytes.Replace(good, []byte(strconv.FormatInt(o.TotalCents, 10)), []byte("1"), 1)
	attacks := map[string]*http.Response{
		"no signature":   signedPost(t, path, nil, now, good),
		"another secret": signedPost(t, path, []byte("guessed-secret"), now, good),
		"old timestamp":  signedPost(t, path, secret, now.Add(-10*time.Minute), good),
		"wrong amount":   signedPost(t, path, secret, now, event("evt_e2e_"+uuid.NewString(), 1)),
		"unknown charge": signedPost(t, path, secret, now, bytes.Replace(good, []byte(charge), []byte("ch_forged"), 1)),
	}
	time.Sleep(150 * time.Millisecond)
	req, _ := http.NewRequest(http.MethodPost, base+path, bytes.NewReader(altered))
	req.Header.Set(webhook.Header, webhook.Sign(secret, now, good))
	altResp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	altResp.Body.Close()
	attacks["body altered after signing"] = altResp
	for name, resp := range attacks {
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("%s: got %d, want 400", name, resp.StatusCode)
		}
	}
	if st := orderOf(t, customer, o.ID).Status; st != "awaiting_payment" {
		t.Fatalf("after the attacks the order is %q, want awaiting_payment", st)
	}

	first := signedPost(t, path, secret, now, good)
	replay := signedPost(t, path, secret, now, good)
	if first.StatusCode != http.StatusOK || replay.StatusCode != http.StatusOK {
		t.Errorf("valid event got %d, replay %d, want 200 and 200", first.StatusCode, replay.StatusCode)
	}
	var replayed struct {
		Status string `json:"status"`
	}
	decode(t, replay, &replayed)
	if replayed.Status != "duplicate" {
		t.Errorf("replay answered %q, want duplicate", replayed.Status)
	}
	if st := orderOf(t, customer, o.ID).Status; st != "paid" {
		t.Errorf("order %q, want paid", st)
	}

	out := dockerCompose(t, "exec", "-T", "postgres-audit", "psql", "-U", "postgres", "-d", "audit", "-tAc",
		"SELECT count(*) FROM events WHERE name = 'payment.rejected' AND target = '"+o.ID+"' AND reason LIKE '%amount_mismatch%'")
	if strings.TrimSpace(out) != "1" {
		t.Errorf("payment.rejected with amount_mismatch: %q, want 1", strings.TrimSpace(out))
	}
}

// C15: a forged carrier notification does not ship anything.
func TestShippingWebhookForged(t *testing.T) {
	body, _ := json.Marshal(partners.ShippingEvent{EventID: "evt_forged", Type: partners.ShipmentDelivered,
		ShipmentID: "shp_forged", OrderID: uuid.New()})
	for name, resp := range map[string]*http.Response{
		"no signature":   signedPost(t, "/api/v1/webhooks/shipping", nil, time.Now(), body),
		"another secret": signedPost(t, "/api/v1/webhooks/shipping", []byte("guessed"), time.Now(), body),
		"payments secret on shipping": signedPost(t, "/api/v1/webhooks/shipping",
			[]byte(readSecret(t, "partner_payments_webhook_secret")), time.Now(), body),
	} {
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("%s: got %d, want 400", name, resp.StatusCode)
		}
	}
	// Even with the right secret, a shipment that is not the order's is refused.
	resp := signedPost(t, "/api/v1/webhooks/shipping", []byte(readSecret(t, "partner_shipping_webhook_secret")), time.Now(), body)
	wantStatus(t, resp, http.StatusBadRequest)
}

func TestPaymentOutOfOrder(t *testing.T) {
	a, b := login(t, devlogin.CustomerA), login(t, devlogin.CustomerB)
	quoted := createOrder(t, a, processedFile(t, a, 6, 6, 6))
	wantStatus(t, apiJSON(t, http.MethodPost, "/api/v1/orders/"+quoted.ID+"/payment", a, ""), http.StatusConflict)
	wantStatus(t, apiJSON(t, http.MethodPost, "/api/v1/orders/"+quoted.ID+"/payment", b, ""), http.StatusForbidden)
}

// D03: without the partners, nothing is quoted or charged, and the order
// stays where it was.
func TestPartnersDownFailSafe(t *testing.T) {
	customer := login(t, devlogin.CustomerB)
	o := approvedOrder(t, customer)
	file := processedFile(t, customer, 7, 7, 7)
	dockerCompose(t, "stop", "partnersim")
	t.Cleanup(func() { dockerCompose(t, "start", "partnersim") })

	for name, resp := range map[string]*http.Response{
		"quote":   apiJSON(t, http.MethodPost, "/api/v1/orders", customer, fmt.Sprintf(plaOrder, file)),
		"payment": apiJSON(t, http.MethodPost, "/api/v1/orders/"+o.ID+"/payment", customer, ""),
	} {
		if resp.StatusCode != http.StatusBadGateway {
			t.Errorf("%s with the partner down got %d, want 502", name, resp.StatusCode)
		}
	}
	if st := orderOf(t, customer, o.ID); st.Status != "awaiting_payment" || st.Payment != nil {
		t.Errorf("order %+v, want awaiting_payment without a payment", st)
	}
}

// LGPD, data minimization: support sees the delivery address; the technician
// works on the part and does not.
func TestAddressVisibleOnlyToWhoNeedsIt(t *testing.T) {
	customer := login(t, devlogin.CustomerB)
	o := approvedOrder(t, customer)
	for _, tc := range []struct {
		who  devlogin.Account
		sees bool
	}{{devlogin.Technician, false}, {devlogin.Support, true}} {
		resp := panelDo(t, http.MethodGet, "/panel/v1/orders/"+o.ID, staff(t, tc.who), "")
		wantStatus(t, resp, http.StatusOK)
		var seen orderView
		decode(t, resp, &seen)
		if (seen.Shipping != nil) != tc.sees {
			t.Errorf("%s sees the address: %v, want %v", tc.who.Username, seen.Shipping != nil, tc.sees)
		}
	}
}

type cancelled struct {
	Status  string `json:"status"`
	Payment *struct {
		Status   string `json:"status"`
		RefundID string `json:"refund_id"`
	} `json:"payment"`
}

func cancel(t *testing.T, customer, id string, want int) cancelled {
	t.Helper()
	resp := apiJSON(t, http.MethodPost, "/api/v1/orders/"+id+"/cancel", customer, `{"reason":"e2e: changed my mind"}`)
	wantStatus(t, resp, want)
	var c cancelled
	decode(t, resp, &c)
	return c
}

func paidOrder(t *testing.T, customer string) orderView {
	t.Helper()
	o := approvedOrder(t, customer)
	pay(t, customer, o.ID)
	return waitOrder(t, customer, o.ID, "paid")
}

// RF10, CA06 and T11: only the owner cancels, only up to paid; a paid order
// is refunded at the gateway, and the trail keeps who, from where and why.
func TestCancellation(t *testing.T) {
	a, b := login(t, devlogin.CustomerA), login(t, devlogin.CustomerB)

	review := submittedOrder(t, b, processedFile(t, b, 8, 8, 8))
	wantStatus(t, apiJSON(t, http.MethodPost, "/api/v1/orders/"+review.ID+"/cancel", a, ""), http.StatusForbidden)
	if st := orderOf(t, b, review.ID).Status; st != "awaiting_review" {
		t.Fatalf("another customer's cancel moved the order to %q", st)
	}
	if c := cancel(t, b, review.ID, http.StatusOK); c.Status != "cancelled" || c.Payment != nil {
		t.Errorf("cancel of an order in review: %+v", c)
	}
	wantStatus(t, apiJSON(t, http.MethodPost, "/api/v1/orders/"+review.ID+"/cancel", b, ""), http.StatusConflict)
	mailTo(t, devlogin.CustomerB, "Pedido "+review.ID[:8]+" cancelado")

	paid := paidOrder(t, b)
	if c := cancel(t, b, paid.ID, http.StatusOK); c.Payment == nil || c.Payment.Status != "refunded" || c.Payment.RefundID == "" {
		t.Errorf("cancel of a paid order: %+v", c.Payment)
	}
	if text := mailTo(t, devlogin.CustomerB, "Pedido "+paid.ID[:8]+" cancelado"); !strings.Contains(text, "estorno do pagamento foi feito") {
		t.Errorf("the cancellation notice does not tell the refund: %q", text)
	}

	// D03: without the gateway the cancellation holds and the refund waits;
	// a new call finishes it with the same idempotency key.
	pending := paidOrder(t, a)
	dockerCompose(t, "stop", "partnersim")
	t.Cleanup(func() { dockerCompose(t, "start", "partnersim") })
	if c := cancel(t, a, pending.ID, http.StatusAccepted); c.Status != "cancelled" || c.Payment == nil || c.Payment.Status != "refund_pending" {
		t.Errorf("cancel with the gateway down: %+v", c)
	}
	dockerCompose(t, "start", "partnersim")
	deadline := time.Now().Add(30 * time.Second)
	for {
		resp := apiJSON(t, http.MethodPost, "/api/v1/orders/"+pending.ID+"/cancel", a, "")
		var c cancelled
		decode(t, resp, &c)
		if resp.StatusCode == http.StatusOK && c.Payment != nil && c.Payment.Status == "refunded" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the refund did not finish: %d %+v", resp.StatusCode, c.Payment)
		}
		time.Sleep(2 * time.Second)
	}

	c := login(t, devlogin.CustomerC)
	production := paidOrder(t, c)
	wantStatus(t, panelDo(t, http.MethodPost, "/panel/v1/orders/"+production.ID+"/production", staff(t, devlogin.Technician), ""), http.StatusOK)
	wantStatus(t, apiJSON(t, http.MethodPost, "/api/v1/orders/"+production.ID+"/cancel", c, ""), http.StatusConflict)

	out := dockerCompose(t, "exec", "-T", "postgres-audit", "psql", "-U", "postgres", "-d", "audit", "-tAc",
		"SELECT reason FROM events WHERE name = 'order.cancelled' AND target = '"+paid.ID+"' AND actor = '"+devlogin.CustomerB.ID.String()+"'")
	if !strings.Contains(out, "from paid") || !strings.Contains(out, "changed my mind") {
		t.Errorf("order.cancelled reason %q", strings.TrimSpace(out))
	}
}

type ticketView struct {
	ID       string `json:"id"`
	Status   string `json:"status"`
	Messages []struct {
		AuthorKind string `json:"author_kind"`
		Body       string `json:"body"`
	} `json:"messages"`
}

// RF11: the customer talks to support about their own orders only, and only
// support and admin answer.
func TestSupportTickets(t *testing.T) {
	a, c := login(t, devlogin.CustomerA), login(t, devlogin.CustomerC)
	orderA := createOrder(t, a, processedFile(t, a, 5, 6, 7))

	wantStatus(t, apiJSON(t, http.MethodPost, "/api/v1/tickets", c,
		`{"order_id":"`+orderA.ID+`","subject":"Not mine","message":"e2e: trying someone else's order"}`), http.StatusForbidden)
	wantStatus(t, apiJSON(t, http.MethodPost, "/api/v1/tickets", c,
		`{"subject":"Control","message":"bell \u0007 inside"}`), http.StatusBadRequest)

	resp := apiJSON(t, http.MethodPost, "/api/v1/tickets", c, `{"subject":"Prazo","message":"e2e: <b>quando</b> chega?"}`)
	wantStatus(t, resp, http.StatusCreated)
	var tk ticketView
	decode(t, resp, &tk)
	wantStatus(t, apiJSON(t, http.MethodGet, "/api/v1/tickets/"+tk.ID, a, ""), http.StatusForbidden)

	ticketPath := "/panel/v1/tickets/" + tk.ID
	wantStatus(t, panelDo(t, http.MethodGet, "/panel/v1/tickets", staff(t, devlogin.Technician), ""), http.StatusForbidden)
	support := staff(t, devlogin.Support)
	resp = panelDo(t, http.MethodGet, "/panel/v1/tickets?status=open", support, "")
	wantStatus(t, resp, http.StatusOK)
	var open struct {
		Tickets []ticketView `json:"tickets"`
	}
	decode(t, resp, &open)
	if !slices.ContainsFunc(open.Tickets, func(x ticketView) bool { return x.ID == tk.ID }) {
		t.Errorf("support does not see ticket %s among the open ones", tk.ID)
	}
	wantStatus(t, panelDo(t, http.MethodPost, ticketPath+"/messages", support, `{"message":"e2e: chega em 5 dias úteis."}`), http.StatusOK)
	if text := mailTo(t, devlogin.CustomerC, "Resposta no chamado "+tk.ID[:8]); strings.Contains(text, "5 dias") {
		t.Errorf("the notice carries the answer: %q", text)
	}

	resp = apiJSON(t, http.MethodGet, "/api/v1/tickets/"+tk.ID, c, "")
	wantStatus(t, resp, http.StatusOK)
	decode(t, resp, &tk)
	if tk.Status != "answered" || len(tk.Messages) != 2 || tk.Messages[1].AuthorKind != "staff" || tk.Messages[0].Body != "e2e: <b>quando</b> chega?" {
		t.Errorf("the customer sees %+v", tk)
	}

	wantStatus(t, panelDo(t, http.MethodPost, ticketPath+"/close", support, ""), http.StatusOK)
	wantStatus(t, apiJSON(t, http.MethodPost, "/api/v1/tickets/"+tk.ID+"/messages", c, `{"message":"e2e: ainda aí?"}`), http.StatusConflict)
	wantStatus(t, panelDo(t, http.MethodPost, ticketPath+"/messages", support, `{"message":"e2e: fechado"}`), http.StatusConflict)
}

var addressCode = regexp.MustCompile(`código (\d{8})`)

// CA04 and C21: support alone cannot move a delivery. The change waits for
// the code sent to the registered e-mail, which only the owner, logged in,
// can type.
func TestAddressChangeNeedsTheCustomer(t *testing.T) {
	a, b := login(t, devlogin.CustomerE), login(t, devlogin.CustomerB)
	o := approvedOrder(t, a)
	support := staff(t, devlogin.Support)
	changePath := "/panel/v1/orders/" + o.ID + "/address-change"

	wantStatus(t, panelDo(t, http.MethodPost, changePath, staff(t, devlogin.Technician), newAddressBody), http.StatusForbidden)
	wantStatus(t, panelDo(t, http.MethodPost, changePath, support, strings.Replace(newAddressBody, `"justification":"e2e: customer asked for a new delivery address"`, `"justification":""`, 1)), 422)

	resp := panelDo(t, http.MethodPost, changePath, support, newAddressBody)
	wantStatus(t, resp, http.StatusAccepted)
	answer, _ := io.ReadAll(resp.Body)
	code := addressCode.FindStringSubmatch(mailTo(t, devlogin.CustomerE, "Confirme a troca de endereço do pedido "+o.ID[:8]))
	if code == nil {
		t.Fatal("no code in the confirmation e-mail")
	}
	if strings.Contains(string(answer), code[1]) {
		t.Fatal("the panel's answer carries the code")
	}
	if got := orderOf(t, a, o.ID); got.Shipping == nil || got.Shipping.PostalCode != "01310-100" {
		t.Fatalf("the address changed before the confirmation: %+v", got.Shipping)
	}

	// Paced to the nginx rate per IP, so the limiter does not answer instead
	// of the attempt counter.
	confirm := func(token, code string) *http.Response {
		time.Sleep(200 * time.Millisecond)
		return apiJSON(t, http.MethodPost, "/api/v1/orders/"+o.ID+"/address-change/confirm", token, `{"code":"`+code+`"}`)
	}
	wantStatus(t, confirm(b, code[1]), http.StatusForbidden)
	wrong := "00000000"
	if wrong == code[1] {
		wrong = "11111111"
	}
	for range 4 {
		wantStatus(t, confirm(a, wrong), http.StatusBadRequest)
	}
	wantStatus(t, confirm(a, wrong), http.StatusConflict)
	wantStatus(t, confirm(a, code[1]), http.StatusConflict)

	// A new proposal, and this time the customer types the right code.
	wantStatus(t, panelDo(t, http.MethodPost, changePath, support, newAddressBody), http.StatusAccepted)
	deadline := time.Now().Add(20 * time.Second)
	var fresh []string
	for {
		fresh = addressCode.FindStringSubmatch(mailTo(t, devlogin.CustomerE, "Confirme a troca de endereço do pedido "+o.ID[:8]))
		if fresh != nil && fresh[1] != code[1] {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("no new confirmation code")
		}
		time.Sleep(time.Second)
	}
	pendingResp := apiJSON(t, http.MethodGet, "/api/v1/orders/"+o.ID+"/address-change", a, "")
	wantStatus(t, pendingResp, http.StatusOK)
	wantStatus(t, confirm(a, fresh[1]), http.StatusOK)
	if got := orderOf(t, a, o.ID); got.Shipping == nil || got.Shipping.PostalCode != "01304-000" {
		t.Errorf("address after the confirmation: %+v", got.Shipping)
	}
	mailTo(t, devlogin.CustomerE, "Endereço do pedido "+o.ID[:8]+" alterado")

	out := dockerCompose(t, "exec", "-T", "postgres-audit", "psql", "-U", "postgres", "-d", "audit", "-tAc",
		"SELECT name, reason FROM events WHERE target = '"+o.ID+"' AND name LIKE 'address.%' ORDER BY occurred_at")
	if strings.Count(out, "address.change_failed") != 5 || !strings.Contains(out, "address.changed") ||
		!strings.Contains(out, "01310-***") || strings.Contains(out, "Augusta") || strings.Contains(out, "01304-000") {
		t.Errorf("address events:\n%s", out)
	}
}

// E8.5: the retention deletes the design 90 days after the delivery and the
// design never ordered after 30 days, clears the address of a closed order,
// and keeps what a live order still needs. The test moves the dates back in
// the database, as only a test may.
func TestRetention(t *testing.T) {
	b := login(t, devlogin.CustomerF)
	o := paidOrder(t, b)
	tech := staff(t, devlogin.Technician)
	wantStatus(t, panelDo(t, http.MethodPost, "/panel/v1/orders/"+o.ID+"/production", tech, ""), http.StatusOK)
	wantStatus(t, panelDo(t, http.MethodPost, "/panel/v1/orders/"+o.ID+"/ship", tech, ""), http.StatusAccepted)
	waitOrder(t, b, o.ID, "delivered")
	oldURL := downloadURL(t, b, o.FileID)

	unordered := processedFile(t, b, 3, 3, 3)
	live := submittedOrder(t, b, processedFile(t, b, 4, 4, 4))

	sql := func(q string) {
		dockerCompose(t, "exec", "-T", "postgres", "psql", "-U", "postgres", "-d", "nugget", "-v", "ON_ERROR_STOP=1", "-c", q)
	}
	sql("UPDATE order_status_history SET at = at - interval '91 days' WHERE order_id = '" + o.ID + "'")
	sql("UPDATE files SET created_at = created_at - interval '31 days' WHERE id IN ('" + unordered + "', '" + live.FileID + "')")
	dockerCompose(t, "run", "--rm", "retention", "-once")

	for _, id := range []string{o.FileID, unordered} {
		if st := status(t, b, id); st.Status != "purged" {
			t.Errorf("file %s is %q, want purged", id, st.Status)
		}
		wantStatus(t, do(t, http.MethodGet, base+"/api/v1/files/"+id+"/download", b, nil, -1), http.StatusGone)
	}
	wantStatus(t, do(t, http.MethodGet, oldURL, "", nil, -1), http.StatusNotFound)
	if st := status(t, b, live.FileID); st.Status != "done" {
		t.Errorf("the file of a live order is %q, want done", st.Status)
	}
	if got := orderOf(t, b, o.ID); got.Shipping != nil {
		t.Errorf("the delivered order still shows the address: %+v", got.Shipping)
	}
	out := dockerCompose(t, "exec", "-T", "postgres-audit", "psql", "-U", "postgres", "-d", "audit", "-tAc",
		"SELECT count(*) FROM events WHERE name = 'file.purged' AND target IN ('"+o.FileID+"', '"+unordered+"')")
	if strings.TrimSpace(out) != "2" {
		t.Errorf("file.purged events: %q, want 2", strings.TrimSpace(out))
	}
}

// E8.4: the notifier sees only the queue and the mail server, and each
// RabbitMQ user reaches only its own queues.
func TestNotifierIsIsolated(t *testing.T) {
	out := dockerCompose(t, "ps", "--format", "{{.Service}} {{.Networks}}")
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		service, networks, _ := strings.Cut(line, " ")
		onNotify := strings.Contains(networks, "nuggetmachine_notify")
		if service == "notifier" && networks != "nuggetmachine_notify" {
			t.Errorf("the notifier is on %s", networks)
		}
		if onNotify && !slices.Contains([]string{"notifier", "mailpit", "rabbitmq"}, service) {
			t.Errorf("%s is on the notify network", service)
		}
	}
	perms := dockerCompose(t, "exec", "-T", "rabbitmq", "rabbitmqctl", "list_permissions", "-q")
	for _, want := range []string{"notifier\t^$\t^$\t^notifications$", "panel\t^$\t^notifications$\t^$"} {
		if !strings.Contains(perms, want) {
			t.Errorf("RabbitMQ permissions without %q:\n%s", want, perms)
		}
	}
}

const grafanaBase = "https://localhost:9446"

// grafana calls the Grafana API as the local admin and decodes the answer.
func grafana(t *testing.T, method, path, body string, want int, out any) {
	t.Helper()
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, grafanaBase+path, r)
	if err != nil {
		t.Fatal(err)
	}
	req.SetBasicAuth("admin", readSecret(t, "grafana_admin_password"))
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != want {
		t.Fatalf("%s %s: status %d, want %d (%s)", method, path, resp.StatusCode, want, data)
	}
	if out != nil {
		if err := json.Unmarshal(data, out); err != nil {
			t.Fatalf("%s %s: %v", method, path, err)
		}
	}
}

// promQuery asks Prometheus through Grafana, which is the only way in: the
// Prometheus port is not published.
func promQuery(t *testing.T, query string) map[string]float64 {
	t.Helper()
	var out struct {
		Data struct {
			Result []struct {
				Metric map[string]string `json:"metric"`
				Value  [2]any            `json:"value"`
			} `json:"result"`
		} `json:"data"`
	}
	grafana(t, http.MethodGet, "/api/datasources/proxy/uid/prometheus/api/v1/query?query="+url.QueryEscape(query), "", http.StatusOK, &out)
	got := map[string]float64{}
	for _, r := range out.Data.Result {
		v, _ := strconv.ParseFloat(fmt.Sprint(r.Value[1]), 64)
		got[r.Metric["job"]] += v
	}
	return got
}

// C19: /metrics lives on the internal listener only.
func TestMetricsAreNotPublic(t *testing.T) {
	for _, u := range []string{base + "/metrics", base + "/api/v1/../metrics", panelBase + "/metrics", idp + "/metrics"} {
		resp := do(t, http.MethodGet, u, "", nil, -1)
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("GET %s: %d, want 404", u, resp.StatusCode)
		}
	}
}

// C19: Prometheus reaches every service, and the API's own counters move.
func TestPrometheusScrapesEverything(t *testing.T) {
	up := promQuery(t, "min by (job) (up)")
	for _, job := range []string{"api", "panel", "postgres", "postgres-audit", "valkey", "rabbitmq", "rabbitmq-queues", "garage", "keycloak", "nginx", "prometheus"} {
		if v, ok := up[job]; !ok || v != 1 {
			t.Errorf("job %s: up = %v (present %v), want 1", job, v, ok)
		}
	}
	const q = `sum by (job) (nugget_http_requests_total{job="api", route="/api/v1/catalog"})`
	before := promQuery(t, q)["api"]
	token := login(t, devlogin.CustomerE)
	for range 3 {
		wantStatus(t, apiJSON(t, http.MethodGet, "/api/v1/catalog", token, ""), http.StatusOK)
	}
	deadline := time.Now().Add(45 * time.Second)
	for promQuery(t, q)["api"] < before+3 {
		if time.Now().After(deadline) {
			t.Fatalf("the catalog counter stayed at %v after 3 requests", promQuery(t, q)["api"])
		}
		time.Sleep(3 * time.Second)
	}
}

// E7.4: Grafana needs its login, and shows the trail through a read-only role.
func TestGrafanaShowsTheAuditTrail(t *testing.T) {
	resp := do(t, http.MethodGet, grafanaBase+"/api/search", "", nil, -1)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("Grafana without login: %d, want 401", resp.StatusCode)
	}
	req, _ := http.NewRequest(http.MethodGet, grafanaBase+"/api/search", nil)
	req.SetBasicAuth("admin", "wrong-password")
	wrong, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	wrong.Body.Close()
	if wrong.StatusCode != http.StatusUnauthorized {
		t.Errorf("Grafana with a wrong password: %d, want 401", wrong.StatusCode)
	}

	var dashboards []struct {
		UID string `json:"uid"`
	}
	grafana(t, http.MethodGet, "/api/search?type=dash-db", "", http.StatusOK, &dashboards)
	var datasources []struct {
		UID string `json:"uid"`
	}
	grafana(t, http.MethodGet, "/api/datasources", "", http.StatusOK, &datasources)
	for _, uid := range []string{"nugget-security", "nugget-operations", "nugget-business"} {
		if !slices.ContainsFunc(dashboards, func(d struct {
			UID string `json:"uid"`
		}) bool {
			return d.UID == uid
		}) {
			t.Errorf("dashboard %s missing", uid)
		}
	}
	if len(datasources) != 3 {
		t.Errorf("%d datasources, want 3", len(datasources))
	}

	// The "denials per hour" panel of the security dashboard, as Grafana
	// runs it: the suite already produced denials.
	body := `{"from":"now-24h","to":"now","queries":[{"refId":"A","datasource":{"type":"grafana-postgresql-datasource","uid":"audit"},` +
		`"format":"table","rawQuery":true,"rawSql":"SELECT count(*) AS denials FROM events WHERE name = 'authz.denied' AND $__timeFilter(occurred_at)"}]}`
	var res struct {
		Results map[string]struct {
			Frames []struct {
				Data struct {
					Values [][]float64 `json:"values"`
				} `json:"data"`
			} `json:"frames"`
		} `json:"results"`
	}
	grafana(t, http.MethodPost, "/api/ds/query", body, http.StatusOK, &res)
	if frames := res.Results["A"].Frames; len(frames) == 0 || len(frames[0].Data.Values) == 0 || frames[0].Data.Values[0][0] < 1 {
		t.Errorf("no denials through Grafana: %+v", res)
	}
}

type alertRule struct {
	Name   string `json:"name"`
	Health string `json:"health"`
	Alerts []struct {
		Labels map[string]string `json:"labels"`
		State  string            `json:"state"`
	} `json:"alerts"`
}

func alertRules(t *testing.T) map[string]alertRule {
	t.Helper()
	var out struct {
		Data struct {
			Groups []struct {
				Rules []alertRule `json:"rules"`
			} `json:"groups"`
		} `json:"data"`
	}
	grafana(t, http.MethodGet, "/api/prometheus/grafana/api/v1/rules", "", http.StatusOK, &out)
	rules := map[string]alertRule{}
	for _, g := range out.Data.Groups {
		for _, r := range g.Rules {
			rules[r.Name] = r
		}
	}
	return rules
}

// waitAlert waits for the rule to fire for the account.
func waitAlert(t *testing.T, rule string, a devlogin.Account) {
	t.Helper()
	deadline := time.Now().Add(90 * time.Second)
	for {
		for _, alert := range alertRules(t)[rule].Alerts {
			if alert.Labels["actor"] == a.ID.String() && alert.State == "Alerting" {
				return
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("%q did not fire for %s: %+v", rule, a.Username, alertRules(t)[rule])
		}
		time.Sleep(5 * time.Second)
	}
}

// E7.5: the rules of Stage 6 are loaded and evaluate without errors.
func TestAlertRulesAreLoaded(t *testing.T) {
	rules := alertRules(t)
	for _, name := range []string{"Enumeração de IDs", "Abuso de upload por uma conta", "Acesso ao painel fora do horário",
		"Operações críticas em série no painel", "Webhooks com assinatura inválida", "Serviço fora do ar",
		"Erros 5xx acima de 5%", "Mensagens na fila de falhas"} {
		r, ok := rules[name]
		if !ok {
			t.Errorf("rule %q missing", name)
			continue
		}
		if r.Health == "error" {
			t.Errorf("rule %q: health %q", name, r.Health)
		}
	}
}

// E7.5, CA01: a script walking through IDs fires the enumeration alert for
// its account.
func TestEnumerationFiresTheAlert(t *testing.T) {
	token := login(t, devlogin.CustomerE)
	for range 15 {
		time.Sleep(150 * time.Millisecond)
		wantStatus(t, do(t, http.MethodGet, base+"/api/v1/files/"+uuid.NewString(), token, nil, -1), http.StatusForbidden)
	}
	waitAlert(t, "Enumeração de IDs", devlogin.CustomerE)
}

// E7.5, T19: invalid uploads in a row fire the upload abuse alert.
func TestUploadAbuseFiresTheAlert(t *testing.T) {
	token := login(t, devlogin.CustomerF)
	for i := range 10 {
		time.Sleep(150 * time.Millisecond)
		junk := []byte(fmt.Sprintf("not an STL %d", i))
		wantStatus(t, do(t, http.MethodPost, base+"/api/v1/files", token, bytes.NewReader(junk), int64(len(junk))), http.StatusUnprocessableEntity)
	}
	waitAlert(t, "Abuso de upload por uma conta", devlogin.CustomerF)
}

// E4.8: the worker runs under gVisor, unless the host has no gVisor and
// WORKER_RUNTIME says so.
func TestWorkerRunsUnderGVisor(t *testing.T) {
	want := "runsc"
	if v := os.Getenv("WORKER_RUNTIME"); v != "" {
		want = v
	}
	id := strings.TrimSpace(dockerCompose(t, "ps", "-q", "worker"))
	out, err := exec.Command("docker", "inspect", "--format", "{{.HostConfig.Runtime}}", id).CombinedOutput()
	if err != nil {
		t.Fatalf("docker inspect: %v: %s", err, out)
	}
	if got := strings.TrimSpace(string(out)); got != want {
		t.Errorf("worker runtime %q, want %q", got, want)
	}
}

// E3.4: the replica follows the primary, refuses writes and serves the
// customer's order list.
func TestReadReplica(t *testing.T) {
	if out := dockerCompose(t, "exec", "-T", "postgres", "psql", "-U", "postgres", "-tAc", "SELECT state FROM pg_stat_replication"); strings.TrimSpace(out) != "streaming" {
		t.Errorf("replication state %q, want streaming", strings.TrimSpace(out))
	}
	cmd := exec.Command("docker", "compose", "-f", compose, "exec", "-T", "postgres-replica",
		"psql", "-U", "postgres", "-d", "nugget", "-c", "CREATE TABLE replica_write_test (id int)")
	if out, err := cmd.CombinedOutput(); err == nil || !strings.Contains(string(out), "read-only") {
		t.Errorf("write on the replica: %v: %s", err, out)
	}

	token := login(t, devlogin.CustomerE)
	o := createOrder(t, token, processedFile(t, token, 6, 7, 8))
	deadline := time.Now().Add(10 * time.Second)
	for {
		resp := apiJSON(t, http.MethodGet, "/api/v1/orders", token, "")
		var list struct {
			Orders []orderView `json:"orders"`
		}
		decode(t, resp, &list)
		if slices.ContainsFunc(list.Orders, func(x orderView) bool { return x.ID == o.ID }) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("order %s not in the list read from the replica", o.ID)
		}
		time.Sleep(time.Second)
	}
}

// garageS3 reaches the buckets through the public Garage port with one of
// the platform's keys, as an attacker holding that key would.
func garageS3(t *testing.T, prefix string) *s3.Client {
	t.Helper()
	return s3.New(s3.Options{
		Region: "garage", BaseEndpoint: aws.String("https://localhost:8443"), UsePathStyle: true, HTTPClient: client,
		Credentials:                credentials.NewStaticCredentialsProvider(readSecret(t, prefix+"_access_key_id"), readSecret(t, prefix+"_secret_access_key"), ""),
		RequestChecksumCalculation: aws.RequestChecksumCalculationWhenRequired,
		ResponseChecksumValidation: aws.ResponseChecksumValidationWhenRequired,
	})
}

// E3.5 and E3.6: the backup is encrypted, only its own key reaches the
// bucket, and the latest backup restores with the expected counts.
func TestBackupAndRestore(t *testing.T) {
	dockerCompose(t, "run", "--rm", "backup", "-once")
	ctx := context.Background()

	_, err := garageS3(t, "s3").ListObjectsV2(ctx, &s3.ListObjectsV2Input{Bucket: aws.String("backups")})
	if err == nil || !strings.Contains(err.Error(), "403") {
		t.Errorf("the API key listing the backups: %v, want 403", err)
	}

	backups := garageS3(t, "s3_backup")
	list, err := backups.ListObjectsV2(ctx, &s3.ListObjectsV2Input{Bucket: aws.String("backups"), Prefix: aws.String("backups/")})
	if err != nil || len(list.Contents) < 4 {
		t.Fatalf("backups listed with the backup key: %v, %d objects", err, len(list.Contents))
	}
	obj, err := backups.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String("backups"), Key: list.Contents[len(list.Contents)-1].Key})
	if err != nil {
		t.Fatal(err)
	}
	head := make([]byte, 21)
	_, _ = io.ReadFull(obj.Body, head)
	obj.Body.Close()
	if string(head) != "age-encryption.org/v1" {
		t.Errorf("backup object starts with %q, want the age header", head)
	}

	t.Cleanup(func() { dockerCompose(t, "--profile", "restore", "rm", "-sf", "restore-db") })
	out := dockerCompose(t, "--profile", "restore", "run", "--rm", "restore")
	if !strings.Contains(out, "restore-test ok") {
		t.Errorf("restore-test output:\n%s", out)
	}
}

// C09 (FE0): every public answer carries the security headers once, and the
// API answers get the CSP of a JSON document.
func TestSecurityHeaders(t *testing.T) {
	common := map[string]string{
		"Strict-Transport-Security": "max-age=31536000",
		"X-Content-Type-Options":    "nosniff",
		"Referrer-Policy":           "no-referrer",
	}
	const jsonCSP = "default-src 'none'; frame-ancestors 'none'"
	for _, c := range []struct {
		url, csp string
	}{
		{base + "/", ""},
		{base + "/api/v1/catalog", jsonCSP},
		{base + "/api/v1/webhooks/payments", jsonCSP},
		{panelBase + "/panel/v1/me", jsonCSP},
		{idp + "/realms/nugget/", "-"},
		{"https://localhost:8443/files", jsonCSP},
	} {
		resp := do(t, http.MethodGet, c.url, "", nil, -1)
		for name, want := range common {
			if got := resp.Header.Values(name); len(got) != 1 || got[0] != want {
				t.Errorf("%s: %s = %q, want once %q", c.url, name, got, want)
			}
		}
		csp := resp.Header.Get("Content-Security-Policy")
		switch c.csp {
		case "":
			if !strings.Contains(csp, "script-src 'self'") || !strings.Contains(csp, "require-trusted-types-for 'script'") {
				t.Errorf("%s: app CSP %q", c.url, csp)
			}
		case "-":
		default:
			if csp != c.csp {
				t.Errorf("%s: CSP %q, want %q", c.url, csp, c.csp)
			}
		}
	}
}

// RF19: a part bigger than the printer shows up flagged in the review.
func TestDFMFlagInThePanel(t *testing.T) {
	customer := login(t, devlogin.CustomerA)
	o := createOrder(t, customer, processedFile(t, customer, 300, 10, 10))

	resp := panelDo(t, http.MethodGet, "/panel/v1/orders/"+o.ID, staff(t, devlogin.Admin), "")
	wantStatus(t, resp, http.StatusOK)
	var seen orderView
	decode(t, resp, &seen)
	if !slices.Contains(seen.DFMFlags, "exceeds_build_volume") {
		t.Errorf("DFM flags %v, want exceeds_build_volume", seen.DFMFlags)
	}
}

// The admin picks the technician from a list that Keycloak gives by role.
func TestTechniciansList(t *testing.T) {
	resp := panelDo(t, http.MethodGet, "/panel/v1/technicians", staff(t, devlogin.Admin), "")
	wantStatus(t, resp, http.StatusOK)
	var out struct {
		Technicians []struct {
			ID       string `json:"id"`
			Username string `json:"username"`
		} `json:"technicians"`
	}
	decode(t, resp, &out)
	names := map[string]string{}
	for _, u := range out.Technicians {
		names[u.Username] = u.ID
	}
	if names["technician"] != devlogin.Technician.ID.String() || names["technician-2"] != devlogin.Technician2.ID.String() {
		t.Errorf("technicians %v, want technician and technician-2", out.Technicians)
	}
	if _, ok := names["support"]; ok {
		t.Errorf("technicians %v include the support account", out.Technicians)
	}

	wantStatus(t, panelDo(t, http.MethodGet, "/panel/v1/technicians", staff(t, devlogin.Support), ""), http.StatusForbidden)
}

// E5.1: a logout in Keycloak revokes the access token in every API replica.
// The back-channel call reaches one replica; the revocation is in Valkey.
func TestLogoutRevokesTokenOnAllReplicas(t *testing.T) {
	dockerCompose(t, "up", "-d", "--no-recreate", "--scale", "api=3")
	// nginx re-resolves the replicas every 10s, so both scaling steps wait for
	// it: otherwise the next test could hit a replica that no longer exists.
	t.Cleanup(func() {
		dockerCompose(t, "up", "-d", "--no-recreate", "--scale", "api=1")
		time.Sleep(12 * time.Second)
	})
	time.Sleep(12 * time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), httpWait)
	defer cancel()
	tokens, err := keycloak.Login(ctx, devlogin.CustomerA)
	if err != nil {
		t.Fatal(err)
	}
	path := base + "/api/v1/files/" + uuid.NewString()
	wantStatus(t, do(t, http.MethodGet, path, tokens.Access, nil, -1), http.StatusForbidden)

	if err := keycloak.Logout(ctx, tokens.Refresh); err != nil {
		t.Fatal(err)
	}
	for i := range 12 {
		if resp := do(t, http.MethodGet, path, tokens.Access, nil, -1); resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("request %d after logout got %d, want 401", i+1, resp.StatusCode)
		}
		time.Sleep(150 * time.Millisecond)
	}
}

// D03: without Valkey the API cannot check revocations, so it refuses.
func TestValkeyDownFailsClosed(t *testing.T) {
	token := login(t, devlogin.CustomerA)
	path := base + "/api/v1/files/" + uuid.NewString()
	dockerCompose(t, "stop", "valkey")
	t.Cleanup(func() { dockerCompose(t, "start", "valkey") })

	wantStatus(t, do(t, http.MethodGet, path, token, nil, -1), http.StatusServiceUnavailable)

	dockerCompose(t, "start", "valkey")
	deadline := time.Now().Add(30 * time.Second)
	for {
		resp := do(t, http.MethodGet, path, token, nil, -1)
		if resp.StatusCode == http.StatusForbidden {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("the API still answers %d after Valkey came back", resp.StatusCode)
		}
		time.Sleep(time.Second)
	}
}

// The API's Valkey user reaches only its own keys and commands, and the
// default user is off.
func TestValkeyACL(t *testing.T) {
	password := readSecret(t, "valkey_api_password")
	for _, cmd := range [][]string{{"FLUSHALL"}, {"KEYS", "*"}, {"CONFIG", "GET", "*"}, {"SET", "other:key", "1"}} {
		args := append([]string{"exec", "-T", "-e", "REDISCLI_AUTH=" + password, "valkey", "valkey-cli", "--user", "api", "--no-auth-warning"}, cmd...)
		if out := composeOutput(args...); !strings.Contains(out, "NOPERM") {
			t.Errorf("%v as api: %q, want NOPERM", cmd, strings.TrimSpace(out))
		}
	}
	if out := composeOutput("exec", "-T", "valkey", "valkey-cli", "PING"); !strings.Contains(out, "NOAUTH") {
		t.Errorf("PING as default: %q, want NOAUTH", strings.TrimSpace(out))
	}
}

// composeOutput runs docker compose and returns the output even when the
// command fails, for checks that expect an error.
func composeOutput(args ...string) string {
	out, _ := exec.Command("docker", append([]string{"compose", "-f", compose}, args...)...).CombinedOutput()
	return string(out)
}

// The back-channel logout lives on the internal listener only.
func TestBackchannelLogoutIsNotPublic(t *testing.T) {
	resp := do(t, http.MethodPost, base+"/oidc/backchannel-logout", "", strings.NewReader("logout_token=x"), -1)
	wantStatus(t, resp, http.StatusNotFound)
}

// E5.2: one user is limited across replicas and IPs, while another user from
// the same IP is not. The pace stays below the per-IP limit of nginx. Runs
// near the end because customer B stays limited for a minute.
func TestUserRateLimit(t *testing.T) {
	limited, other := login(t, devlogin.CustomerB), login(t, devlogin.CustomerC)
	path := base + "/api/v1/files/" + uuid.NewString()

	var rejected *http.Response
	for range 70 {
		resp := do(t, http.MethodGet, path, limited, nil, -1)
		if resp.StatusCode == http.StatusTooManyRequests {
			rejected = resp
		}
		time.Sleep(120 * time.Millisecond)
	}
	if rejected == nil {
		t.Fatal("none of the 70 requests got 429")
	}
	var body struct {
		Code string `json:"code"`
	}
	decode(t, rejected, &body)
	if body.Code != "rate_limited" || rejected.Header.Get("Retry-After") == "" {
		t.Errorf("429 with code %q and Retry-After %q", body.Code, rejected.Header.Get("Retry-After"))
	}
	wantStatus(t, do(t, http.MethodGet, path, other, nil, -1), http.StatusForbidden)
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
