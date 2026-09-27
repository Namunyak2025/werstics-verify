package providers

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestC2BNormalizeConfirmation(t *testing.T) {
	payload := `{
		"TransactionType":"Pay Bill",
		"TransID":"RKL51ZDR4F",
		"TransTime":"20231121121325",
		"TransAmount":"1500.00",
		"BusinessShortCode":"600983",
		"BillRefNumber":"ORDER-1500",
		"InvoiceNumber":"",
		"OrgAccountBalance":"0",
		"ThirdPartyTransID":"",
		"MSISDN":"254712345678",
		"FirstName":"NICHOLAS",
		"MiddleName":"",
		"LastName":""
	}`

	event, err := NewC2BAdapter().Normalize(
		context.Background(),
		[]byte(payload),
	)
	if err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}

	if event.EventID != "c2b:RKL51ZDR4F" {
		t.Fatalf("EventID = %q", event.EventID)
	}

	if event.Provider != "c2b" {
		t.Fatalf("Provider = %q", event.Provider)
	}

	if event.ProviderEventID != "RKL51ZDR4F" {
		t.Fatalf("ProviderEventID = %q", event.ProviderEventID)
	}

	if event.ProviderRef != "ORDER-1500" {
		t.Fatalf("ProviderRef = %q", event.ProviderRef)
	}

	if event.MerchantID != "600983" {
		t.Fatalf("MerchantID = %q", event.MerchantID)
	}

	if event.Amount.Currency != "KES" {
		t.Fatalf("Currency = %q", event.Amount.Currency)
	}

	if event.Amount.Minor != 1500 {
		t.Fatalf("Amount.Minor = %d", event.Amount.Minor)
	}

	if event.CustomerDisplay != "NICHOLAS" {
		t.Fatalf("CustomerDisplay = %q", event.CustomerDisplay)
	}

	if event.Kind != "payment.confirmed" {
		t.Fatalf("Kind = %q", event.Kind)
	}

	wantTime := time.Date(
		2023,
		time.November,
		21,
		9,
		13,
		25,
		0,
		time.UTC,
	)

	if !event.OccurredAt.Equal(wantTime) {
		t.Fatalf(
			"OccurredAt = %s, want %s",
			event.OccurredAt,
			wantTime,
		)
	}
}

func TestC2BAcceptsNumericAmount(t *testing.T) {
	payload := `{
		"TransID":"NUMERIC1",
		"TransTime":"20231121121325",
		"TransAmount":1500,
		"BusinessShortCode":"600983",
		"BillRefNumber":"ORDER-1500"
	}`

	event, err := NewC2BAdapter().Normalize(
		context.Background(),
		[]byte(payload),
	)
	if err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}

	if event.Amount.Minor != 1500 {
		t.Fatalf("Amount.Minor = %d, want 1500", event.Amount.Minor)
	}
}

func TestC2BRejectsMissingBillRefNumber(t *testing.T) {
	payload := `{
		"TransID":"MISSINGREF",
		"TransTime":"20231121121325",
		"TransAmount":"1500",
		"BusinessShortCode":"600983"
	}`

	_, err := NewC2BAdapter().Normalize(
		context.Background(),
		[]byte(payload),
	)

	if err == nil {
		t.Fatal("Normalize() expected an error")
	}
}

func TestC2BRejectsFractionalAmount(t *testing.T) {
	payload := `{
		"TransID":"FRACTIONAL",
		"TransTime":"20231121121325",
		"TransAmount":"1500.50",
		"BusinessShortCode":"600983",
		"BillRefNumber":"ORDER-1500"
	}`

	_, err := NewC2BAdapter().Normalize(
		context.Background(),
		[]byte(payload),
	)

	if err == nil {
		t.Fatal("Normalize() expected an error")
	}

	if !strings.Contains(err.Error(), "TransAmount") {
		t.Fatalf("error = %q, expected TransAmount error", err)
	}
}

func TestC2BVerifySignatureIsNoOp(t *testing.T) {
	err := NewC2BAdapter().VerifySignature(
		context.Background(),
		[]byte(`{}`),
		map[string]string{},
	)

	if err != nil {
		t.Fatalf("VerifySignature() error = %v", err)
	}
}
