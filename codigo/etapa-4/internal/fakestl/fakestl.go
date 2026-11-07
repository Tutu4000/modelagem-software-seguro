// Package fakestl generates binary STLs in code for the tests and the seed:
// the environment never uses a real design (section 3.4).
package fakestl

import (
	"encoding/binary"
	"math"
)

// Box generates a closed x × y × z mm box with 12 outward-facing triangles.
func Box(x, y, z float32) []byte {
	p := [8][3]float32{
		{0, 0, 0}, {x, 0, 0}, {x, y, 0}, {0, y, 0},
		{0, 0, z}, {x, 0, z}, {x, y, z}, {0, y, z},
	}
	faces := [12][3]int{
		{0, 2, 1}, {0, 3, 2}, // bottom
		{4, 5, 6}, {4, 6, 7}, // top
		{0, 1, 5}, {0, 5, 4}, // front
		{2, 3, 7}, {2, 7, 6}, // back
		{1, 2, 6}, {1, 6, 5}, // right
		{3, 0, 4}, {3, 4, 7}, // left
	}
	b := make([]byte, 84+50*len(faces))
	copy(b, "fakestl: generated in code")
	binary.LittleEndian.PutUint32(b[80:84], uint32(len(faces)))
	for i, f := range faces {
		off := 84 + 50*i + 12 // the normal stays zeroed, as many exporters do
		for j, idx := range f {
			for k := range 3 {
				binary.LittleEndian.PutUint32(b[off+12*j+4*k:], math.Float32bits(p[idx][k]))
			}
		}
	}
	return b
}
