package notify

import (
	"strings"
	"testing"
)

func TestValidate(t *testing.T) {
	ok := New("customer-a@nugget.local", "Pedido pago", "O pagamento foi confirmado.")
	if err := ok.Validate(); err != nil {
		t.Fatalf("valid message refused: %v", err)
	}
	for name, change := range map[string]func(*Message){
		"header injection in the address": func(m *Message) { m.To = "a@nugget.local\r\nBcc: x@evil.test" },
		"header injection in the subject": func(m *Message) { m.Subject = "Oi\nBcc: x@evil.test" },
		"address with a display name":     func(m *Message) { m.To = "Eve <eve@evil.test>" },
		"two addresses":                   func(m *Message) { m.To = "a@nugget.local, b@nugget.local" },
		"not an address":                  func(m *Message) { m.To = "nugget.local" },
		"empty subject":                   func(m *Message) { m.Subject = "" },
		"text too long":                   func(m *Message) { m.Text = strings.Repeat("a", maxText+1) },
		"invalid UTF-8":                   func(m *Message) { m.Text = "\xff" },
		"no ID":                           func(m *Message) { m.ID = [16]byte{} },
	} {
		m := ok
		change(&m)
		if err := m.Validate(); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}
