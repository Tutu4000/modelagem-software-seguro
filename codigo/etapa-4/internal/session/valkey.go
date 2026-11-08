package session

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/valkey-io/valkey-go"
)

// Valkey implements Store and Limiter. The ACL of the "api" user only allows
// the session:* and ratelimit:* keys and the commands used here.
type Valkey struct {
	c valkey.Client
}

func NewValkey(addr, user string, password []byte) (*Valkey, error) {
	c, err := valkey.NewClient(valkey.ClientOption{
		InitAddress: []string{addr},
		Username:    user,
		Password:    string(password),
		// Client-side caching needs CLIENT TRACKING, which the ACL does not
		// grant, and would keep a revocation from showing up at once.
		DisableCache: true,
	})
	if err != nil {
		return nil, err
	}
	return &Valkey{c: c}, nil
}

func (v *Valkey) Close() { v.c.Close() }

func (v *Valkey) RevokeSession(ctx context.Context, sid string, ttl time.Duration) error {
	return v.c.Do(ctx, v.c.B().Set().Key("session:sid:"+sid).Value("1").Px(ttl).Build()).Error()
}

func (v *Valkey) RevokeUser(ctx context.Context, sub string, cutoff time.Time) error {
	value := strconv.FormatInt(cutoff.UnixMilli(), 10)
	return v.c.Do(ctx, v.c.B().Set().Key("session:sub:"+sub).Value(value).Px(RevocationTTL).Build()).Error()
}

// Revoked reads the session and the user keys with one MGET. The token's iat
// has second precision, so a token issued in the same second as the cutoff is
// refused too: the safe side.
func (v *Valkey) Revoked(ctx context.Context, sid, sub string, issuedAt time.Time) (bool, error) {
	values, err := v.c.Do(ctx, v.c.B().Mget().Key("session:sid:"+sid, "session:sub:"+sub).Build()).ToArray()
	if err != nil {
		return false, err
	}
	if len(values) != 2 {
		return false, errors.New("unexpected MGET reply")
	}
	if !values[0].IsNil() {
		return true, nil
	}
	if values[1].IsNil() {
		return false, nil
	}
	cutoff, err := values[1].AsInt64()
	if err != nil {
		return false, err
	}
	return issuedAt.Before(time.UnixMilli(cutoff)), nil
}

func (v *Valkey) FirstUse(ctx context.Context, id string, ttl time.Duration) (bool, error) {
	err := v.c.Do(ctx, v.c.B().Set().Key("session:jti:"+id).Value("1").Nx().Px(ttl).Build()).Error()
	if valkey.IsValkeyNil(err) {
		return false, nil
	}
	return err == nil, err
}

// slidingWindow approximates a sliding window with two fixed windows: the
// previous one counts in proportion to how much of it still overlaps the last
// minute. The third key marks the first rejection of the window.
//
// KEYS: current window, previous window, rejection flag.
// ARGV: limit, window in ms, ms elapsed in the current window.
var slidingWindow = valkey.NewLuaScript(`
local current = redis.call('INCR', KEYS[1])
if current == 1 then
  redis.call('PEXPIRE', KEYS[1], ARGV[2] * 2)
end
local previous = tonumber(redis.call('GET', KEYS[2]) or '0')
local window = tonumber(ARGV[2])
local estimate = previous * (window - tonumber(ARGV[3])) / window + current
if estimate <= tonumber(ARGV[1]) then
  return {1, 0}
end
local first = redis.call('SET', KEYS[3], '1', 'NX', 'PX', window * 2)
if first then
  return {0, 1}
end
return {0, 0}
`)

func (v *Valkey) Allow(ctx context.Context, key string, limit int, window time.Duration) (Decision, error) {
	now := time.Now()
	ms := window.Milliseconds()
	slot := now.UnixMilli() / ms
	elapsed := now.UnixMilli() % ms
	prefix := "ratelimit:" + key + ":"
	reply, err := slidingWindow.Exec(ctx, v.c,
		[]string{prefix + strconv.FormatInt(slot, 10), prefix + strconv.FormatInt(slot-1, 10), prefix + strconv.FormatInt(slot, 10) + ":flagged"},
		[]string{strconv.Itoa(limit), strconv.FormatInt(ms, 10), strconv.FormatInt(elapsed, 10)},
	).AsIntSlice()
	if err != nil {
		return Decision{}, err
	}
	if len(reply) != 2 {
		return Decision{}, errors.New("unexpected rate limit reply")
	}
	if reply[0] == 1 {
		return Decision{Allowed: true}, nil
	}
	return Decision{RetryAfter: time.Duration(ms-elapsed) * time.Millisecond, FirstRejection: reply[1] == 1}, nil
}
