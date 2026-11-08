package tests

// Request log (E7.3, C17): the internal cause goes to the log, but the token
// never does.

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestRequestLogHasCauseAndNoToken(t *testing.T) {
	e := newEnv(t)
	a, b := newUser(), newUser()
	fileB := e.seedFile(b, []byte("confidential design of customer B"))
	valid := e.token(a)
	expired := sign(t, e.key, e.claims(a, -time.Minute))

	wantStatus(t, e.get(downloadPath(fileB.ID), valid), http.StatusForbidden)
	wantStatus(t, e.get(downloadPath(fileB.ID), expired), http.StatusUnauthorized)
	wantStatus(t, e.upload(valid, bytes.NewReader(binarySTL(1)), -1), http.StatusAccepted)

	lines := strings.Split(strings.TrimSpace(e.app.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("%d lines in the log, want 3: %s", len(lines), e.app.String())
	}
	var unauthorized map[string]any
	if err := json.Unmarshal([]byte(lines[1]), &unauthorized); err != nil {
		t.Fatalf("log line is not JSON: %q", lines[1])
	}
	if unauthorized["status"] != float64(http.StatusUnauthorized) || !strings.Contains(lines[1], "invalid token") {
		t.Errorf("the 401 log line has no cause: %s", lines[1])
	}

	for _, token := range []string{valid, expired} {
		if strings.Contains(e.app.String(), token) {
			t.Error("the token appeared in the request log (C17)")
		}
	}
	if strings.Contains(strings.ToLower(e.app.String()), "bearer") {
		t.Error("the Authorization header appeared in the request log")
	}
}
