package webhook

import (
	"errors"
	"strings"
	"testing"
	"time"
)

var (
	secret = []byte("partner-webhook-secret")
	now    = time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	body   = []byte(`{"event_id":"evt_1","amount_cents":1180}`)
)

func TestValidSignature(t *testing.T) {
	if err := Verify(secret, Sign(secret, now, body), body, now, Tolerance); err != nil {
		t.Fatal(err)
	}
}

func TestRefusedSignatures(t *testing.T) {
	valid := Sign(secret, now, body)
	cases := []struct {
		name   string
		header string
		body   []byte
		now    time.Time
		want   error
	}{
		{"body changed", valid, []byte(`{"event_id":"evt_1","amount_cents":1}`), now, ErrMismatch},
		{"another secret", Sign([]byte("other"), now, body), body, now, ErrMismatch},
		{"too old", Sign(secret, now.Add(-10*time.Minute), body), body, now, ErrExpired},
		{"from the future", Sign(secret, now.Add(10*time.Minute), body), body, now, ErrExpired},
		{"timestamp swapped", strings.Replace(valid, "t=", "t=1", 1), body, now, ErrExpired},
		{"empty", "", body, now, ErrMalformed},
		{"no signature", "t=" + strings.Split(strings.TrimPrefix(valid, "t="), ",")[0], body, now, ErrMalformed},
		{"not hex", "t=1794139200,v1=zz", body, time.Unix(1794139200, 0), ErrMalformed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := Verify(secret, tc.header, tc.body, tc.now, Tolerance); !errors.Is(err, tc.want) {
				t.Errorf("error %v, want %v", err, tc.want)
			}
		})
	}
}
