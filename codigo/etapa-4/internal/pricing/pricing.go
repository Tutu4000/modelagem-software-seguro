// Package pricing computes the quote and the manufacturability checks (DFM)
// only on the server, from the active price table (E8.1, CA03). Everything is
// in integer cents.
package pricing

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"sort"
	"strconv"
	"strings"
)

// Rules is the JSON of one price table version.
type Rules struct {
	Materials map[string]Material `json:"materials"`
	// Qualities are multipliers in per mille: 1000 keeps the material price.
	Qualities map[string]int64 `json:"qualities"`
	// Finishings cost a fixed amount per unit.
	Finishings map[string]int64 `json:"finishings"`
	// SetupCents is charged once per order.
	SetupCents int64  `json:"setup_cents"`
	Discounts  []Tier `json:"discounts"`
}

type Material struct {
	Name        string   `json:"name"`
	CentsPerCM3 int64    `json:"cents_per_cm3"`
	Colors      []string `json:"colors"`
}

// Tier gives PercentOff of the units price from MinQuantity units (RF24).
type Tier struct {
	MinQuantity int `json:"min_quantity"`
	PercentOff  int `json:"percent_off"`
}

const (
	MinQuantity = 1
	MaxQuantity = 1000
)

// ErrInvalidChoice is any option that is not in the price table.
var ErrInvalidChoice = errors.New("option not in the price table")

// Validate refuses a table that would produce wrong or negative prices. It runs
// when an admin proposes a new version.
func (r Rules) Validate() error {
	if len(r.Materials) == 0 || len(r.Qualities) == 0 || len(r.Finishings) == 0 {
		return errors.New("the table needs materials, qualities and finishings")
	}
	for id, m := range r.Materials {
		if m.Name == "" || m.CentsPerCM3 <= 0 || len(m.Colors) == 0 {
			return fmt.Errorf("material %q needs a name, a positive price and colors", id)
		}
	}
	for id, q := range r.Qualities {
		if q < 100 || q > 10_000 {
			return fmt.Errorf("quality %q: multiplier %d per mille out of 100..10000", id, q)
		}
	}
	for id, f := range r.Finishings {
		if f < 0 {
			return fmt.Errorf("finishing %q with a negative price", id)
		}
	}
	if r.SetupCents < 0 {
		return errors.New("negative setup fee")
	}
	last, lastPct := 0, 0
	for _, t := range r.Discounts {
		if t.MinQuantity <= last || t.PercentOff <= lastPct || t.PercentOff >= 100 {
			return errors.New("discount tiers must grow in quantity and percentage, below 100%")
		}
		last, lastPct = t.MinQuantity, t.PercentOff
	}
	return nil
}

// Choice is what the customer configures (RF04, RF05, RF21).
type Choice struct {
	Material  string
	Color     string
	Quality   string
	Finishing string
	Quantity  int
}

// ErrTooExpensive means the model is too big to be priced.
var ErrTooExpensive = errors.New("the model is too big to quote")

// maxUnitCents is 1 billion reais a unit: far above any real part, and far
// below what overflows an int64 when multiplied by MaxQuantity.
const maxUnitCents = 1e11

type Quote struct {
	UnitCents     int64
	DiscountCents int64
	ShippingCents int64
	TotalCents    int64
}

// WithShipping adds the carrier's price (RF07). It comes after the discount:
// the volume discount does not apply to shipping.
func (q Quote) WithShipping(cents int64) Quote {
	q.ShippingCents = cents
	q.TotalCents += cents
	return q
}

// Quote prices one order: per unit, material by volume times the quality
// multiplier, plus the finishing, rounded up to the cent; then the volume
// discount on the units and the setup fee.
func (r Rules) Quote(volumeMM3 float64, c Choice) (Quote, error) {
	m, ok := r.Materials[c.Material]
	if !ok || !slices.Contains(m.Colors, c.Color) {
		return Quote{}, fmt.Errorf("%w: material %q in color %q", ErrInvalidChoice, c.Material, c.Color)
	}
	quality, ok := r.Qualities[c.Quality]
	if !ok {
		return Quote{}, fmt.Errorf("%w: quality %q", ErrInvalidChoice, c.Quality)
	}
	finishing, ok := r.Finishings[c.Finishing]
	if !ok {
		return Quote{}, fmt.Errorf("%w: finishing %q", ErrInvalidChoice, c.Finishing)
	}
	if c.Quantity < MinQuantity || c.Quantity > MaxQuantity {
		return Quote{}, fmt.Errorf("%w: quantity %d out of %d..%d", ErrInvalidChoice, c.Quantity, MinQuantity, MaxQuantity)
	}
	if volumeMM3 <= 0 || math.IsNaN(volumeMM3) || math.IsInf(volumeMM3, 0) {
		return Quote{}, errors.New("the model has no volume")
	}

	material := math.Ceil(volumeMM3 / 1000 * float64(m.CentsPerCM3) * float64(quality) / 1000)
	// Above this the conversion to int64 and the product by the quantity
	// could overflow and wrap to a small or negative total.
	if material > maxUnitCents {
		return Quote{}, fmt.Errorf("%w: a unit above %d cents", ErrTooExpensive, int64(maxUnitCents))
	}
	unit := int64(material) + finishing
	units := unit * int64(c.Quantity)
	discount := units * int64(r.discount(c.Quantity)) / 100
	return Quote{UnitCents: unit, DiscountCents: discount, TotalCents: units - discount + r.SetupCents}, nil
}

func (r Rules) discount(quantity int) int {
	tiers := slices.Clone(r.Discounts)
	sort.Slice(tiers, func(i, j int) bool { return tiers[i].MinQuantity < tiers[j].MinQuantity })
	pct := 0
	for _, t := range tiers {
		if quantity >= t.MinQuantity {
			pct = t.PercentOff
		}
	}
	return pct
}

// ParseBuildVolume reads "x,y,z" in millimeters, such as "250,250,250".
func ParseBuildVolume(s string) ([3]float64, error) {
	var v [3]float64
	parts := strings.Split(s, ",")
	if len(parts) != 3 {
		return v, fmt.Errorf("build volume %q: want x,y,z in mm", s)
	}
	for i, p := range parts {
		f, err := strconv.ParseFloat(strings.TrimSpace(p), 64)
		if err != nil || f <= 0 {
			return v, fmt.Errorf("build volume %q: want positive numbers", s)
		}
		v[i] = f
	}
	return v, nil
}

// FlagExceedsBuildVolume marks a part bigger than the printer in any axis.
const FlagExceedsBuildVolume = "exceeds_build_volume"

// DFM runs the manufacturability checks on the model's bounding box. The
// technician sees the flags in the review (RF19).
func DFM(bboxMM, buildVolumeMM [3]float64) []string {
	flags := []string{}
	for i := range 3 {
		if bboxMM[i] > buildVolumeMM[i] {
			flags = append(flags, FlagExceedsBuildVolume)
			break
		}
	}
	return flags
}
