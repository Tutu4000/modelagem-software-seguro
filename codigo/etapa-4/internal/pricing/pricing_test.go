package pricing

import (
	"errors"
	"slices"
	"testing"
)

func rules() Rules {
	return Rules{
		Materials: map[string]Material{
			"pla":  {Name: "PLA", CentsPerCM3: 15, Colors: []string{"black", "white"}},
			"petg": {Name: "PETG", CentsPerCM3: 20, Colors: []string{"clear"}},
		},
		Qualities:  map[string]int64{"draft": 800, "standard": 1000, "fine": 1500},
		Finishings: map[string]int64{"none": 0, "painted": 1500},
		SetupCents: 1000,
		Discounts:  []Tier{{MinQuantity: 10, PercentOff: 5}, {MinQuantity: 50, PercentOff: 10}},
	}
}

func TestQuoteByHand(t *testing.T) {
	cases := []struct {
		name   string
		volume float64
		choice Choice
		want   Quote
	}{
		// 6 cm3 x 15 = 90 cents per unit; 2 units = 180; + setup 1000.
		{"PLA standard", 6000, Choice{"pla", "black", "standard", "none", 2}, Quote{UnitCents: 90, TotalCents: 1180}},
		// 6 cm3 x 15 x 1.5 = 135 + painted 1500 = 1635 per unit.
		{"PLA fine painted", 6000, Choice{"pla", "white", "fine", "painted", 1}, Quote{UnitCents: 1635, TotalCents: 2635}},
		// 1.234 cm3 x 20 x 0.8 = 19.744, rounded up to 20.
		{"rounds up to the cent", 1234, Choice{"petg", "clear", "draft", "none", 1}, Quote{UnitCents: 20, TotalCents: 1020}},
		// 10 units: 900, 5% off = 45.
		{"first discount tier", 6000, Choice{"pla", "black", "standard", "none", 10}, Quote{UnitCents: 90, DiscountCents: 45, TotalCents: 1855}},
		// 50 units: 4500, 10% off = 450.
		{"second discount tier", 6000, Choice{"pla", "black", "standard", "none", 50}, Quote{UnitCents: 90, DiscountCents: 450, TotalCents: 5050}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := rules().Quote(tc.volume, tc.choice)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Errorf("quote %+v, want %+v", got, tc.want)
			}
		})
	}
}

// The volume discount does not apply to shipping.
func TestShippingComesAfterTheDiscount(t *testing.T) {
	q, err := rules().Quote(6000, Choice{"pla", "black", "standard", "none", 10})
	if err != nil {
		t.Fatal(err)
	}
	got := q.WithShipping(2500)
	if got != (Quote{UnitCents: 90, DiscountCents: 45, ShippingCents: 2500, TotalCents: 1855 + 2500}) {
		t.Errorf("quote %+v", got)
	}
}

func TestQuoteRefusesOptionsOutsideTheTable(t *testing.T) {
	cases := map[string]Choice{
		"unknown material":       {"abs", "black", "standard", "none", 1},
		"color of another":       {"pla", "clear", "standard", "none", 1},
		"unknown quality":        {"pla", "black", "ultra", "none", 1},
		"unknown finishing":      {"pla", "black", "standard", "gold", 1},
		"zero quantity":          {"pla", "black", "standard", "none", 0},
		"quantity above the cap": {"pla", "black", "standard", "none", 1001},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := rules().Quote(6000, c); !errors.Is(err, ErrInvalidChoice) {
				t.Errorf("error %v, want ErrInvalidChoice", err)
			}
		})
	}
	if _, err := rules().Quote(0, Choice{"pla", "black", "standard", "none", 1}); err == nil {
		t.Error("a model without volume got a quote")
	}
}

func TestValidateRefusesBrokenTables(t *testing.T) {
	if err := rules().Validate(); err != nil {
		t.Fatalf("valid table refused: %v", err)
	}
	broken := map[string]func(*Rules){
		"negative material price": func(r *Rules) { r.Materials["pla"] = Material{"PLA", -1, []string{"black"}} },
		"material without colors": func(r *Rules) { r.Materials["pla"] = Material{"PLA", 15, nil} },
		"zero quality multiplier": func(r *Rules) { r.Qualities["draft"] = 0 },
		"negative finishing":      func(r *Rules) { r.Finishings["painted"] = -1 },
		"negative setup":          func(r *Rules) { r.SetupCents = -1 },
		"100% discount":           func(r *Rules) { r.Discounts = []Tier{{10, 100}} },
		"tiers out of order":      func(r *Rules) { r.Discounts = []Tier{{50, 10}, {10, 5}} },
		"no materials":            func(r *Rules) { r.Materials = nil },
	}
	for name, breakIt := range broken {
		t.Run(name, func(t *testing.T) {
			r := rules()
			breakIt(&r)
			if r.Validate() == nil {
				t.Error("broken table accepted")
			}
		})
	}
}

func TestDFMFlagsPartsBiggerThanThePrinter(t *testing.T) {
	build := [3]float64{250, 250, 250}
	if flags := DFM([3]float64{100, 200, 250}, build); len(flags) != 0 {
		t.Errorf("flags %v for a part that fits", flags)
	}
	if flags := DFM([3]float64{100, 300, 50}, build); !slices.Equal(flags, []string{FlagExceedsBuildVolume}) {
		t.Errorf("flags %v, want exceeds_build_volume", flags)
	}
}

// A volume built to overflow the cents may not wrap into a cheap total.
func TestHugeVolumeIsRefusedInsteadOfOverflowing(t *testing.T) {
	for _, volume := range []float64{1e20, 4.6e21, 1e30, 1e300} {
		q, err := rules().Quote(volume, Choice{"pla", "black", "standard", "none", MaxQuantity})
		if !errors.Is(err, ErrTooExpensive) {
			t.Errorf("volume %g: quote %+v and error %v, want ErrTooExpensive", volume, q, err)
		}
	}
}
