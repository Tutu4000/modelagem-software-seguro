package tests

// Practice 1 (RS01, D01). The IDs follow the table in docs/etapa-4-codigo-seguro.md.

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"nuggetmachine/internal/files"
)

func TestTS01_CustomerDownloadsOwnSTL(t *testing.T) {
	e := newEnv(t)
	a := newUser()
	content := []byte("design of customer A")
	f := e.seedFile(a, content)

	rec := e.fetch(e.downloadURL(a, f))

	wantStatus(t, rec, http.StatusOK)
	if !bytes.Equal(rec.Body.Bytes(), content) {
		t.Fatal("the URL did not deliver the customer's file")
	}
}

func TestTS02_CustomerCannotDownloadAnotherCustomersSTL(t *testing.T) {
	e := newEnv(t)
	a, b := newUser(), newUser()
	fileB := e.seedFile(b, []byte("confidential design of customer B"))
	missing := files.NewFileID()
	tokenA := e.token(a)

	cases := []struct {
		name   string
		path   string
		target files.FileID
	}{
		{"another customer's file", downloadPath(fileB.ID), fileB.ID},
		{"another customer's user_id in the URL is ignored", downloadPath(fileB.ID) + "?user_id=" + b.String(), fileB.ID},
		{"missing file gets the same response", downloadPath(missing), missing},
	}
	bodies := map[string]string{}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := e.get(tc.path, tokenA)
			wantStatus(t, rec, http.StatusForbidden)
			if code := problemCode(t, rec); code != "forbidden" {
				t.Errorf("code %q, want \"forbidden\"", code)
			}
			var body map[string]any
			decode(t, rec, &body)
			if _, ok := body["url"]; ok {
				t.Fatal("the denied response carried a URL")
			}
			bodies[tc.name] = rec.Body.String()
		})
	}
	if bodies[cases[0].name] != bodies[cases[2].name] {
		t.Error("another customer's file and a missing file got different responses, which reveals which IDs exist")
	}

	denied := e.events("authz.denied")
	if len(denied) != len(cases) {
		t.Fatalf("%d authz.denied events, want %d", len(denied), len(cases))
	}
	for i, ev := range denied {
		if ev["actor"] != a.String() || ev["target"] != cases[i].target.String() || ev["result"] != "denied" {
			t.Errorf("unexpected event for %q: %v", cases[i].name, ev)
		}
	}
	if strings.Contains(e.audit.String(), tokenA) {
		t.Error("the token appeared in the log (C17)")
	}
}

func TestTS03_RequestWithoutValidTokenGets401(t *testing.T) {
	e := newEnv(t)
	a, b := newUser(), newUser()
	fileB := e.seedFile(b, []byte("confidential design of customer B"))

	_, otherKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	unsigned, err := jwt.NewWithClaims(jwt.SigningMethodNone, e.claims(b, 15*time.Minute)).
		SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name  string
		token string
	}{
		{"no token", ""},
		{"malformed token", "this-is-not-a-jwt"},
		{"customer B token signed by another key", sign(t, otherKey, e.claims(b, 15*time.Minute))},
		{"expired customer B token", sign(t, e.key, e.claims(b, -time.Minute))},
		{"unsigned token (alg none)", unsigned},
		{"customer A token with the user swapped to B", swapSubject(t, e.token(a), b)},
		{"token from another issuer", sign(t, e.key, with(e.claims(b, 15*time.Minute), "iss", "https://evil.test/realms/nugget"))},
		{"token issued for another audience", sign(t, e.key, with(e.claims(b, 15*time.Minute), "aud", []string{"panel"}))},
		{"ID token instead of an access token", sign(t, e.key, with(e.claims(b, 15*time.Minute), "typ", "ID"))},
		{"token without an application role", sign(t, e.key, with(e.claims(b, 15*time.Minute), "realm_access", map[string]any{"roles": []string{"offline_access"}}))},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := e.get(downloadPath(fileB.ID), tc.token)
			wantStatus(t, rec, http.StatusUnauthorized)
			if code := problemCode(t, rec); code != "unauthenticated" {
				t.Errorf("code %q, want \"unauthenticated\"", code)
			}
		})
	}
}

// A valid staff token is authenticated, but the public API only serves
// customers (D02): 403, not 401.
func TestStaffTokenGets403OnCustomerAPI(t *testing.T) {
	e := newEnv(t)
	tech := newUser()
	fileB := e.seedFile(newUser(), []byte("confidential design of customer B"))
	staff := with(e.claims(tech, 15*time.Minute), "realm_access", map[string]any{"roles": []string{"technician"}})

	rec := e.get(downloadPath(fileB.ID), sign(t, e.key, staff))

	wantStatus(t, rec, http.StatusForbidden)
	if code := problemCode(t, rec); code != "forbidden" {
		t.Errorf("code %q, want \"forbidden\"", code)
	}
}

func TestTS04_PresignedURLExpiresIn5Minutes(t *testing.T) {
	e := newEnv(t)
	a := newUser()
	signed := e.downloadURL(a, e.seedFile(a, []byte("design of customer A")))

	e.clock.Advance(4*time.Minute + 59*time.Second)
	wantStatus(t, e.fetch(signed), http.StatusOK)

	e.clock.Advance(2 * time.Second)
	wantStatus(t, e.fetch(signed), http.StatusForbidden)
}

func TestTS05_TamperedURLGets403(t *testing.T) {
	e := newEnv(t)
	a, b := newUser(), newUser()
	fileA := e.seedFile(a, []byte("design of customer A"))
	fileB := e.seedFile(b, []byte("confidential design of customer B"))
	signed, err := url.Parse(e.downloadURL(a, fileA))
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name   string
		change func(u *url.URL)
	}{
		{"direct access, no signature", func(u *url.URL) {
			u.RawQuery = ""
		}},
		{"altered signature", func(u *url.URL) {
			q := u.Query()
			q.Set("sig", flipLast(q.Get("sig")))
			u.RawQuery = q.Encode()
		}},
		{"extended expiry", func(u *url.URL) {
			q := u.Query()
			exp, _ := strconv.ParseInt(q.Get("exp"), 10, 64)
			q.Set("exp", strconv.FormatInt(exp+3600, 10))
			u.RawQuery = q.Encode()
		}},
		{"signature of file A used on file B", func(u *url.URL) {
			u.Path = strings.Replace(u.Path, fileA.ID.String(), fileB.ID.String(), 1)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			u := *signed
			tc.change(&u)
			rec := e.fetch(u.String())
			wantStatus(t, rec, http.StatusForbidden)
			if bytes.Contains(rec.Body.Bytes(), []byte("design")) {
				t.Fatal("the storage delivered content")
			}
		})
	}
}

func flipLast(sig string) string {
	if sig == "" {
		return "0"
	}
	repl := "0"
	if strings.HasSuffix(sig, "0") {
		repl = "1"
	}
	return sig[:len(sig)-1] + repl
}
