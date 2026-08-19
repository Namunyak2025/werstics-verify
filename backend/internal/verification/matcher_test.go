package verification

import (
	"testing"
	"time"

	"github.com/Namunyak2025/werstics-verify/backend/internal/domain"
)

func TestMatch(t *testing.T) {
	p := domain.Payment{
		ID: "p1", MerchantID: "m1", Provider: "mpesa",
		Expected: domain.Money{Currency: "KES", Minor: 250000},
		Status: domain.StatusRequested,
	}
	e := domain.PaymentEvent{
		Provider: "mpesa", PaymentID: "p1", MerchantID: "m1",
		Amount: domain.Money{Currency: "KES", Minor: 250000},
		Kind: "payment.confirmed", OccurredAt: time.Now(),
	}
	r := Match(p, e)
	if !r.Matched {
		t.Fatalf("expected payment to match: %s", r.Reason)
	}
}
