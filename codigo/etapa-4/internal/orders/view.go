package orders

import (
	"time"

	"github.com/google/uuid"

	"nuggetmachine/internal/partners"
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
	ShippingCents     int64     `json:"shipping_cents"`
	TotalCents        int64     `json:"total_cents"`
	Currency          string    `json:"currency"`
	Status            string    `json:"status"`
	QuoteExpiresAt    time.Time `json:"quote_expires_at"`
	CreatedAt         time.Time `json:"created_at"`
	// DFMFlags are the manufacturability warnings, such as a part bigger than
	// the printer (RF19). The customer sees them with the quote.
	DFMFlags     []string          `json:"dfm_flags"`
	Shipping     *partners.Address `json:"shipping,omitempty"`
	TrackingCode *string           `json:"tracking_code,omitempty"`
	Payment      *Payment          `json:"payment,omitempty"`
	History      []Change          `json:"history,omitempty"`
	Review       *Review           `json:"review,omitempty"`
}

// StaffView adds what the panel needs for the review (RF19).
type StaffView struct {
	View
	OwnerID            uuid.UUID  `json:"owner_id"`
	VolumeMM3          float64    `json:"volume_mm3"`
	BBoxMM             []float64  `json:"bbox_mm"`
	AssignedTechnician *uuid.UUID `json:"assigned_technician"`
}

// Details are the parts of an order loaded only for its detail page.
type Details struct {
	History []Change
	Review  *Review
	Payment *Payment
}

func AddressOf(o Order) partners.Address {
	return partners.Address{
		Recipient: o.ShipRecipient, Street: o.ShipStreet, Number: o.ShipNumber,
		City: o.ShipCity, State: o.ShipState, PostalCode: o.ShipPostalCode,
	}
}

// NewView is the customer's view, with their own address until the
// retention clears it.
func NewView(o Order, d Details) View {
	var shipping *partners.Address
	if o.AddressPurgedAt == nil {
		address := AddressOf(o)
		shipping = &address
	}
	return View{
		ID: o.ID, FileID: o.FileID,
		Material: o.Material, Color: o.Color, Quality: o.Quality, Finishing: o.Finishing, Quantity: o.Quantity,
		PriceTableVersion: o.PriceTableVersion, UnitCents: o.UnitCents, DiscountCents: o.DiscountCents,
		ShippingCents: o.ShippingCents, TotalCents: o.TotalCents,
		Currency: "BRL", Status: o.Status, QuoteExpiresAt: o.QuoteExpiresAt, CreatedAt: o.CreatedAt,
		DFMFlags: dfm(o.DfmFlags),
		Shipping: shipping, TrackingCode: o.TrackingCode, Payment: d.Payment,
		History: d.History, Review: d.Review,
	}
}

// NewStaffView shows the address only when withAddress: support and admin
// need it, the technician does not (LGPD: data minimization).
func NewStaffView(o Order, d Details, withAddress bool) StaffView {
	v := NewView(o, d)
	if !withAddress {
		v.Shipping = nil
	}
	return StaffView{
		View:    v,
		OwnerID: o.OwnerID, VolumeMM3: o.VolumeMm3, BBoxMM: o.BboxMm,
		AssignedTechnician: o.AssignedTechnician,
	}
}

// dfm never answers null: an order without warnings has an empty list.
func dfm(flags []string) []string {
	if flags == nil {
		return []string{}
	}
	return flags
}
