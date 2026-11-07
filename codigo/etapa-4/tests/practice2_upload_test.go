package tests

// Practice 2 (RS03, D03). The IDs follow the table in docs/etapa-4-codigo-seguro.md.

import (
	"bytes"
	"crypto/sha256"
	"io"
	"net/http"
	"strings"
	"testing"
)

// Fixed here, not read from stl, so that the test checks the requirement.
const (
	maxSize      = 100 << 20
	maxTriangles = 2_000_000
)

func TestTS06_ValidSTLUploadCreatesATask(t *testing.T) {
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
		t.Errorf("status %q, want \"processing\" (D03)", body.Status)
	}
	tasks := e.queue.Tasks()
	if len(tasks) != 1 {
		t.Fatalf("%d tasks in the queue, want 1", len(tasks))
	}
	task := tasks[0]
	if task.FileID.String() != body.ID || task.Owner != a || task.Triangles != 12 || task.SHA256 != sha256.Sum256(model) {
		t.Errorf("unexpected task: %+v", task)
	}
	if e.repo.Count() != 1 || e.store.Count() != 1 {
		t.Error("the file was not stored")
	}
}

func TestTS07_FileAbove100MBGets413(t *testing.T) {
	const size = maxSize + 50<<20 // 150 MB
	cases := []struct {
		name     string
		declared int64
	}{
		{"with declared Content-Length", size},
		{"without Content-Length", -1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t)
			rec := e.upload(e.token(newUser()), io.LimitReader(zeros{}, size), tc.declared)
			wantStatus(t, rec, http.StatusRequestEntityTooLarge)
			if code := problemCode(t, rec); code != "file_too_large" {
				t.Errorf("code %q, want \"file_too_large\"", code)
			}
			e.nothingStored()
		})
	}
}

func TestTS08_FileThatIsNotBinarySTLGets422(t *testing.T) {
	valid := binarySTL(12)
	cases := []struct {
		name string
		body []byte
	}{
		{"PDF sent as STL", []byte("%PDF-1.7\n" + strings.Repeat("1 0 obj << /Type /Page >> endobj\n", 4) + "%%EOF\n")},
		{"text STL (ASCII)", []byte("solid cube\n facet normal 0 0 1\n  outer loop\n   vertex 0 0 0\n   vertex 1 0 0\n   vertex 0 1 0\n  endloop\n endfacet\nendsolid cube\n")},
		{"truncated STL", valid[:len(valid)-10]},
		{"STL with trailing bytes", append(append([]byte{}, valid...), 1, 2, 3)},
		{"file smaller than the header", []byte("short")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t)
			rec := e.upload(e.token(newUser()), bytes.NewReader(tc.body), int64(len(tc.body)))
			wantStatus(t, rec, http.StatusUnprocessableEntity)
			if code := problemCode(t, rec); code != "invalid_stl" {
				t.Errorf("code %q, want \"invalid_stl\"", code)
			}
			e.nothingStored()
		})
	}
}

func TestTS09_STLWithMoreThan2MillionTrianglesGets422(t *testing.T) {
	e := newEnv(t)
	body, size := binarySTLStream(maxTriangles + 1)
	if size > maxSize {
		t.Fatalf("the test file has %d bytes and would exceed the size limit", size)
	}

	rec := e.upload(e.token(newUser()), body, size)

	wantStatus(t, rec, http.StatusUnprocessableEntity)
	if code := problemCode(t, rec); code != "too_many_triangles" {
		t.Errorf("code %q, want \"too_many_triangles\"", code)
	}
	e.nothingStored()
}
