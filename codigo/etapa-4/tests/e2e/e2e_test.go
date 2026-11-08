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
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

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
		{"panel_orders cannot read files", "postgres", "nugget", "panel_orders", "db_panel_orders_password", "SELECT 1 FROM files LIMIT 1"},
		{"panel_orders cannot mark a payment", "postgres", "nugget", "panel_orders", "db_panel_orders_password", "UPDATE payments SET status = 'succeeded'"},
		{"orders cannot change a payment amount", "postgres", "nugget", "orders", "db_orders_password", "UPDATE payments SET amount_cents = 1"},
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
	want = []string{"order.in_production", "payment.requested", "payment.succeeded", "shipment.delivered", "shipment.picked_up", "shipment.requested"}
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
}

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
