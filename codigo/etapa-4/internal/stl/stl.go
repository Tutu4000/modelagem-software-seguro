// Package stl validates binary STLs before they are stored (RS03).
package stl

import (
	"encoding/binary"
	"fmt"
)

const (
	MaxSize      = 100 << 20 // 100 MiB: fits an STL with MaxTriangles triangles
	MaxTriangles = 2_000_000

	headerSize   = 84
	triangleSize = 50
)

type FormatError struct {
	Size      int
	Triangles uint32 // declared in the header
}

func (e *FormatError) Error() string {
	if e.Size < headerSize {
		return fmt.Sprintf("file of %d bytes, smaller than a binary STL header", e.Size)
	}
	return fmt.Sprintf("file of %d bytes, but the header declares %d triangles (%d bytes)",
		e.Size, e.Triangles, expectedSize(e.Triangles))
}

type TooManyTrianglesError struct {
	Triangles uint32
}

func (e *TooManyTrianglesError) Error() string {
	return fmt.Sprintf("STL with %d triangles, above the limit of %d", e.Triangles, MaxTriangles)
}

// Validated only comes out of Parse, and only it becomes a task.
type Validated struct {
	triangles uint32
}

func (v Validated) Triangles() uint32 { return v.triangles }

// Parse checks the size before the triangle limit: a file that is not an STL
// declares an arbitrary number in the header.
func Parse(data []byte) (Validated, error) {
	if len(data) < headerSize {
		return Validated{}, &FormatError{Size: len(data)}
	}
	n := binary.LittleEndian.Uint32(data[80:headerSize])
	if int64(len(data)) != expectedSize(n) {
		return Validated{}, &FormatError{Size: len(data), Triangles: n}
	}
	if n > MaxTriangles {
		return Validated{}, &TooManyTrianglesError{Triangles: n}
	}
	return Validated{triangles: n}, nil
}

func expectedSize(n uint32) int64 { return headerSize + triangleSize*int64(n) }
