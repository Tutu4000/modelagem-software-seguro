package processing

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"math"
	"testing"
	"time"

	"nuggetmachine/internal/fakestl"
)

func TestVolumeAndBoundingBoxOfABox(t *testing.T) {
	data := fakestl.Box(10, 20, 30)

	got, err := Analyze(context.Background(), data, sha256.Sum256(data))
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(got.VolumeMM3-6000) > 1e-6 {
		t.Errorf("volume %v, want 6000", got.VolumeMM3)
	}
	if got.BBoxMM != [3]float64{10, 20, 30} {
		t.Errorf("bounding box %v, want [10 20 30]", got.BBoxMM)
	}
}

func TestDifferentHashIsRejected(t *testing.T) {
	data := fakestl.Box(1, 1, 1)
	other := sha256.Sum256([]byte("another file"))

	if _, err := Analyze(context.Background(), data, other); !errors.Is(err, ErrHashMismatch) {
		t.Fatalf("error %v, want ErrHashMismatch", err)
	}
}

func TestExpiredDeadlineStopsProcessing(t *testing.T) {
	data := fakestl.Box(1, 1, 1)
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()

	if _, err := Analyze(ctx, data, sha256.Sum256(data)); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error %v, want context.DeadlineExceeded", err)
	}
}

// triangles builds a binary STL from raw vertices, three per triangle.
func triangles(vertices ...[3]float32) []byte {
	n := len(vertices) / 3
	data := make([]byte, 84+50*n)
	binary.LittleEndian.PutUint32(data[80:], uint32(n))
	for i, v := range vertices {
		base := 84 + 50*(i/3) + 12 + 12*(i%3)
		for k, c := range v {
			binary.LittleEndian.PutUint32(data[base+4*k:], math.Float32bits(c))
		}
	}
	return data
}

func TestMeshThatCannotBeASolidIsRejected(t *testing.T) {
	inf := float32(math.Inf(1))
	nan := float32(math.NaN())
	cases := []struct {
		name string
		data []byte
	}{
		// One open triangle far from the origin: a huge "volume" in a flat
		// bounding box, which would overflow the quote.
		{"volume larger than the bounding box", triangles([3]float32{0, 0, 1e30}, [3]float32{200, 0, 1e30}, [3]float32{0, 200, 1e30})},
		{"infinite coordinate", triangles([3]float32{0, 0, 0}, [3]float32{inf, 0, 0}, [3]float32{0, 1, 0})},
		{"NaN coordinate", triangles([3]float32{0, 0, 0}, [3]float32{1, nan, 0}, [3]float32{0, 1, 0})},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Analyze(context.Background(), tc.data, sha256.Sum256(tc.data)); !errors.Is(err, ErrInvalidMesh) {
				t.Fatalf("error %v, want ErrInvalidMesh", err)
			}
		})
	}
}
