package panel

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"nuggetmachine/internal/auth"
	"nuggetmachine/internal/orders"
	"nuggetmachine/internal/partners"
	"nuggetmachine/internal/problem"
	"nuggetmachine/internal/server"
	"nuggetmachine/internal/support"
)

var ticketStatuses = []string{"", support.StatusOpen, support.StatusAnswered, support.StatusClosed}

func (h handlers) listTickets(c *gin.Context) {
	status := c.Query("status")
	if !slices.Contains(ticketStatuses, status) {
		problem.Abort(c, problem.New(problem.InvalidRequest, fmt.Errorf("status %q", status)))
		return
	}
	list, err := h.d.Support.ListByStatus(c.Request.Context(), status)
	if err != nil {
		problem.Abort(c, err)
		return
	}
	out := make([]support.StaffTicket, 0, len(list))
	for _, t := range list {
		out = append(out, t.Staff())
	}
	c.JSON(http.StatusOK, gin.H{"tickets": out})
}

func (h handlers) getTicket(c *gin.Context) {
	t, ok := h.ticket(c)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, t.Staff())
}

// replyTicket answers the customer, who gets an e-mail saying only that there
// is an answer.
func (h handlers) replyTicket(c *gin.Context) {
	u := auth.MustUser(c)
	t, ok := h.ticket(c)
	if !ok {
		return
	}
	body, ok := support.MessageBody(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()
	err := h.d.Support.Reply(ctx, t, uuid.UUID(u.ID()), true, body)
	if errors.Is(err, support.ErrClosed) {
		problem.Abort(c, problem.New(problem.Conflict, err))
		return
	}
	if err != nil {
		problem.Abort(c, err)
		return
	}
	h.record(c, "ticket.replied", t.ID, "by staff")
	h.notices().Send(ctx, support.NoticeReply(t), t.ID.String(), c.ClientIP())
	t, err = h.d.Support.Get(ctx, t.ID)
	if err != nil {
		problem.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, t.Staff())
}

func (h handlers) closeTicket(c *gin.Context) {
	t, ok := h.ticket(c)
	if !ok {
		return
	}
	err := h.d.Support.Close(c.Request.Context(), t.ID)
	if errors.Is(err, support.ErrClosed) {
		problem.Abort(c, problem.New(problem.Conflict, err))
		return
	}
	if err != nil {
		problem.Abort(c, err)
		return
	}
	h.record(c, "ticket.closed", t.ID, "")
	c.JSON(http.StatusOK, gin.H{"id": t.ID, "status": support.StatusClosed})
}

func (h handlers) ticket(c *gin.Context) (support.Ticket, bool) {
	id, err := uuid.Parse(c.Param("id"))
	var t support.Ticket
	if err == nil {
		t, err = h.d.Support.Get(c.Request.Context(), id)
	} else {
		err = support.ErrNotFound
	}
	if errors.Is(err, support.ErrNotFound) {
		problem.Abort(c, problem.New(problem.Forbidden, err))
		return support.Ticket{}, false
	}
	if err != nil {
		problem.Abort(c, err)
		return support.Ticket{}, false
	}
	return t, true
}

// proposeAddressChange is the first half of C21 (critical: step-up and
// justification). Support never changes the address: it proposes a change,
// and the code goes to the e-mail registered with the order. The answer never
// carries the code, so a fraudster talking to support (CA04) gets nothing.
func (h handlers) proposeAddressChange(c *gin.Context) {
	u := auth.MustUser(c)
	var body struct {
		Address       partners.Address `json:"address"`
		TicketID      *uuid.UUID       `json:"ticket_id"`
		Justification string           `json:"justification"`
	}
	if err := server.DecodeJSON(c, &body); err != nil {
		problem.Abort(c, problem.New(problem.InvalidRequest, err))
		return
	}
	if err := body.Address.Validate(); err != nil {
		problem.Abort(c, problem.New(problem.InvalidRequest, err))
		return
	}
	o, ok := h.order(c, func(orders.Order) error { return nil })
	if !ok {
		return
	}
	if !orders.AddressChangeAllowed(o) {
		problem.Abort(c, problem.New(problem.Conflict, fmt.Errorf("order %s is %s: no address change after the dispatch", o.ID, o.Status)))
		return
	}
	if o.ContactEmail == "" {
		problem.Abort(c, problem.New(problem.Conflict, errors.New("the order has no registered e-mail to confirm with")))
		return
	}
	ctx := c.Request.Context()
	change, code, err := h.d.Orders.ProposeAddressChange(ctx, o.ID, uuid.UUID(u.ID()), body.TicketID, body.Address, h.d.Now())
	if err != nil {
		problem.Abort(c, err)
		return
	}
	// Without the e-mail the change could never be confirmed: it is closed
	// and the request fails.
	if h.d.Notify == nil {
		err = errors.New("no notification sender")
	} else {
		err = h.d.Notify.Send(ctx, orders.NoticeAddressCode(o, body.Address, code, change.ExpiresAt))
	}
	if err != nil {
		if werr := h.d.Orders.WithdrawAddressChange(ctx, change.ID); werr != nil {
			_ = c.Error(werr)
		}
		problem.Abort(c, problem.New(problem.Unavailable, err))
		return
	}
	h.record(c, "address.change_requested", o.ID, fmt.Sprintf("change %s, postal code %s to %s: %s",
		change.ID, orders.MaskPostalCode(o.ShipPostalCode), orders.MaskPostalCode(body.Address.PostalCode), c.GetString(justificationKey)))
	c.JSON(http.StatusAccepted, gin.H{"change_id": change.ID, "status": change.Status, "expires_at": change.ExpiresAt})
}

func (h handlers) notices() orders.Notifier {
	return orders.Notifier{Sender: h.d.Notify, Audit: h.d.Audit, Log: h.log()}
}

func (h handlers) log() *slog.Logger {
	if h.d.Log == nil {
		return slog.New(slog.DiscardHandler)
	}
	return h.d.Log
}
