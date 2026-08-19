package domain

import "testing"

func TestValidTransitions(t *testing.T) {
	cases := []struct {
		from, to PaymentStatus
	}{
		{StatusRequested, StatusPending},
		{StatusPending, StatusConfirmed},
		{StatusConfirmed, StatusSettled},
		{StatusSettled, StatusReversed},
		{StatusReversed, StatusRefunded},
	}
	for _, tc := range cases {
		if !CanTransition(tc.from, tc.to) {
			t.Fatalf("expected transition %s -> %s to be valid", tc.from, tc.to)
		}
	}
}

func TestInvalidTransition(t *testing.T) {
	if CanTransition(StatusFailed, StatusConfirmed) {
		t.Fatal("failed payment must not become confirmed")
	}
}
