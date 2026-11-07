package tests

// Processing status route: same authorization as the download (RS01).

import (
	"bytes"
	"net/http"
	"testing"
)

func TestCustomerSeesStatusOfOwnFileOnly(t *testing.T) {
	e := newEnv(t)
	a, b := newUser(), newUser()

	rec := e.upload(e.token(a), bytes.NewReader(binarySTL(12)), -1)
	wantStatus(t, rec, http.StatusAccepted)
	var created struct {
		ID string `json:"id"`
	}
	decode(t, rec, &created)
	path := "/api/v1/files/" + created.ID

	rec = e.get(path, e.token(a))
	wantStatus(t, rec, http.StatusOK)
	var st struct {
		Status string `json:"status"`
	}
	decode(t, rec, &st)
	if st.Status != "processing" {
		t.Errorf("status %q, want \"processing\"", st.Status)
	}

	rec = e.get(path, e.token(b))
	wantStatus(t, rec, http.StatusForbidden)
	if code := problemCode(t, rec); code != "forbidden" {
		t.Errorf("code %q, want \"forbidden\"", code)
	}
	if denied := e.events("authz.denied"); len(denied) != 1 || denied[0]["action"] != "file.status" {
		t.Errorf("unexpected authz.denied events: %v", denied)
	}
}

func TestRejectedUploadCreatesAuditEvent(t *testing.T) {
	e := newEnv(t)
	a := newUser()

	wantStatus(t, e.upload(e.token(a), bytes.NewReader([]byte("not an STL")), -1), http.StatusUnprocessableEntity)

	rejected := e.events("upload.rejected")
	if len(rejected) != 1 || rejected[0]["actor"] != a.String() || rejected[0]["reason"] != "invalid_stl" {
		t.Errorf("unexpected upload.rejected events: %v", rejected)
	}
}
