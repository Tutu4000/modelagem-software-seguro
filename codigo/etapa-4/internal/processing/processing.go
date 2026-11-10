// Package processing is the worker logic: it checks the file and simulates
// slicing with the volume and the bounding box (E4.4).
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

// ErrHashMismatch means the downloaded file is not the one the API validated.
var ErrHashMismatch = errors.New("the file's SHA-256 does not match")

// ErrInvalidMesh means the triangles cannot be a printable solid. The price
// comes from the volume, so a mesh built to produce an absurd one is refused.
var ErrInvalidMesh = errors.New("the mesh is not a valid solid")

type Result struct {
	VolumeMM3 float64
	BBoxMM    [3]float64
}

// checkEvery is how many triangles pass between deadline checks.
const checkEvery = 100_000

// Analyze checks the hash before anything else and runs stl.Parse again: the
// worker does not trust what comes from the queue.
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
		// Each triangle: normal (12 bytes), three vertices (36) and an
		// attribute (2).
		base := 84 + 50*i + 12
		var v [3][3]float64
		for j := range 3 {
			for k := range 3 {
				off := base + 12*j + 4*k
				c := float64(math.Float32frombits(binary.LittleEndian.Uint32(data[off : off+4])))
				if math.IsNaN(c) || math.IsInf(c, 0) {
					return Result{}, ErrInvalidMesh
				}
				v[j][k] = c
				lo[k] = math.Min(lo[k], c)
				hi[k] = math.Max(hi[k], c)
			}
		}
		// Signed volume of the tetrahedron formed with the origin.
		volume += dot(v[0], cross(v[1], v[2])) / 6
	}
	res := Result{
		VolumeMM3: math.Abs(volume),
		BBoxMM:    [3]float64{hi[0] - lo[0], hi[1] - lo[1], hi[2] - lo[2]},
	}
	// A solid never has more volume than its bounding box. Open or inverted
	// triangles far from the origin do, and would inflate or overflow the
	// quote.
	box := res.BBoxMM[0] * res.BBoxMM[1] * res.BBoxMM[2]
	if math.IsNaN(res.VolumeMM3) || res.VolumeMM3 > box*(1+volumeTolerance) {
		return Result{}, ErrInvalidMesh
	}
	return res, nil
}

// volumeTolerance absorbs the float32 rounding of a box that fills its
// bounding box exactly.
const volumeTolerance = 1e-6

func dot(a, b [3]float64) float64 { return a[0]*b[0] + a[1]*b[1] + a[2]*b[2] }

func cross(a, b [3]float64) [3]float64 {
	return [3]float64{a[1]*b[2] - a[2]*b[1], a[2]*b[0] - a[0]*b[2], a[0]*b[1] - a[1]*b[0]}
}
