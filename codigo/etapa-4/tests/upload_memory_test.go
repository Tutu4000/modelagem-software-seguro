package tests

import (
	"bytes"
	"encoding/binary"
	"io"
	"net/http"
	"sync"
	"testing"
)

// stlHeader declares n triangles and nothing else.
func stlHeader(n uint32) []byte {
	h := make([]byte, 84)
	binary.LittleEndian.PutUint32(h[80:], n)
	return h
}

// Parallel uploads may not hold more memory than the API's budget: the one
// that does not fit is refused before its body is read.
func TestParallelUploadsAreBoundedInMemory(t *testing.T) {
	e := newEnv(t)
	token := e.token(newUser())
	const size = 84 + 50*maxTriangles // about 100 MB each

	// Two uploads declare a full-size STL and stall in the middle of the body,
	// holding their share of the budget.
	var stalled sync.WaitGroup
	var writers []*io.PipeWriter
	for range 2 {
		r, w := io.Pipe()
		writers = append(writers, w)
		stalled.Add(1)
		go func() {
			defer stalled.Done()
			e.upload(token, io.MultiReader(bytes.NewReader(stlHeader(maxTriangles)), r), size)
		}()
		// The write only returns when the handler is reading the body, that
		// is, after it took its share.
		if _, err := w.Write([]byte{0}); err != nil {
			t.Fatal(err)
		}
	}

	rec := e.upload(token, bytes.NewReader(stlHeader(maxTriangles)), -1)
	wantStatus(t, rec, http.StatusServiceUnavailable)
	if code := problemCode(t, rec); code != "unavailable" {
		t.Errorf("code %q, want \"unavailable\"", code)
	}

	for _, w := range writers {
		w.Close()
	}
	stalled.Wait()

	// The budget comes back when the stalled uploads end.
	model := binarySTL(12)
	wantStatus(t, e.upload(token, bytes.NewReader(model), int64(len(model))), http.StatusAccepted)
}

// A body that is not the size its header declares is refused from the
// Content-Length, without being read.
func TestMismatchedContentLengthIsRefusedWithoutReading(t *testing.T) {
	e := newEnv(t)
	body := &countingReader{r: io.MultiReader(bytes.NewReader(stlHeader(12)), io.LimitReader(zeros{}, 50<<20))}

	rec := e.upload(e.token(newUser()), body, 84+50<<20)

	wantStatus(t, rec, http.StatusUnprocessableEntity)
	if code := problemCode(t, rec); code != "invalid_stl" {
		t.Errorf("code %q, want \"invalid_stl\"", code)
	}
	if body.n > 4096 {
		t.Errorf("the API read %d bytes of a body it could refuse from the header", body.n)
	}
	e.nothingStored()
}

type countingReader struct {
	r io.Reader
	n int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}
