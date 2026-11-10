package support

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"nuggetmachine/internal/audit"
	"nuggetmachine/internal/auth"
	"nuggetmachine/internal/problem"
	"nuggetmachine/internal/server"
)

// Handler serves the customer routes of the support channel.
type Handler struct {
	Store *Store
	Audit *audit.Logger
}

type openRequest struct {
	OrderID *uuid.UUID `json:"order_id"`
	Subject string     `json:"subject"`
	Message string     `json:"message"`
}

func (h *Handler) Open(c *gin.Context) {
	user := auth.MustUser(c)
	var req openRequest
	if err := server.DecodeJSON(c, &req); err != nil {
		problem.Abort(c, problem.New(problem.InvalidRequest, err))
		return
	}
	subject, err := CleanText(req.Subject, maxSubject)
	if err == nil {
		req.Message, err = CleanText(req.Message, maxBody)
	}
	if err != nil {
		problem.Abort(c, problem.New(problem.InvalidRequest, err))
		return
	}
	if user.Email() == "" {
		problem.Abort(c, problem.New(problem.InvalidRequest, errors.New("the account has no verified e-mail")))
		return
	}
	t, err := h.Store.Open(c.Request.Context(), uuid.UUID(user.ID()), user.Email(), req.OrderID, subject, req.Message)
	switch {
	case errors.Is(err, ErrNotYourOrder):
		h.denied(c, user, "ticket.open", req.OrderID.String())
		problem.Abort(c, problem.New(problem.Forbidden, err))
		return
	case errors.Is(err, ErrTooManyOpen):
		problem.Abort(c, problem.New(problem.Conflict, err))
		return
	case err != nil:
		problem.Abort(c, err)
		return
	}
	h.record(c, "ticket.created", t.ID, "")
	c.JSON(http.StatusCreated, t)
}

func (h *Handler) List(c *gin.Context) {
	user := auth.MustUser(c)
	list, err := h.Store.ListForOwner(c.Request.Context(), uuid.UUID(user.ID()))
	if err != nil {
		problem.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"tickets": list})
}

func (h *Handler) Get(c *gin.Context) {
	t, ok := h.own(c)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, t)
}

type messageRequest struct {
	Message string `json:"message"`
}

func (h *Handler) Reply(c *gin.Context) {
	user := auth.MustUser(c)
	t, ok := h.own(c)
	if !ok {
		return
	}
	body, ok := MessageBody(c)
	if !ok {
		return
	}
	err := h.Store.Reply(c.Request.Context(), t, uuid.UUID(user.ID()), false, body)
	if errors.Is(err, ErrClosed) {
		problem.Abort(c, problem.New(problem.Conflict, err))
		return
	}
	if err != nil {
		problem.Abort(c, err)
		return
	}
	h.record(c, "ticket.replied", t.ID, "by the customer")
	t, err = h.Store.Get(c.Request.Context(), t.ID)
	if err != nil {
		problem.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, t)
}

// MessageBody decodes and cleans a reply, for the customer and the panel.
func MessageBody(c *gin.Context) (string, bool) {
	var req messageRequest
	if err := server.DecodeJSON(c, &req); err != nil {
		problem.Abort(c, problem.New(problem.InvalidRequest, err))
		return "", false
	}
	body, err := CleanText(req.Message, maxBody)
	if err != nil {
		problem.Abort(c, problem.New(problem.InvalidRequest, err))
		return "", false
	}
	return body, true
}

// own finds a ticket of the user. Someone else's and a missing one get the
// same 403, as with files and orders.
func (h *Handler) own(c *gin.Context) (Ticket, bool) {
	user := auth.MustUser(c)
	id, err := uuid.Parse(c.Param("id"))
	var t Ticket
	if err == nil {
		t, err = h.Store.GetForOwner(c.Request.Context(), uuid.UUID(user.ID()), id)
	} else {
		err = ErrNotFound
	}
	if errors.Is(err, ErrNotFound) {
		h.denied(c, user, "ticket.read", c.Param("id"))
		problem.Abort(c, problem.New(problem.Forbidden, err))
		return Ticket{}, false
	}
	if err != nil {
		problem.Abort(c, err)
		return Ticket{}, false
	}
	return t, true
}

func (h *Handler) denied(c *gin.Context, user auth.User, action, target string) {
	if err := h.Audit.Record(audit.Event{
		Name: "authz.denied", Actor: user.ID().String(), Action: action, Target: target, Result: "denied", Origin: c.ClientIP(),
	}); err != nil {
		_ = c.Error(err)
	}
}

func (h *Handler) record(c *gin.Context, name string, id uuid.UUID, reason string) {
	user := auth.MustUser(c)
	if err := h.Audit.Record(audit.Event{
		Name: name, Actor: user.ID().String(), Action: name, Target: id.String(), Result: "ok", Reason: reason, Origin: c.ClientIP(),
	}); err != nil {
		_ = c.Error(err)
	}
}
