// Package stl valida STL binários antes da gravação (RS03).
package stl

import (
	"encoding/binary"
	"fmt"
)

const (
	MaxSize      = 100 << 20 // 100 MiB: cabe um STL com MaxTriangles triângulos
	MaxTriangles = 2_000_000

	headerSize   = 84
	triangleSize = 50
)

type FormatError struct {
	Size      int
	Triangles uint32 // declarados no cabeçalho
}

func (e *FormatError) Error() string {
	if e.Size < headerSize {
		return fmt.Sprintf("arquivo de %d bytes, menor que o cabeçalho de um STL binário", e.Size)
	}
	return fmt.Sprintf("arquivo de %d bytes, mas o cabeçalho declara %d triângulos (%d bytes)",
		e.Size, e.Triangles, expectedSize(e.Triangles))
}

type TooManyTrianglesError struct {
	Triangles uint32
}

func (e *TooManyTrianglesError) Error() string {
	return fmt.Sprintf("STL com %d triângulos, acima do limite de %d", e.Triangles, MaxTriangles)
}

// Validated só sai do Parse, e só ele vira tarefa.
type Validated struct {
	triangles uint32
}

func (v Validated) Triangles() uint32 { return v.triangles }

// Parse confere o tamanho antes do limite de triângulos: um arquivo que não é
// STL declara um número qualquer no cabeçalho.
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
