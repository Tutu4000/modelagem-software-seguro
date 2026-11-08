package panel

import (
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"nuggetmachine/internal/audit"
	"nuggetmachine/internal/auth"
	"nuggetmachine/internal/authz"
	"nuggetmachine/internal/orders"
	"nuggetmachine/internal/partners"
	"nuggetmachine/internal/pricing"
	"nuggetmachine/internal/problem"
	"nuggetmachine/internal/server"
)

// staffDownloadTTL is the life of the URL a technician gets for a design.
const staffDownloadTTL = 5 * time.Minute

var listableStatuses = []string{"", "quoted", "awaiting_review", "awaiting_payment", "rejected",
	"paid", "in_production", "shipped", "delivered", "cancelled"}

func (h handlers) listOrders(c *gin.Context) {
	status := c.Query("status")
	if !slices.Contains(listableStatuses, status) {
		problem.Abort(c, problem.New(problem.InvalidRequest, fmt.Errorf("unknown status %q", status)))
		return
	}
	list, err := h.d.Orders.ListByStatus(c.Request.Context(), status)
	h.respondList(c, list, err)
}

func (h handlers) assignedOrders(c *gin.Context) {
	list, err := h.d.Orders.ListAssigned(c.Request.Context(), uuid.UUID(auth.MustUser(c).ID()))
	h.respondList(c, list, err)
}

func (h handlers) respondList(c *gin.Context, list []orders.Order, err error) {
	if err != nil {
		problem.Abort(c, err)
		return
	}
	address := seesAddress(auth.MustUser(c))
	out := make([]orders.StaffView, 0, len(list))
	for _, o := range list {
		out = append(out, orders.NewStaffView(o, orders.Details{}, address))
	}
	c.JSON(http.StatusOK, gin.H{"orders": out})
}

// seesAddress: support and admin handle deliveries; the technician does not
// need the customer's address (LGPD: data minimization).
func seesAddress(u auth.User) bool {
	return u.Role() == auth.RoleSupport || u.Role() == auth.RoleAdmin
}

func (h handlers) getOrder(c *gin.Context) {
	u := auth.MustUser(c)
	o, ok := h.order(c, func(o orders.Order) error { return authz.CanReadOrder(u, auth.UserID(o.OwnerID), techOf(o)) })
	if !ok {
		return
	}
	d, err := h.d.Orders.Details(c.Request.Context(), o.ID)
	if err != nil {
		problem.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, orders.NewStaffView(o, d, seesAddress(u)))
}

// startProduction moves a paid order into production (assigned technician).
func (h handlers) startProduction(c *gin.Context) {
	u := auth.MustUser(c)
	o, ok := h.order(c, func(o orders.Order) error { return authz.CanWorkOnOrder(u, techOf(o)) })
	if !ok {
		return
	}
	if err := h.d.Orders.Transition(c.Request.Context(), o.ID, orders.Status(o.Status), orders.StatusInProduction, uuid.UUID(u.ID())); err != nil {
		h.abortOrderError(c, err)
		return
	}
	h.record(c, "order.in_production", o.ID, "")
	c.JSON(http.StatusOK, gin.H{"id": o.ID, "status": orders.StatusInProduction})
}

// ship asks the carrier for a pickup with the order's address and keeps the
// tracking code. The order becomes shipped when the carrier's signed
// notification of the pickup arrives.
func (h handlers) ship(c *gin.Context) {
	u := auth.MustUser(c)
	o, ok := h.order(c, func(o orders.Order) error { return authz.CanWorkOnOrder(u, techOf(o)) })
	if !ok {
		return
	}
	if orders.Status(o.Status) != orders.StatusInProduction || o.ShipmentID != nil {
		problem.Abort(c, problem.New(problem.Conflict, fmt.Errorf("order %s is %s", o.ID, o.Status)))
		return
	}
	ctx := c.Request.Context()
	shipment, err := h.d.Carrier.CreateShipment(ctx, partners.ShipmentRequest{OrderID: o.ID, Address: orders.AddressOf(o)})
	if err != nil {
		problem.Abort(c, problem.New(problem.PartnerUnavailable, err))
		return
	}
	if err := h.d.Orders.SetShipment(ctx, o.ID, shipment.ShipmentID, shipment.TrackingCode); err != nil {
		h.abortOrderError(c, err)
		return
	}
	h.record(c, "shipment.requested", o.ID, "tracking "+shipment.TrackingCode)
	c.JSON(http.StatusAccepted, gin.H{"id": o.ID, "shipment_id": shipment.ShipmentID, "tracking_code": shipment.TrackingCode})
}

// assign gives an order awaiting review to a technician. Keycloak confirms
// that the account really is a technician.
func (h handlers) assign(c *gin.Context) {
	var body struct {
		TechnicianID uuid.UUID `json:"technician_id"`
	}
	if err := server.DecodeJSON(c, &body); err != nil {
		problem.Abort(c, problem.New(problem.InvalidRequest, err))
		return
	}
	o, ok := h.order(c, func(orders.Order) error { return nil })
	if !ok {
		return
	}
	ctx := c.Request.Context()
	isTech, err := h.d.IdP.HasRealmRole(ctx, body.TechnicianID, string(auth.RoleTechnician))
	if err != nil {
		problem.Abort(c, problem.New(problem.IdPUnavailable, err))
		return
	}
	if !isTech {
		problem.Abort(c, problem.New(problem.InvalidRequest, fmt.Errorf("%s is not a technician", body.TechnicianID)))
		return
	}
	if err := h.d.Orders.Assign(ctx, o.ID, body.TechnicianID); err != nil {
		h.abortOrderError(c, err)
		return
	}
	h.record(c, "order.assigned", o.ID, "technician "+body.TechnicianID.String())
	c.JSON(http.StatusOK, gin.H{"id": o.ID, "assigned_technician": body.TechnicianID})
}

// file gives the assigned technician a short-lived URL to the customer's
// design (critical: step-up and justification, D02). Not even an admin can
// open a design (R14, R17).
func (h handlers) file(c *gin.Context) {
	u := auth.MustUser(c)
	o, ok := h.order(c, func(o orders.Order) error { return authz.CanWorkOnOrder(u, techOf(o)) })
	if !ok {
		return
	}
	url, err := h.d.Files.PresignGet(o.FileID.String(), staffDownloadTTL)
	if err != nil {
		problem.Abort(c, err)
		return
	}
	h.record(c, "file.staff_download", o.ID, "file "+o.FileID.String())
	c.JSON(http.StatusOK, gin.H{"url": url, "expires_in": int(staffDownloadTTL.Seconds())})
}

// review records the DFM report and the decision of the assigned technician
// (E6.3). The customer cannot reach this route, and a second review of the
// same order is refused.
func (h handlers) review(c *gin.Context) {
	u := auth.MustUser(c)
	var body struct {
		Decision string `json:"decision"`
		Report   string `json:"report"`
	}
	if err := server.DecodeJSON(c, &body); err != nil {
		problem.Abort(c, problem.New(problem.InvalidRequest, err))
		return
	}
	if body.Decision != "approved" && body.Decision != "rejected" {
		problem.Abort(c, problem.New(problem.InvalidRequest, fmt.Errorf("decision %q", body.Decision)))
		return
	}
	if n := utf8.RuneCountInString(body.Report); n < 10 || n > 5000 {
		problem.Abort(c, problem.New(problem.InvalidRequest, errors.New("the DFM report must have 10 to 5000 characters")))
		return
	}
	o, ok := h.order(c, func(o orders.Order) error { return authz.CanWorkOnOrder(u, techOf(o)) })
	if !ok {
		return
	}
	if err := h.d.Orders.Review(c.Request.Context(), o.ID, uuid.UUID(u.ID()), body.Decision == "approved", body.Report); err != nil {
		h.abortOrderError(c, err)
		return
	}
	h.record(c, "review."+body.Decision, o.ID, "")
	c.JSON(http.StatusOK, gin.H{"id": o.ID, "decision": body.Decision})
}

func (h handlers) priceTables(c *gin.Context) {
	tables, err := h.d.Orders.PriceTables(c.Request.Context())
	if err != nil {
		problem.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"price_tables": tables})
}

// proposePriceTable creates a draft. It changes no price until another admin
// approves it.
func (h handlers) proposePriceTable(c *gin.Context) {
	var body struct {
		Rules pricing.Rules `json:"rules"`
	}
	if err := server.DecodeJSON(c, &body); err != nil {
		problem.Abort(c, problem.New(problem.InvalidRequest, err))
		return
	}
	if err := body.Rules.Validate(); err != nil {
		problem.Abort(c, problem.New(problem.InvalidRequest, err))
		return
	}
	version, err := h.d.Orders.ProposePriceTable(c.Request.Context(), body.Rules, uuid.UUID(auth.MustUser(c).ID()))
	if err != nil {
		problem.Abort(c, err)
		return
	}
	h.recordTable(c, "price_table.proposed", version)
	c.JSON(http.StatusCreated, gin.H{"version": version, "status": "draft"})
}

// approvePriceTable activates a draft (critical). The proposer cannot approve
// their own table: changing prices takes two admins (D02).
func (h handlers) approvePriceTable(c *gin.Context) {
	version, err := strconv.ParseInt(c.Param("version"), 10, 64)
	if err != nil || version < 1 {
		problem.Abort(c, problem.New(problem.InvalidRequest, fmt.Errorf("version %q", c.Param("version"))))
		return
	}
	err = h.d.Orders.ApprovePriceTable(c.Request.Context(), version, uuid.UUID(auth.MustUser(c).ID()))
	switch {
	case errors.Is(err, orders.ErrSameApprover):
		problem.Abort(c, problem.New(problem.Forbidden, err))
		return
	case errors.Is(err, orders.ErrNotFound):
		problem.Abort(c, problem.New(problem.InvalidRequest, err))
		return
	case err != nil:
		h.abortOrderError(c, err)
		return
	}
	h.recordTable(c, "price_table.approved", version)
	c.JSON(http.StatusOK, gin.H{"version": version, "status": "active"})
}

// order loads the order of the path and applies the authorization rule.
// Someone else's order and a missing one get the same 403.
func (h handlers) order(c *gin.Context, allow func(orders.Order) error) (orders.Order, bool) {
	u := auth.MustUser(c)
	o, err := h.lookup(c)
	if err != nil && !errors.Is(err, orders.ErrNotFound) {
		problem.Abort(c, err)
		return orders.Order{}, false
	}
	if err == nil {
		err = allow(o)
	}
	if err != nil {
		if rerr := h.d.Audit.Record(audit.Event{
			Name: "authz.denied", Actor: u.ID().String(), Action: c.Request.Method + " " + c.FullPath(),
			Target: c.Param("id"), Result: "denied", Origin: c.ClientIP(),
		}); rerr != nil {
			_ = c.Error(rerr)
		}
		problem.Abort(c, problem.New(problem.Forbidden, err))
		return orders.Order{}, false
	}
	return o, true
}

// lookup treats an ID that is not a UUID like a missing order.
func (h handlers) lookup(c *gin.Context) (orders.Order, error) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return orders.Order{}, orders.ErrNotFound
	}
	return h.d.Orders.Get(c.Request.Context(), id)
}

func (h handlers) abortOrderError(c *gin.Context, err error) {
	if errors.Is(err, orders.ErrConflict) {
		problem.Abort(c, problem.New(problem.Conflict, err))
		return
	}
	problem.Abort(c, err)
}

func (h handlers) record(c *gin.Context, name string, id uuid.UUID, reason string) {
	if err := h.d.Audit.Record(audit.Event{
		Name: name, Actor: auth.MustUser(c).ID().String(), Action: name, Target: id.String(),
		Result: "ok", Reason: reason, Origin: c.ClientIP(),
	}); err != nil {
		_ = c.Error(err)
	}
}

func (h handlers) recordTable(c *gin.Context, name string, version int64) {
	if err := h.d.Audit.Record(audit.Event{
		Name: name, Actor: auth.MustUser(c).ID().String(), Action: name, Target: "price_table:" + strconv.FormatInt(version, 10),
		Result: "ok", Reason: c.GetString(justificationKey), Origin: c.ClientIP(),
	}); err != nil {
		_ = c.Error(err)
	}
}

func techOf(o orders.Order) *auth.UserID {
	if o.AssignedTechnician == nil {
		return nil
	}
	t := auth.UserID(*o.AssignedTechnician)
	return &t
}
