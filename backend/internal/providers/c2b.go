package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Namunyak2025/werstics-verify/backend/internal/domain"
)

const (
	c2BDateTimeLayout = "20060102150405"
	c2BTimezoneOffset = 3 * 60 * 60
)

// C2BAdapter normalizes Safaricom Daraja C2B payloads into the
// provider-neutral Werstics Verify payment-event model.
type C2BAdapter struct{}

func NewC2BAdapter() *C2BAdapter {
	return &C2BAdapter{}
}

func (a *C2BAdapter) Name() string {
	return "c2b"
}

func (a *C2BAdapter) VerifySignature(
	_ context.Context,
	_ []byte,
	_ map[string]string,
) error {
	return nil
}

type c2BPayload struct {
	TransactionType   string          `json:"TransactionType"`
	TransID           string          `json:"TransID"`
	TransTime         string          `json:"TransTime"`
	TransAmount       json.RawMessage `json:"TransAmount"`
	BusinessShortCode string          `json:"BusinessShortCode"`
	BillRefNumber     string          `json:"BillRefNumber"`
	InvoiceNumber     string          `json:"InvoiceNumber"`
	OrgAccountBalance string          `json:"OrgAccountBalance"`
	ThirdPartyTransID string          `json:"ThirdPartyTransID"`
	MSISDN            string          `json:"MSISDN"`
	FirstName         string          `json:"FirstName"`
	MiddleName        string          `json:"MiddleName"`
	LastName          string          `json:"LastName"`
}

func (a *C2BAdapter) Normalize(
	_ context.Context,
	payload []byte,
) (domain.PaymentEvent, error) {
	var input c2BPayload

	if err := json.Unmarshal(payload, &input); err != nil {
		return domain.PaymentEvent{}, fmt.Errorf(
			"%w: %v",
			ErrMalformedPayload,
			err,
		)
	}

	input.TransactionType = strings.TrimSpace(input.TransactionType)
	input.TransID = strings.TrimSpace(input.TransID)
	input.TransTime = strings.TrimSpace(input.TransTime)
	input.BusinessShortCode = strings.TrimSpace(input.BusinessShortCode)
	input.BillRefNumber = strings.TrimSpace(input.BillRefNumber)

	if input.TransID == "" ||
		input.TransTime == "" ||
		input.BusinessShortCode == "" ||
		input.BillRefNumber == "" {
		return domain.PaymentEvent{}, ErrMalformedPayload
	}

	amount, err := parseC2BAmount(input.TransAmount)
	if err != nil {
		return domain.PaymentEvent{}, err
	}

	occurredAt, err := parseC2BTime(input.TransTime)
	if err != nil {
		return domain.PaymentEvent{}, err
	}

	return domain.PaymentEvent{
		EventID:         "c2b:" + input.TransID,
		Provider:        a.Name(),
		ProviderEventID: input.TransID,
		ProviderRef:     input.BillRefNumber,
		MerchantID:      input.BusinessShortCode,
		Amount: domain.Money{
			Currency: "KES",
			Minor:    amount,
		},
		CustomerDisplay: c2BCustomerDisplay(input),
		Kind:            "payment.confirmed",
		OccurredAt:      occurredAt,
	}, nil
}

func parseC2BAmount(raw json.RawMessage) (int64, error) {
	value := strings.TrimSpace(string(raw))

	if value == "" || value == "null" {
		return 0, fmt.Errorf(
			"%w: TransAmount is required",
			ErrMalformedPayload,
		)
	}

	if strings.HasPrefix(value, `"`) {
		var decoded string

		if err := json.Unmarshal(raw, &decoded); err != nil {
			return 0, fmt.Errorf(
				"%w: invalid TransAmount",
				ErrMalformedPayload,
			)
		}

		value = strings.TrimSpace(decoded)
	}

	if strings.Contains(value, ".") {
		parts := strings.Split(value, ".")

		if len(parts) != 2 ||
			parts[0] == "" ||
			strings.Trim(parts[1], "0") != "" {
			return 0, fmt.Errorf(
				"%w: TransAmount must be a whole KES amount",
				ErrMalformedPayload,
			)
		}

		value = parts[0]
	}

	amount, err := strconv.ParseInt(value, 10, 64)

	if err != nil || amount <= 0 {
		return 0, fmt.Errorf(
			"%w: invalid TransAmount",
			ErrMalformedPayload,
		)
	}

	return amount, nil
}

func parseC2BTime(value string) (time.Time, error) {
	location := time.FixedZone("EAT", c2BTimezoneOffset)

	parsed, err := time.ParseInLocation(
		c2BDateTimeLayout,
		value,
		location,
	)

	if err != nil {
		return time.Time{}, fmt.Errorf(
			"%w: invalid TransTime",
			ErrMalformedPayload,
		)
	}

	return parsed.UTC(), nil
}

func c2BCustomerDisplay(input c2BPayload) string {
	parts := make([]string, 0, 3)

	for _, part := range []string{
		strings.TrimSpace(input.FirstName),
		strings.TrimSpace(input.MiddleName),
		strings.TrimSpace(input.LastName),
	} {
		if part != "" {
			parts = append(parts, part)
		}
	}

	if len(parts) > 0 {
		return strings.Join(parts, " ")
	}

	return strings.TrimSpace(input.MSISDN)
}
