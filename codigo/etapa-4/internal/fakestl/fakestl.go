// Package fakestl gera STLs binários em código, para os testes e o seed: o
// ambiente não usa nenhum projeto real (seção 3.4).
package fakestl

import (
	"encoding/binary"
	"math"
)

// Box gera um paralelepípedo fechado de x × y × z mm, com 12 triângulos
// orientados para fora.
func Box(x, y, z float32) []byte {
	p := [8][3]float32{
		{0, 0, 0}, {x, 0, 0}, {x, y, 0}, {0, y, 0},
		{0, 0, z}, {x, 0, z}, {x, y, z}, {0, y, z},
	}
	faces := [12][3]int{
		{0, 2, 1}, {0, 3, 2}, // base
		{4, 5, 6}, {4, 6, 7}, // topo
		{0, 1, 5}, {0, 5, 4}, // frente
		{2, 3, 7}, {2, 7, 6}, // fundo
		{1, 2, 6}, {1, 6, 5}, // direita
		{3, 0, 4}, {3, 4, 7}, // esquerda
	}
	b := make([]byte, 84+50*len(faces))
	copy(b, "fakestl: gerado em codigo")
	binary.LittleEndian.PutUint32(b[80:84], uint32(len(faces)))
	for i, f := range faces {
		off := 84 + 50*i + 12 // a normal fica zerada, como muitos exportadores fazem
		for j, idx := range f {
			for k := range 3 {
				binary.LittleEndian.PutUint32(b[off+12*j+4*k:], math.Float32bits(p[idx][k]))
			}
		}
	}
	return b
}
