package verification

import "github.com/Namunyak2025/werstics-verify/backend/internal/domain"

type MatchResult struct {
	Matched          bool   `json:"matched"`
	ProviderMatched  bool   `json:"provider_matched"`
	ProviderRefMatch bool   `json:"provider_ref_matched"`
	AmountMatched    bool   `json:"amount_matched"`
	MerchantMatch    bool   `json:"merchant_matched"`
	Reason           string `json:"reason"`
}

func Match(payment domain.Payment, event domain.PaymentEvent) MatchResult {
	result := MatchResult{
		ProviderMatched: payment.Provider == event.Provider,
		AmountMatched: payment.Expected.Currency == event.Amount.Currency &&
			payment.Expected.Minor == event.Amount.Minor,
		MerchantMatch: payment.MerchantID == event.MerchantID,
	}

	// A configured payment provider reference is authoritative.
	// When the payment has one, the incoming event must carry the
	// same reference. When the payment has none, the event reference
	// is not required for a match.
	result.ProviderRefMatch = payment.ProviderRef == "" ||
		payment.ProviderRef == event.ProviderRef

	switch {
	case !result.ProviderMatched:
		result.Reason = "payment belongs to a different provider"
	case !result.ProviderRefMatch:
		result.Reason = "provider reference does not match expected payment"
	case !result.MerchantMatch:
		result.Reason = "payment belongs to a different merchant"
	case !result.AmountMatched:
		result.Reason = "received amount does not match expected amount"
	default:
		result.Matched = true
		result.Reason = "payment matched"
	}

	return result
}
