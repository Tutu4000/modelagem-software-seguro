package session

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"time"

	"nuggetmachine/internal/audit"
	"nuggetmachine/internal/auth"
)

// logoutMaxAge is how old a logout token may be. Keycloak sends it right
// after the logout.
const logoutMaxAge = 2 * time.Minute

// LogoutHandler receives the OIDC back-channel logout from Keycloak and
// revokes the session in every replica. It is served only on the internal
// listener, never through nginx.
type LogoutHandler struct {
	Verifier *auth.Verifier
	// Clients are the Keycloak clients allowed to ask for a logout.
	Clients []string
	Store   Store
	Audit   *audit.Logger
	Log     *slog.Logger
}

func (h LogoutHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	token, err := h.Verifier.VerifyLogout(r.PostForm.Get("logout_token"), h.Clients, logoutMaxAge)
	if err != nil {
		h.Log.Warn("logout token refused", "error", err)
		http.Error(w, "invalid logout token", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), storeTimeout)
	defer cancel()
	first, err := h.Store.FirstUse(ctx, token.ID, RevocationTTL)
	if err != nil {
		h.Log.Error("session store unavailable", "error", err)
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	}
	if !first {
		h.Log.Warn("logout token replayed", "jti", token.ID)
		http.Error(w, "invalid logout token", http.StatusBadRequest)
		return
	}
	if err := h.Store.RevokeSession(ctx, token.SessionID, RevocationTTL); err != nil {
		h.Log.Error("session store unavailable", "error", err)
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	}

	origin, _, _ := net.SplitHostPort(r.RemoteAddr)
	if err := h.Audit.Record(audit.Event{
		Name:   "token.revoked",
		Actor:  token.Subject,
		Action: "session.logout",
		Target: token.SessionID,
		Result: "revoked",
		Origin: origin,
	}); err != nil {
		h.Log.Error("failed to audit the logout", "error", err)
	}
	w.WriteHeader(http.StatusOK)
}
