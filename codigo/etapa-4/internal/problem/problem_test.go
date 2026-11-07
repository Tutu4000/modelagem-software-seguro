package problem_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"nuggetmachine/internal/problem"
)

func init() { gin.SetMode(gin.TestMode) }

type details struct {
	Type   string `json:"type"`
	Title  string `json:"title"`
	Status int    `json:"status"`
	Code   string `json:"code"`
}

func respond(t *testing.T, err error) (*httptest.ResponseRecorder, details) {
	t.Helper()
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	problem.Abort(c, err)

	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/problem+json") {
		t.Fatalf("Content-Type %q, want application/problem+json", ct)
	}
	var d details
	if err := json.Unmarshal(rec.Body.Bytes(), &d); err != nil {
		t.Fatalf("the response is not JSON: %q", rec.Body.String())
	}
	if d.Type == "" || d.Title == "" || d.Status != rec.Code {
		t.Errorf("incomplete Problem Details: %s", rec.Body.String())
	}
	return rec, d
}

func TestKnownErrorBecomesProblemDetailsWithoutCause(t *testing.T) {
	cause := errors.New("size 151 differs from 84 + 50 * 2019914866")

	rec, d := respond(t, problem.New(problem.InvalidSTL, cause))

	if rec.Code != http.StatusUnprocessableEntity || d.Code != "invalid_stl" {
		t.Errorf("status %d and code %q, want 422 and \"invalid_stl\"", rec.Code, d.Code)
	}
	if strings.Contains(rec.Body.String(), "2019914866") {
		t.Error("the internal cause appeared in the response")
	}
}

func TestWrappedErrorKeepsTheCode(t *testing.T) {
	err := fmt.Errorf("file upload: %w", problem.New(problem.FileTooLarge, nil))

	rec, d := respond(t, err)

	if rec.Code != http.StatusRequestEntityTooLarge || d.Code != "file_too_large" {
		t.Errorf("status %d and code %q, want 413 and \"file_too_large\"", rec.Code, d.Code)
	}
}

func TestUnknownErrorBecomesInternalErrorWithoutDetails(t *testing.T) {
	rec, d := respond(t, errors.New("failed to connect to postgres://app:password@db:5432"))

	if rec.Code != http.StatusInternalServerError || d.Code != "internal" {
		t.Errorf("status %d and code %q, want 500 and \"internal\"", rec.Code, d.Code)
	}
	if strings.Contains(rec.Body.String(), "password") {
		t.Error("the internal cause appeared in the response")
	}
}
