package tests

// Prática 2 (RS03, D03). Os IDs seguem a tabela de docs/etapa-4-codigo-seguro.md.

import (
	"bytes"
	"crypto/sha256"
	"io"
	"net/http"
	"strings"
	"testing"
)

// Fixos aqui, e não lidos de stl, para o teste conferir o requisito.
const (
	maxSize      = 100 << 20
	maxTriangles = 2_000_000
)

func TestTS06_UploadDeSTLValidoCriaUmaTarefa(t *testing.T) {
	e := newEnv(t)
	a := newUser()
	model := binarySTL(12)

	rec := e.upload(e.token(a), bytes.NewReader(model), int64(len(model)))

	wantStatus(t, rec, http.StatusAccepted)
	var body struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	decode(t, rec, &body)
	if body.Status != "processing" {
		t.Errorf("status %q, esperado \"processing\" (em processamento, D03)", body.Status)
	}
	tasks := e.queue.Tasks()
	if len(tasks) != 1 {
		t.Fatalf("%d tarefas na fila, esperado 1", len(tasks))
	}
	task := tasks[0]
	if task.FileID.String() != body.ID || task.Owner != a || task.Triangles != 12 || task.SHA256 != sha256.Sum256(model) {
		t.Errorf("tarefa inesperada: %+v", task)
	}
	if e.repo.Count() != 1 || e.store.Count() != 1 {
		t.Error("o arquivo não foi gravado")
	}
}

func TestTS07_ArquivoAcimaDe100MBRecebe413(t *testing.T) {
	const size = maxSize + 50<<20 // 150 MB
	cases := []struct {
		name     string
		declared int64
	}{
		{"com Content-Length declarado", size},
		{"sem Content-Length", -1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t)
			rec := e.upload(e.token(newUser()), io.LimitReader(zeros{}, size), tc.declared)
			wantStatus(t, rec, http.StatusRequestEntityTooLarge)
			if code := errorCode(t, rec); code != "file_too_large" {
				t.Errorf("erro %q, esperado \"file_too_large\"", code)
			}
			e.nothingStored()
		})
	}
}

func TestTS08_ArquivoQueNaoEhSTLBinarioRecebe422(t *testing.T) {
	valid := binarySTL(12)
	cases := []struct {
		name string
		body []byte
	}{
		{"PDF enviado como STL", []byte("%PDF-1.7\n" + strings.Repeat("1 0 obj << /Type /Page >> endobj\n", 4) + "%%EOF\n")},
		{"STL em texto (ASCII)", []byte("solid cubo\n facet normal 0 0 1\n  outer loop\n   vertex 0 0 0\n   vertex 1 0 0\n   vertex 0 1 0\n  endloop\n endfacet\nendsolid cubo\n")},
		{"STL truncado", valid[:len(valid)-10]},
		{"STL com bytes sobrando", append(append([]byte{}, valid...), 1, 2, 3)},
		{"arquivo menor que o cabeçalho", []byte("curto")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t)
			rec := e.upload(e.token(newUser()), bytes.NewReader(tc.body), int64(len(tc.body)))
			wantStatus(t, rec, http.StatusUnprocessableEntity)
			if code := errorCode(t, rec); code != "invalid_stl" {
				t.Errorf("erro %q, esperado \"invalid_stl\"", code)
			}
			e.nothingStored()
		})
	}
}

func TestTS09_STLComMaisDe2MilhoesDeTriangulosRecebe422(t *testing.T) {
	e := newEnv(t)
	body, size := binarySTLStream(maxTriangles + 1)
	if size > maxSize {
		t.Fatalf("o arquivo de teste tem %d bytes e passaria do limite de tamanho", size)
	}

	rec := e.upload(e.token(newUser()), body, size)

	wantStatus(t, rec, http.StatusUnprocessableEntity)
	if code := errorCode(t, rec); code != "too_many_triangles" {
		t.Errorf("erro %q, esperado \"too_many_triangles\"", code)
	}
	e.nothingStored()
}
