package orders

import "testing"

func TestTransitions(t *testing.T) {
	allowed := [][2]Status{
		{StatusQuoted, StatusAwaitingReview},
		{StatusQuoted, StatusCancelled},
		{StatusAwaitingReview, StatusAwaitingPayment},
		{StatusAwaitingReview, StatusRejected},
		{StatusAwaitingPayment, StatusPaid},
		{StatusPaid, StatusInProduction},
		{StatusInProduction, StatusShipped},
		{StatusShipped, StatusDelivered},
	}
	for _, tr := range allowed {
		if !CanTransition(tr[0], tr[1]) {
			t.Errorf("%s -> %s refused", tr[0], tr[1])
		}
	}

	refused := [][2]Status{
		// Skipping the review or the payment (CA05).
		{StatusQuoted, StatusAwaitingPayment},
		{StatusQuoted, StatusPaid},
		{StatusAwaitingReview, StatusPaid},
		// Going back, or reopening a closed order.
		{StatusAwaitingPayment, StatusAwaitingReview},
		{StatusRejected, StatusAwaitingPayment},
		{StatusCancelled, StatusQuoted},
		{StatusDelivered, StatusShipped},
		// Cancelling once the part is being made.
		{StatusInProduction, StatusCancelled},
		{StatusQuoted, StatusQuoted},
	}
	for _, tr := range refused {
		if CanTransition(tr[0], tr[1]) {
			t.Errorf("%s -> %s allowed", tr[0], tr[1])
		}
	}
}
