package devlogin

import (
	"testing"
	"time"
)

// Test vector from RFC 6238, appendix B (SHA-1, 8 digits): 94287082. The last
// 6 digits are what a 6-digit policy shows.
func TestTOTPMatchesRFC6238(t *testing.T) {
	got := TOTP([]byte("12345678901234567890"), time.Unix(59, 0))
	if got != "287082" {
		t.Errorf("TOTP %s, want 287082", got)
	}
}
