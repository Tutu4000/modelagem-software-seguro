// Package processing é a lógica do worker: confere o arquivo e simula o
// fatiamento com o volume e a caixa envolvente (E4.4).
package processing

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"math"

	"nuggetmachine/internal/stl"
)

// ErrHashMismatch indica que o arquivo baixado não é o que a API validou.
var ErrHashMismatch = errors.New("o SHA-256 do arquivo não confere")

type Result struct {
	VolumeMM3 float64
	BoxMM     [3]float64
}

// checkEvery é de quantos em quantos triângulos o prazo é conferido.
const checkEvery = 100_000

// Analyze confere o hash antes de tudo e passa de novo pelo stl.Parse: o worker
// não confia no que vem da fila.
func Analyze(ctx context.Context, data []byte, want [32]byte) (Result, error) {
	got := sha256.Sum256(data)
	if !bytes.Equal(got[:], want[:]) {
		return Result{}, ErrHashMismatch
	}
	model, err := stl.Parse(data)
	if err != nil {
		return Result{}, err
	}

	n := int(model.Triangles())
	if n == 0 {
		return Result{}, nil
	}
	lo := [3]float64{math.Inf(1), math.Inf(1), math.Inf(1)}
	hi := [3]float64{math.Inf(-1), math.Inf(-1), math.Inf(-1)}
	var volume float64
	for i := range n {
		if i%checkEvery == 0 {
			if err := ctx.Err(); err != nil {
				return Result{}, err
			}
		}
		// Cada triângulo: normal (12 bytes), três vértices (36) e um atributo (2).
		base := 84 + 50*i + 12
		var v [3][3]float64
		for j := range 3 {
			for k := range 3 {
				off := base + 12*j + 4*k
				c := float64(math.Float32frombits(binary.LittleEndian.Uint32(data[off : off+4])))
				v[j][k] = c
				lo[k] = math.Min(lo[k], c)
				hi[k] = math.Max(hi[k], c)
			}
		}
		// Volume com sinal do tetraedro formado com a origem.
		volume += dot(v[0], cross(v[1], v[2])) / 6
	}
	return Result{
		VolumeMM3: math.Abs(volume),
		BoxMM:     [3]float64{hi[0] - lo[0], hi[1] - lo[1], hi[2] - lo[2]},
	}, nil
}

func dot(a, b [3]float64) float64 { return a[0]*b[0] + a[1]*b[1] + a[2]*b[2] }

func cross(a, b [3]float64) [3]float64 {
	return [3]float64{a[1]*b[2] - a[2]*b[1], a[2]*b[0] - a[0]*b[2], a[0]*b[1] - a[1]*b[0]}
}
