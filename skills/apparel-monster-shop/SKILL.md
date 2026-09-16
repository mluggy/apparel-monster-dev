---
name: apparel-monster-shop
description: "Buy something from apparel.monster on a shopper's behalf: search, add a variant, set an address, pick shipping, pay, place the order. Use when the user wants to actually purchase, not just browse."
---

# Buying from Apparel Monster

Six calls, one session id, no credentials.

## How

```
npx apparel-monster search "denim shirt" --limit 3
```

Or in code, where the SDK carries the session for you:

```js
import { ApparelMonster } from "apparel-monster";
const store = new ApparelMonster();

const found = await store.search("denim shirt", { limit: 3 });
await store.addToCart(found.products[0].variants[0].id, 1);
const cart = await store.setBilling({
  email: "shopper@example.com", line_one: "1 Market St",
  city: "San Francisco", region_id: "CA", postal_code: "94105",
});
await store.setShipping(cart.shipping_options[0].id);
await store.setPayment("tok_test_123");
await store.checkout();
```

Over MCP at `https://apparel.monster/mcp`, the same flow is `search_products`,
`manage_cart`, `checkout`.

## The two mistakes worth avoiding

**Add a variant, not a product.** A product with five sizes has five variants
and five prices. `variants[].id` is what the cart takes; a product id is
rejected.

**Read `totals.total`.** Do not re-derive it from line items, shipping and tax:
promotions and rounding live in the difference.

## When a human is available

Prefer `wallet` over the cart flow: `store.wallet(["118"])` returns a hosted
Apple Pay / Google Pay link that collects the shopper's email, address and card
on their own device. You never handle any of them, and it is one call rather
than six.

## What not to do

Confirm before placing an order, always. Do not invent a payment token for a
real store — on this one any string works because the gateway is a test
gateway, and that is a property of this store rather than a pattern to carry
elsewhere. Do not promise delivery: apparel.monster never fulfils anything.
