package partners

import "testing"

func validAddress() Address {
	return Address{Recipient: "Customer Test", Street: "Avenida Paulista", Number: "1000", City: "São Paulo", State: "SP", PostalCode: "01310-100"}
}

func TestValidAddressIsAccepted(t *testing.T) {
	if err := validAddress().Validate(); err != nil {
		t.Fatalf("a valid address was refused: %v", err)
	}
}

// A NUL byte passed the length check and reached Postgres, which refuses it:
// the order creation answered 500. Line breaks and other control characters
// would go on to the carrier and to the e-mail with the address.
func TestAddressWithControlCharactersIsRefused(t *testing.T) {
	cases := map[string]func(a *Address){
		"NUL in the recipient":    func(a *Address) { a.Recipient += "\x00x" },
		"NUL in the street":       func(a *Address) { a.Street += "\x00x" },
		"NUL in the number":       func(a *Address) { a.Number += "\x00" },
		"NUL in the city":         func(a *Address) { a.City = "São\x00Paulo" },
		"line break in the city":  func(a *Address) { a.City = "São Paulo\r\nBcc: x@example.com" },
		"escape in the recipient": func(a *Address) { a.Recipient = "Customer\x1b[2J" },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			a := validAddress()
			change(&a)
			if err := a.Validate(); err == nil {
				t.Fatal("the address was accepted")
			}
		})
	}
}
