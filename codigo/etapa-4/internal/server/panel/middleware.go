package panel

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"

	"nuggetmachine/internal/audit"
	"nuggetmachine/internal/auth"
	"nuggetmachine/internal/problem"
)

// mfaLevel is the level of authentication of a login with password and TOTP
// (acr.loa.map of the realm).
const mfaLevel = "2"

const justificationKey = "nuggetmachine/panel.justification"

// RequireMFA accepts only tokens that prove the second factor. The password
// grant always yields acr 1, so only the browser login reaches the panel.
func RequireMFA() gin.HandlerFunc {
	return func(c *gin.Context) {
		if u := auth.MustUser(c); u.ACR() != mfaLevel {
			problem.Abort(c, problem.New(problem.MFARequired, fmt.Errorf("token with acr %q", u.ACR())))
			return
		}
		c.Next()
	}
}

// Access records every request to the panel, allowed or not (D02, C19).
func Access(a *audit.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		u := auth.MustUser(c)
		err := a.Record(audit.Event{
			Name:   "panel.access",
			Actor:  u.ID().String(),
			Action: c.Request.Method + " " + c.FullPath(),
			Target: c.Param("id"),
			Result: strconv.Itoa(c.Writer.Status()),
			Origin: c.ClientIP(),
		})
		if err != nil {
			_ = c.Error(err)
		}
	}
}

// Critical guards the operations of D02 that ask for the second factor again:
// the login must be at most maxAge old (step-up, RFC 9470), and the request
// must carry a written justification, which goes to the audit log and feeds
// the alert of Stage 6.
func Critical(maxAge time.Duration, now func() time.Time, a *audit.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		u := auth.MustUser(c)
		if u.AuthTime().IsZero() || now().Sub(u.AuthTime()) > maxAge {
			c.Header("WWW-Authenticate", fmt.Sprintf(
				`Bearer error="insufficient_user_authentication", error_description="a recent login is required", max_age=%d, acr_values="%s"`,
				int(maxAge.Seconds()), mfaLevel))
			problem.Abort(c, problem.New(problem.StepUpRequired, fmt.Errorf("login at %s, older than %s", u.AuthTime(), maxAge)))
			return
		}
		// The body is read once and put back, so the operation can read the
		// rest of it; its own decoder knows the justification field too.
		raw, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, 64<<10))
		if err != nil {
			problem.Abort(c, problem.New(problem.InvalidRequest, err))
			return
		}
		c.Request.Body = io.NopCloser(bytes.NewReader(raw))
		var body struct {
			Justification string `json:"justification"`
		}
		if err := json.Unmarshal(raw, &body); err != nil {
			problem.Abort(c, problem.New(problem.JustificationRequired, err))
			return
		}
		justification := strings.TrimSpace(body.Justification)
		if n := utf8.RuneCountInString(justification); n < 10 || n > 500 {
			problem.Abort(c, problem.New(problem.JustificationRequired, errors.New("justification must have 10 to 500 characters")))
			return
		}
		c.Set(justificationKey, justification)
		c.Next()

		err = a.Record(audit.Event{
			Name:   "panel.critical",
			Actor:  u.ID().String(),
			Action: c.Request.Method + " " + c.FullPath(),
			Target: c.Param("id"),
			Result: strconv.Itoa(c.Writer.Status()),
			Reason: justification,
			Origin: c.ClientIP(),
		})
		if err != nil {
			_ = c.Error(err)
		}
	}
}
