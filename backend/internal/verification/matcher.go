package verification

import "github.com/Namunyak2025/werstics-verify/backend/internal/domain"

type MatchResult struct {
	Matched       bool   `json:"matched"`
	AmountMatched bool   `json:"amount_matched"`
	MerchantMatch bool   `json:"merchant_matched"`
	Reason        string `json:"reason"`
}

func Match(payment domain.Payment, event domain.PaymentEvent) MatchResult {
	result := MatchResult{
		MerchantMatch: payment.MerchantID == event.MerchantID,
		AmountMatched: payment.Expected.Currency == event.Amount.Currency &&
			payment.Expected.Minor == event.Amount.Minor,
	}

	switch {
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
