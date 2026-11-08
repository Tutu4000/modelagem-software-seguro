package orders

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"nuggetmachine/internal/partners"
)

func TestMaskPostalCode(t *testing.T) {
	for in, want := range map[string]string{"01310-100": "01310-***", "01310100": "01310-***", "123": "***"} {
		if got := MaskPostalCode(in); got != want {
			t.Errorf("MaskPostalCode(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestChangedFields(t *testing.T) {
	a := partners.Address{Recipient: "A", Street: "Rua 1", Number: "1", City: "SP", State: "SP", PostalCode: "01310-100"}
	b := a
	b.Street, b.PostalCode = "Rua 2", "01304-000"
	if got := ChangedFields(a, b); !slices.Equal(got, []string{"street", "postal_code"}) {
		t.Errorf("ChangedFields = %v", got)
	}
}

func TestCodeIsBoundToItsChange(t *testing.T) {
	code, err := newCode()
	if err != nil || len(code) != codeDigits {
		t.Fatalf("newCode() = %q, %v", code, err)
	}
	one, other := uuid.New(), uuid.New()
	if string(codeHash(one, code)) == string(codeHash(other, code)) {
		t.Error("the same code gives the same hash in two changes")
	}
}

// E8.4: only the address change e-mail shows the address, to whom confirms.
func TestNoticesDoNotCarryTheAddress(t *testing.T) {
	tracking := "NG12345BR"
	o := Order{ID: uuid.New(), ContactEmail: "customer-a@nugget.local", TrackingCode: &tracking,
		ShipRecipient: "Customer Test", ShipStreet: "Avenida Paulista", ShipNumber: "1000", ShipPostalCode: "01310-100"}
	for _, m := range []string{
		NoticeReview(o, true).Text, NoticeReview(o, false).Text, NoticePaid(o).Text, NoticeShipped(o).Text,
		NoticeDelivered(o).Text, NoticeShipmentProblem(o).Text, NoticeCancelled(o, nil).Text, NoticeAddressChanged(o).Text,
	} {
		if strings.Contains(m, "Paulista") || strings.Contains(m, "01310") {
			t.Errorf("notice carries the address: %q", m)
		}
	}
	confirm := NoticeAddressCode(o, AddressOf(o), "12345678", time.Now())
	if !strings.Contains(confirm.Text, "Paulista") || !strings.Contains(confirm.Text, "12345678") {
		t.Errorf("the confirmation must show the new address and the code: %q", confirm.Text)
	}
	if err := confirm.Validate(); err != nil {
		t.Errorf("invalid confirmation message: %v", err)
	}
}

func TestCancellableUpToPaid(t *testing.T) {
	for s, want := range map[Status]bool{
		StatusQuoted: true, StatusAwaitingReview: true, StatusAwaitingPayment: true, StatusPaid: true,
		StatusInProduction: false, StatusShipped: false, StatusDelivered: false, StatusRejected: false, StatusCancelled: false,
	} {
		if Cancellable(s) != want {
			t.Errorf("Cancellable(%s) = %v, want %v", s, !want, want)
		}
	}
}
