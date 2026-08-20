package verification

import (
	"testing"

	"github.com/Namunyak2025/werstics-verify/backend/internal/domain"
)

func TestMatch(t *testing.T) {
	p := domain.Payment{
		ID:             "pay_test_001",
		OrganizationID: "org_test_001",
		MerchantID:     "merchant_test_001",
		Expected: domain.Money{
			Currency: "KES",
			Minor:    1500,
		},
	}

	event := domain.PaymentEvent{
		EventID:    "evt_test_001",
		MerchantID: "merchant_test_001",
		Amount: domain.Money{
			Currency: "KES",
			Minor:    1500,
		},
	}

	result := Match(p, event)

	if !result.Matched {
		t.Fatalf("expected payment to match, got reason: %s", result.Reason)
	}

	if !result.AmountMatched {
		t.Fatal("expected amount to match")
	}

	if !result.MerchantMatch {
		t.Fatal("expected merchant to match")
	}
}

func TestMatchRejectsWrongMerchant(t *testing.T) {
	p := domain.Payment{
		ID:             "pay_test_001",
		OrganizationID: "org_test_001",
		MerchantID:     "merchant_test_001",
		Expected: domain.Money{
			Currency: "KES",
			Minor:    1500,
		},
	}

	event := domain.PaymentEvent{
		EventID:    "evt_test_002",
		MerchantID: "wrong_merchant",
		Amount: domain.Money{
			Currency: "KES",
			Minor:    1500,
		},
	}

	result := Match(p, event)

	if result.Matched {
		t.Fatal("expected payment not to match")
	}

	if result.MerchantMatch {
		t.Fatal("expected merchant mismatch")
	}

	if result.Reason != "payment belongs to a different merchant" {
		t.Fatalf("unexpected reason: %s", result.Reason)
	}
}

func TestMatchRejectsWrongAmount(t *testing.T) {
	p := domain.Payment{
		ID:             "pay_test_001",
		OrganizationID: "org_test_001",
		MerchantID:     "merchant_test_001",
		Expected: domain.Money{
			Currency: "KES",
			Minor:    1500,
		},
	}

	event := domain.PaymentEvent{
		EventID:    "evt_test_003",
		MerchantID: "merchant_test_001",
		Amount: domain.Money{
			Currency: "KES",
			Minor:    999,
		},
	}

	result := Match(p, event)

	if result.Matched {
		t.Fatal("expected payment not to match")
	}

	if result.AmountMatched {
		t.Fatal("expected amount mismatch")
	}

	if result.Reason != "received amount does not match expected amount" {
		t.Fatalf("unexpected reason: %s", result.Reason)
	}
}
