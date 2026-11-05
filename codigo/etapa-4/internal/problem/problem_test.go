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
		t.Fatalf("Content-Type %q, esperado application/problem+json", ct)
	}
	var d details
	if err := json.Unmarshal(rec.Body.Bytes(), &d); err != nil {
		t.Fatalf("a resposta não é JSON: %q", rec.Body.String())
	}
	if d.Type == "" || d.Title == "" || d.Status != rec.Code {
		t.Errorf("Problem Details incompleto: %s", rec.Body.String())
	}
	return rec, d
}

func TestErroConhecidoViraProblemDetailsSemACausa(t *testing.T) {
	cause := errors.New("tamanho 151 diferente de 84 + 50 * 2019914866")

	rec, d := respond(t, problem.New(problem.InvalidSTL, cause))

	if rec.Code != http.StatusUnprocessableEntity || d.Code != "invalid_stl" {
		t.Errorf("status %d e code %q, esperado 422 e \"invalid_stl\"", rec.Code, d.Code)
	}
	if strings.Contains(rec.Body.String(), "2019914866") {
		t.Error("a causa interna apareceu na resposta")
	}
}

func TestErroEmbrulhadoMantemOCodigo(t *testing.T) {
	err := fmt.Errorf("upload do arquivo: %w", problem.New(problem.FileTooLarge, nil))

	rec, d := respond(t, err)

	if rec.Code != http.StatusRequestEntityTooLarge || d.Code != "file_too_large" {
		t.Errorf("status %d e code %q, esperado 413 e \"file_too_large\"", rec.Code, d.Code)
	}
}

func TestErroDesconhecidoViraErroInternoSemDetalhes(t *testing.T) {
	rec, d := respond(t, errors.New("falha ao conectar em postgres://app:senha@db:5432"))

	if rec.Code != http.StatusInternalServerError || d.Code != "internal" {
		t.Errorf("status %d e code %q, esperado 500 e \"internal\"", rec.Code, d.Code)
	}
	if strings.Contains(rec.Body.String(), "senha") {
		t.Error("a causa interna apareceu na resposta")
	}
}
