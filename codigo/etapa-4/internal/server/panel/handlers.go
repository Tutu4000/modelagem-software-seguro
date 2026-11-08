package panel

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
	"nuggetmachine/internal/problem"
)

type handlers struct {
	d Deps
}

// me tells the panel front-end who is logged in and whether a critical
// operation would need a new login.
func (h handlers) me(c *gin.Context) {
	u := auth.MustUser(c)
	c.JSON(http.StatusOK, gin.H{
		"id":                u.ID().String(),
		"role":              u.Role(),
		"session":           u.SessionID(),
		"authenticated_at":  u.AuthTime().UTC(),
		"step_up_needed_at": u.AuthTime().Add(h.d.StepUpMaxAge).UTC(),
	})
}

// revoke is the incident response: it cuts the user's existing tokens in every
// replica at once (Valkey), then disables the account and ends its sessions in
// Keycloak, so no refresh or new login brings it back.
func (h handlers) revoke(c *gin.Context) {
	admin := auth.MustUser(c)
	target, err := uuid.Parse(c.Param("id"))
	if err != nil {
		problem.Abort(c, problem.New(problem.InvalidRequest, err))
		return
	}
	if auth.UserID(target) == admin.ID() {
		problem.Abort(c, problem.New(problem.InvalidRequest, errors.New("an admin cannot revoke themselves")))
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()
	if err := h.d.Sessions.RevokeUser(ctx, target.String(), h.d.Now()); err != nil {
		problem.Abort(c, problem.New(problem.Unavailable, fmt.Errorf("session store: %w", err)))
		return
	}
	idpErr := h.d.IdP.LogoutAndDisable(ctx, target)

	result := "revoked"
	if idpErr != nil {
		result = "tokens_cut_idp_failed"
	}
	if err := h.d.Audit.Record(audit.Event{
		Name:   "user.revoked",
		Actor:  admin.ID().String(),
		Action: "user.revoke",
		Target: target.String(),
		Result: result,
		Reason: c.GetString(justificationKey),
		Origin: c.ClientIP(),
	}); err != nil {
		_ = c.Error(err)
	}
	if idpErr != nil {
		// The cut in Valkey already holds; the admin must retry so that the
		// account is disabled too.
		problem.Abort(c, problem.New(problem.IdPUnavailable, idpErr))
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": target.String(), "tokens": "revoked", "enabled": false, "sessions": "ended"})
}
