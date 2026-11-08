package orders

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"nuggetmachine/internal/audit"
	"nuggetmachine/internal/auth"
	"nuggetmachine/internal/files"
	"nuggetmachine/internal/partners"
	"nuggetmachine/internal/pricing"
	"nuggetmachine/internal/problem"
	"nuggetmachine/internal/server"
)

// Handler serves the customer routes of the orders module.
type Handler struct {
	Store *Store
	// Files and Tasks belong to the files and queue modules: the order only
	// reads them through these interfaces (E3.1).
	Files       files.Repo
	Tasks       files.TaskReader
	Audit       *audit.Logger
	BuildVolume [3]float64
	Now         func() time.Time
	// Gateway and Carrier are the partners (FC5), reached on the partners
	// network.
	Gateway Gateway
	Carrier Carrier
}

type Gateway interface {
	CreateCharge(ctx context.Context, r partners.ChargeRequest) (partners.Charge, error)
}

type Carrier interface {
	Quote(ctx context.Context, r partners.QuoteRequest) (partners.ShippingQuote, error)
}

// Catalog is the active price table, for the front-end to build the order
// form and show prices.
func (h *Handler) Catalog(c *gin.Context) {
	version, rules, err := h.Store.ActiveRules(c.Request.Context())
	if err != nil {
		problem.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"version": version, "currency": "BRL", "rules": rules})
}

type createRequest struct {
	FileID    uuid.UUID        `json:"file_id"`
	Material  string           `json:"material"`
	Color     string           `json:"color"`
	Quality   string           `json:"quality"`
	Finishing string           `json:"finishing"`
	Quantity  int              `json:"quantity"`
	Shipping  partners.Address `json:"shipping"`
}

// Create quotes an order on the server, from the customer's processed file and
// the active price table. The body has no price field, and any unknown field
// is refused (CA03).
func (h *Handler) Create(c *gin.Context) {
	user := auth.MustUser(c)
	var req createRequest
	if err := server.DecodeJSON(c, &req); err != nil {
		problem.Abort(c, problem.New(problem.InvalidRequest, err))
		return
	}
	if err := req.Shipping.Validate(); err != nil {
		problem.Abort(c, problem.New(problem.InvalidRequest, err))
		return
	}
	ctx := c.Request.Context()

	file, err := h.Files.FindForUser(ctx, user, files.FileID(req.FileID))
	if errors.Is(err, files.ErrNotFound) {
		problem.Abort(c, problem.New(problem.Forbidden, err))
		return
	}
	if err != nil {
		problem.Abort(c, err)
		return
	}
	task, err := h.Tasks.StatusOf(ctx, file.ID)
	if err != nil {
		problem.Abort(c, err)
		return
	}
	if task.State != "done" || task.VolumeMM3 == nil || task.BBoxMM == nil {
		problem.Abort(c, problem.New(problem.Conflict, fmt.Errorf("file %s is %s, not processed", file.ID, task.State)))
		return
	}

	version, rules, err := h.Store.ActiveRules(ctx)
	if err != nil {
		problem.Abort(c, err)
		return
	}
	quote, err := rules.Quote(*task.VolumeMM3, pricing.Choice{
		Material: req.Material, Color: req.Color, Quality: req.Quality, Finishing: req.Finishing, Quantity: req.Quantity,
	})
	if err != nil {
		problem.Abort(c, problem.New(problem.InvalidRequest, err))
		return
	}
	// The carrier quotes the shipping of all units (RF07). Without it there
	// is no total, so no order (D03).
	shipping, err := h.Carrier.Quote(ctx, partners.QuoteRequest{
		PostalCode: req.Shipping.PostalCode,
		VolumeCM3:  *task.VolumeMM3 / 1000 * float64(req.Quantity),
	})
	if err != nil {
		problem.Abort(c, problem.New(problem.PartnerUnavailable, err))
		return
	}
	quote = quote.WithShipping(shipping.PriceCents)

	now := h.Now()
	a := req.Shipping
	o := Order{
		ID: uuid.New(), OwnerID: uuid.UUID(user.ID()), FileID: uuid.UUID(file.ID),
		Material: req.Material, Color: req.Color, Quality: req.Quality, Finishing: req.Finishing,
		Quantity:          int32(req.Quantity), // #nosec G115 -- Quote limits the quantity to 1..1000
		PriceTableVersion: version, UnitCents: quote.UnitCents, DiscountCents: quote.DiscountCents,
		ShippingCents: quote.ShippingCents, TotalCents: quote.TotalCents,
		VolumeMm3: *task.VolumeMM3, BboxMm: task.BBoxMM[:], DfmFlags: pricing.DFM(*task.BBoxMM, h.BuildVolume),
		Status: string(StatusQuoted), QuoteExpiresAt: now.Add(QuoteValidity), CreatedAt: now,
		ShipRecipient: a.Recipient, ShipStreet: a.Street, ShipNumber: a.Number,
		ShipCity: a.City, ShipState: a.State, ShipPostalCode: a.PostalCode,
	}
	if err := h.Store.Create(ctx, o); err != nil {
		problem.Abort(c, err)
		return
	}
	h.record(c, "order.created", o.ID, fmt.Sprintf("total %d cents, price table %d", o.TotalCents, version))
	c.JSON(http.StatusCreated, NewView(o, Details{}))
}

// Pay creates the charge at the gateway for the total computed by the server
// (C14). The order only becomes paid when the gateway's signed notification
// arrives. A second call while a charge is pending returns the same charge.
func (h *Handler) Pay(c *gin.Context) {
	user := auth.MustUser(c)
	o, ok := h.own(c, user)
	if !ok {
		return
	}
	if Status(o.Status) != StatusAwaitingPayment {
		problem.Abort(c, problem.New(problem.Conflict, fmt.Errorf("order %s is %s", o.ID, o.Status)))
		return
	}
	ctx := c.Request.Context()
	if p, err := h.Store.LivePayment(ctx, o.ID); err != nil {
		problem.Abort(c, err)
		return
	} else if p != nil {
		c.JSON(http.StatusAccepted, p)
		return
	}
	charge, err := h.Gateway.CreateCharge(ctx, partners.ChargeRequest{
		OrderID: o.ID, AmountCents: o.TotalCents, IdempotencyKey: "order-" + o.ID.String(),
	})
	if err != nil {
		problem.Abort(c, problem.New(problem.PartnerUnavailable, err))
		return
	}
	if err := h.Store.AddPayment(ctx, o.ID, charge.ChargeID, o.TotalCents); err != nil {
		problem.Abort(c, err)
		return
	}
	h.record(c, "payment.requested", o.ID, fmt.Sprintf("charge %s, %d cents", charge.ChargeID, o.TotalCents))
	c.JSON(http.StatusAccepted, Payment{ChargeID: charge.ChargeID, AmountCents: o.TotalCents, Status: "pending", CreatedAt: h.Now()})
}

// Submit sends a quoted order to technical review, while the quote holds.
func (h *Handler) Submit(c *gin.Context) {
	user := auth.MustUser(c)
	o, ok := h.own(c, user)
	if !ok {
		return
	}
	if Status(o.Status) != StatusQuoted {
		problem.Abort(c, problem.New(problem.Conflict, fmt.Errorf("order %s is %s", o.ID, o.Status)))
		return
	}
	if h.Now().After(o.QuoteExpiresAt) {
		problem.Abort(c, problem.New(problem.QuoteExpired, fmt.Errorf("quote of order %s expired at %s", o.ID, o.QuoteExpiresAt)))
		return
	}
	err := h.Store.Transition(c.Request.Context(), o.ID, StatusQuoted, StatusAwaitingReview, uuid.UUID(user.ID()))
	if errors.Is(err, ErrConflict) {
		problem.Abort(c, problem.New(problem.Conflict, err))
		return
	}
	if err != nil {
		problem.Abort(c, err)
		return
	}
	h.record(c, "order.submitted", o.ID, "")
	o.Status = string(StatusAwaitingReview)
	c.JSON(http.StatusOK, NewView(o, Details{}))
}

func (h *Handler) List(c *gin.Context) {
	user := auth.MustUser(c)
	list, err := h.Store.ListForOwner(c.Request.Context(), uuid.UUID(user.ID()))
	if err != nil {
		problem.Abort(c, err)
		return
	}
	out := make([]View, 0, len(list))
	for _, o := range list {
		out = append(out, NewView(o, Details{}))
	}
	c.JSON(http.StatusOK, gin.H{"orders": out})
}

// Get shows one order with its history and tracking (RF12), the review result
// (RF20) and the payment.
func (h *Handler) Get(c *gin.Context) {
	user := auth.MustUser(c)
	o, ok := h.own(c, user)
	if !ok {
		return
	}
	d, err := h.Store.Details(c.Request.Context(), o.ID)
	if err != nil {
		problem.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, NewView(o, d))
}

// own finds an order of the user. Someone else's order and a missing one get
// the same 403, as with files.
func (h *Handler) own(c *gin.Context, user auth.User) (Order, bool) {
	order, err := h.lookup(c, user)
	if err == nil {
		return order, true
	}
	if !errors.Is(err, ErrNotFound) {
		problem.Abort(c, err)
		return Order{}, false
	}
	if rerr := h.Audit.Record(audit.Event{
		Name: "authz.denied", Actor: user.ID().String(), Action: "order.read", Target: c.Param("id"), Result: "denied", Origin: c.ClientIP(),
	}); rerr != nil {
		_ = c.Error(rerr)
	}
	problem.Abort(c, problem.New(problem.Forbidden, err))
	return Order{}, false
}

// lookup treats an ID that is not a UUID like a missing order.
func (h *Handler) lookup(c *gin.Context, user auth.User) (Order, error) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return Order{}, ErrNotFound
	}
	return h.Store.GetForOwner(c.Request.Context(), uuid.UUID(user.ID()), id)
}

func (h *Handler) record(c *gin.Context, name string, id uuid.UUID, reason string) {
	user := auth.MustUser(c)
	if err := h.Audit.Record(audit.Event{
		Name: name, Actor: user.ID().String(), Action: name, Target: id.String(), Result: "ok", Reason: reason, Origin: c.ClientIP(),
	}); err != nil {
		_ = c.Error(err)
	}
}
