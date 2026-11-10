// Package session keeps the state the API replicas share: revoked sessions
// and per-user rate limits (E5.1, E5.2). It fails closed: when the store does
// not answer, the request is refused instead of trusting the token (D03).
package session

import (
	"context"
	"sync"
	"time"
)

// RevocationTTL outlives every access token issued before a revocation: the
// tokens last 5 minutes, and the rest is margin for clock skew.
const RevocationTTL = 10 * time.Minute

type Store interface {
	// RevokeSession refuses every token of one Keycloak session.
	RevokeSession(ctx context.Context, sid string, ttl time.Duration) error
	// RevokeUser refuses every token of the user issued before cutoff, in any
	// session. It is the incident response of the panel (F3).
	RevokeUser(ctx context.Context, sub string, cutoff time.Time) error
	// Revoked answers both checks in one round trip.
	Revoked(ctx context.Context, sid, sub string, issuedAt time.Time) (bool, error)
	// FirstUse is true only the first time an ID is seen, which stops a
	// logout token from being replayed.
	FirstUse(ctx context.Context, id string, ttl time.Duration) (bool, error)
}

type Decision struct {
	Allowed    bool
	RetryAfter time.Duration
	// FirstRejection is true only for the first refused request of a window,
	// so that a flood creates one audit event and not thousands.
	FirstRejection bool
}

type Limiter interface {
	Allow(ctx context.Context, key string, limit int, window time.Duration) (Decision, error)
}

// Memory implements Store and Limiter for the tests.
type Memory struct {
	mu       sync.Mutex
	now      func() time.Time
	sessions map[string]time.Time // sid -> expiry
	users    map[string]time.Time // sub -> cutoff
	seen     map[string]bool
	counts   map[string]int
	flagged  map[string]bool
	// Fail makes every call return this error, to test the fail-closed path.
	Fail error
}

func NewMemory(now func() time.Time) *Memory {
	return &Memory{
		now:      now,
		sessions: map[string]time.Time{},
		users:    map[string]time.Time{},
		seen:     map[string]bool{},
		counts:   map[string]int{},
		flagged:  map[string]bool{},
	}
}

func (m *Memory) RevokeSession(_ context.Context, sid string, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Fail != nil {
		return m.Fail
	}
	m.sessions[sid] = m.now().Add(ttl)
	return nil
}

func (m *Memory) RevokeUser(_ context.Context, sub string, cutoff time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Fail != nil {
		return m.Fail
	}
	m.users[sub] = cutoff
	return nil
}

func (m *Memory) Revoked(_ context.Context, sid, sub string, issuedAt time.Time) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Fail != nil {
		return false, m.Fail
	}
	if exp, ok := m.sessions[sid]; ok && m.now().Before(exp) {
		return true, nil
	}
	cutoff, ok := m.users[sub]
	return ok && issuedAt.Before(cutoff), nil
}

func (m *Memory) FirstUse(_ context.Context, id string, _ time.Duration) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Fail != nil {
		return false, m.Fail
	}
	if m.seen[id] {
		return false, nil
	}
	m.seen[id] = true
	return true, nil
}

// Allow counts in fixed windows. The Valkey limiter smooths the window edges;
// the tests only need the limit itself.
func (m *Memory) Allow(_ context.Context, key string, limit int, window time.Duration) (Decision, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Fail != nil {
		return Decision{}, m.Fail
	}
	now := m.now()
	slot := key + "@" + now.Truncate(window).String()
	m.counts[slot]++
	if m.counts[slot] <= limit {
		return Decision{Allowed: true}, nil
	}
	first := !m.flagged[slot]
	m.flagged[slot] = true
	return Decision{RetryAfter: now.Truncate(window).Add(window).Sub(now), FirstRejection: first}, nil
}
