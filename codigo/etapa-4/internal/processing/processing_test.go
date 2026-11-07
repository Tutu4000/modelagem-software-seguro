package processing

import (
	"context"
	"crypto/sha256"
	"errors"
	"math"
	"testing"
	"time"

	"nuggetmachine/internal/fakestl"
)

func TestVolumeECaixaDeUmParalelepipedo(t *testing.T) {
	data := fakestl.Box(10, 20, 30)

	got, err := Analyze(context.Background(), data, sha256.Sum256(data))
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(got.VolumeMM3-6000) > 1e-6 {
		t.Errorf("volume %v, esperado 6000", got.VolumeMM3)
	}
	if got.BoxMM != [3]float64{10, 20, 30} {
		t.Errorf("caixa %v, esperado [10 20 30]", got.BoxMM)
	}
}

func TestHashDiferenteERecusado(t *testing.T) {
	data := fakestl.Box(1, 1, 1)
	other := sha256.Sum256([]byte("outro arquivo"))

	if _, err := Analyze(context.Background(), data, other); !errors.Is(err, ErrHashMismatch) {
		t.Fatalf("erro %v, esperado ErrHashMismatch", err)
	}
}

func TestPrazoVencidoInterrompe(t *testing.T) {
	data := fakestl.Box(1, 1, 1)
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()

	if _, err := Analyze(ctx, data, sha256.Sum256(data)); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("erro %v, esperado context.DeadlineExceeded", err)
	}
}
