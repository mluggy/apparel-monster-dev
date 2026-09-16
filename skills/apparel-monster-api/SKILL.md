---
name: apparel-monster-api
description: "Integrate with the apparel.monster agent commerce API: endpoints, the session model, batching, streaming, rate limits, error shapes and the SDKs. Use when writing code against the store rather than shopping on it."
---

# Apparel Monster API integration

Base: `https://apparel.monster/api/v1`. OpenAPI 3.2:
`https://apparel.monster/api/v1/openapi.yaml`. No credentials except `track`.

## Install a client

```
npm install apparel-monster
pip install apparel-monster
gem install apparel-monster
go get github.com/mluggy/apparel-monster-dev/go
```

All four are standard library only and carry the session id for you.

## The shape of it

- Every stateful call carries `session.id`, minted by the first `search`.
- `POST /cart` takes an `action`: add, update, remove, view, billing, shipping,
  coupon, payment.
- `POST /batch` runs up to 10 operations in one round trip, in order. Each
  result has its own status; rate limit counts per operation.
- `POST /search` and `POST /checkout` stream with `Accept: text/event-stream`.
- `GET /ask` is NLWeb: a question in, schema.org results out, `streaming=true`
  for SSE.
- 120 requests per 60s per IP, with `RateLimit-*` and `X-RateLimit-*` headers
  and `Retry-After` on a 429.

## Errors are actionable

A failure names the fields it needed: `error: "missing_fields"` with a
`missing[]` array giving each field, every alias it is accepted under, and an
example. A cart in the wrong state answers `checkout_blocked` with `next_call`
— the call that unblocks it. Read those rather than guessing.

## Versioning

Path-versioned. Additive changes ship inside v1 without notice, so ignore
fields you do not recognise. A deprecation announces itself with `Deprecation`
and `Sunset` headers at least 180 days ahead:
`https://apparel.monster/docs/deprecation-policy.md`.

## What not to do

Do not poll `/track` for an order you placed anonymously — it needs OAuth and
reads only the token owner's own orders. Use the order number from checkout, or
the `order_status` URL it returns.
