# ADR-003: WhatsApp Cloud API for Notifications

## Status

Accepted

## Context

Order status updates need to reach customers. In India, WhatsApp is the
dominant messaging platform. Email open rates are low.

Options:

- **Email only**: low engagement in India
- **Twilio WhatsApp**: easy SDK, expensive at scale
- **WhatsApp Cloud API (Meta)**: free tier (1,000 conversations/month), official

## Decision

Use WhatsApp Cloud API (Meta) for order status notifications.

## Consequences

- Free for low volume, scales with Meta's pricing
- Requires Meta Business verification and template approval
- Message templates must be pre-approved for transactional messages
- Promotional messages require customer opt-in
- No vendor lock-in beyond Meta's API terms
