# Werstics Verify Architecture

## Principle

The payment provider is the source of financial truth. Werstics Verify is the verification, orchestration, notification and operational layer.

## Core flow

Customer -> payment rail -> provider -> trusted provider event -> Werstics Verify -> deterministic matcher -> transaction state -> authorized employee notification.

## Privacy boundary

The employee receives operational payment information required to complete a sale. The system should not expose customer account balances, unrelated transactions, PINs, CVVs, credentials or full private banking/mobile-money messages.

## Provider adapters

Each payment provider will implement a common adapter contract for:

- payment initiation where supported
- webhook/event ingestion
- event verification/authentication
- transaction normalization
- status/reversal/refund events

Provider-specific payloads must not leak into the core domain model.

## Future interfaces

- PWA/web dashboard
- SMS
- USSD
- QR
- provider payment authorization prompts
- APIs for merchant integrations

## AI boundary

Werstics AI may provide analytics, anomaly detection and operational assistance. It cannot be the authoritative source for whether money was received.
