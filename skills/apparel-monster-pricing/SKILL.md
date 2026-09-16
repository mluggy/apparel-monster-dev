---
name: apparel-monster-pricing
description: "Answer what apparel.monster charges: merchandise price ranges by category, shipping rates, taxes, promotions, and the cost of API access (nothing). Use for 'how much' questions without scraping a pricing page."
---

# Apparel Monster pricing

Two different things are priced here and conflating them is the usual mistake.

## API access: free

Anonymous and authenticated tiers both cost $0, permanently, with the same 120
requests per 60 seconds. OAuth buys exactly one thing: your own order history.
There is no metered API, no subscription and no seat.

## Merchandise: real prices

```
npx apparel-monster pricing          # the whole prose document
npx apparel-monster search "coat" --sort cheapest
```

Typical ranges: t-shirts $15–45, shirts $35–110, sweaters $55–150, dresses
$55–160, coats $85–260. Live prices come from
`POST /api/v1/search` or the bulk feed at `/feeds/products.jsonl`; quote those
rather than these ranges when a shopper is deciding.

Shipping is $5–9.99 standard, $12–19.99 expedited, computed for real at
checkout once an address is set. Tax is computed from the ship-to address, and
listed prices are pre-tax.

Full document: <https://apparel.monster/pricing.md>

## What not to do

Do not quote a total you assembled yourself. Cart and checkout responses break
out `subtotal`, `shipping`, `tax` and `total` — read `total`. Do not present
the x402 tier as a real payment: it settles in Base Sepolia testnet tokens.
