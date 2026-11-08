// Package webhook signs and verifies the notifications of the partners
// (payment gateway and carrier, FC5). The header follows the Stripe format:
//
//	X-Nugget-Signature: t=<unix seconds>,v1=<hex HMAC-SHA256 of "t.body">
//
// The timestamp is signed with the body, so an old notification cannot be
// replayed later with a fresh timestamp (C14, C15).
package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const Header = "X-Nugget-Signature"

// Tolerance is how far the timestamp may be from the receiver's clock.
const Tolerance = 5 * time.Minute

var (
	ErrMalformed = errors.New("malformed signature header")
	ErrExpired   = errors.New("signature timestamp out of tolerance")
	ErrMismatch  = errors.New("signature does not match")
)

// Sign returns the header value for body at time t.
func Sign(secret []byte, t time.Time, body []byte) string {
	ts := strconv.FormatInt(t.Unix(), 10)
	return "t=" + ts + ",v1=" + hex.EncodeToString(mac(secret, ts, body))
}

// Verify checks the header against the exact bytes received, in constant
// time.
func Verify(secret []byte, header string, body []byte, now time.Time, tolerance time.Duration) error {
	var ts, sig string
	for _, part := range strings.Split(header, ",") {
		k, v, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			return ErrMalformed
		}
		switch k {
		case "t":
			ts = v
		case "v1":
			sig = v
		}
	}
	if ts == "" || sig == "" {
		return ErrMalformed
	}
	unix, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return ErrMalformed
	}
	if d := now.Sub(time.Unix(unix, 0)); d > tolerance || d < -tolerance {
		return fmt.Errorf("%w: %s off", ErrExpired, d.Round(time.Second))
	}
	got, err := hex.DecodeString(sig)
	if err != nil {
		return ErrMalformed
	}
	if !hmac.Equal(got, mac(secret, ts, body)) {
		return ErrMismatch
	}
	return nil
}

func mac(secret []byte, ts string, body []byte) []byte {
	h := hmac.New(sha256.New, secret)
	h.Write([]byte(ts))
	h.Write([]byte("."))
	h.Write(body)
	return h.Sum(nil)
}
