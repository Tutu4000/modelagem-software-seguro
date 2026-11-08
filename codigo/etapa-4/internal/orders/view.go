package orders

import (
	"time"

	"github.com/google/uuid"
)

// View is what the customer sees of an order: no internal fields such as the
// assigned technician.
type View struct {
	ID                uuid.UUID `json:"id"`
	FileID            uuid.UUID `json:"file_id"`
	Material          string    `json:"material"`
	Color             string    `json:"color"`
	Quality           string    `json:"quality"`
	Finishing         string    `json:"finishing"`
	Quantity          int32     `json:"quantity"`
	PriceTableVersion int64     `json:"price_table_version"`
	UnitCents         int64     `json:"unit_cents"`
	DiscountCents     int64     `json:"discount_cents"`
	TotalCents        int64     `json:"total_cents"`
	Currency          string    `json:"currency"`
	Status            string    `json:"status"`
	QuoteExpiresAt    time.Time `json:"quote_expires_at"`
	CreatedAt         time.Time `json:"created_at"`
	History           []Change  `json:"history,omitempty"`
	Review            *Review   `json:"review,omitempty"`
}

// StaffView adds what the panel needs for the review (RF19).
type StaffView struct {
	View
	OwnerID            uuid.UUID  `json:"owner_id"`
	VolumeMM3          float64    `json:"volume_mm3"`
	BBoxMM             []float64  `json:"bbox_mm"`
	DFMFlags           []string   `json:"dfm_flags"`
	AssignedTechnician *uuid.UUID `json:"assigned_technician"`
}

func NewView(o Order, history []Change, review *Review) View {
	return View{
		ID: o.ID, FileID: o.FileID,
		Material: o.Material, Color: o.Color, Quality: o.Quality, Finishing: o.Finishing, Quantity: o.Quantity,
		PriceTableVersion: o.PriceTableVersion, UnitCents: o.UnitCents, DiscountCents: o.DiscountCents, TotalCents: o.TotalCents,
		Currency: "BRL", Status: o.Status, QuoteExpiresAt: o.QuoteExpiresAt, CreatedAt: o.CreatedAt,
		History: history, Review: review,
	}
}

func NewStaffView(o Order, history []Change, review *Review) StaffView {
	return StaffView{
		View:    NewView(o, history, review),
		OwnerID: o.OwnerID, VolumeMM3: o.VolumeMm3, BBoxMM: o.BboxMm, DFMFlags: o.DfmFlags,
		AssignedTechnician: o.AssignedTechnician,
	}
}
