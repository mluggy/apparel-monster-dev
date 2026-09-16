# apparel-monster

SDK and CLI for the [Apparel Monster](https://apparel.monster) agent commerce
API, in four languages. Search a catalog of 116 products, build a cart, place an
order, mint an Apple Pay / Google Pay link, watch a price, or ask in plain
language.

**No authentication.** Search, product, cart, checkout, wallet, price-watch,
batch and `/ask` need no key, no account and no signup. Only order history
(`track`) uses OAuth. If something asks you for an Apparel Monster API key, it
is not us.

**No dependencies.** Every client is standard library only — `fetch`, `urllib`,
`net/http`, `net/http`. Installing one pulls in nothing.

> apparel.monster is a **demonstration storefront**. The catalog, prices, stock
> and checkout flow are real Spree objects, but orders are placed against a test
> gateway, nothing is fulfilled and no money moves. It exists to be a worked
> example of an agent-ready commerce surface — including this repository.

## Install

| | |
|---|---|
| npm | `npm install apparel-monster` |
| PyPI | `pip install apparel-monster` |
| RubyGems | `gem install apparel-monster` |
| Go | `go get github.com/mluggy/apparel-monster-dev/go` |

The CLI ships inside each package, so `npx apparel-monster search "denim shirt"`
works without installing anything at all.

## Sixty seconds

```js
import { ApparelMonster } from "apparel-monster";

const store = new ApparelMonster();

// 1. Search. This mints the session id every later call needs — the client
//    carries it for you.
const found = await store.search("denim shirt", { limit: 3 });

// 2. Add a VARIANT, never a product. Prices live on variants: one product with
//    five sizes has five variants and five prices.
const variant = found.products[0].variants.find((v) => v.available);
await store.addToCart(variant.id, 1);

// 3. An address makes shipping options exist.
const cart = await store.setBilling({
  email: "shopper@example.com",
  line_one: "1 Market St",
  city: "San Francisco",
  region_id: "CA",
  postal_code: "94105",
});

// 4. Pick one, pay, place it.
await store.setShipping(cart.shipping_options[0].id);
await store.setPayment("tok_test_123");
const order = await store.checkout();
```

```python
from apparel_monster import ApparelMonster

store = ApparelMonster()
found = store.search("denim shirt", limit=3)
store.add_to_cart(found["products"][0]["variants"][0]["id"])
cart = store.set_billing(
    email="shopper@example.com",
    line_one="1 Market St",
    city="San Francisco",
    region_id="CA",
    postal_code="94105",
)
store.set_shipping(cart["shipping_options"][0]["id"])
store.set_payment("tok_test_123")
order = store.checkout()
```

```ruby
require "apparel_monster"

store = ApparelMonster::Client.new
found = store.search("denim shirt", limit: 3)
store.add_to_cart(found["products"].first["variants"].first["id"])
cart = store.set_billing(
  "email" => "shopper@example.com",
  "line_one" => "1 Market St",
  "city" => "San Francisco",
  "region_id" => "CA",
  "postal_code" => "94105"
)
store.set_shipping(cart["shipping_options"].first["id"])
store.set_payment("tok_test_123")
order = store.checkout
```

```go
store := apparelmonster.New()
found, _ := store.Search(ctx, "denim shirt", apparelmonster.SearchOptions{Limit: 3})
store.AddToCart(ctx, found.Products[0].Variants[0].ID, 1)
cart, _ := store.SetBilling(ctx, apparelmonster.Address{
    Email: "shopper@example.com", LineOne: "1 Market St",
    City: "San Francisco", RegionID: "CA", PostalCode: "94105",
})
store.SetShipping(ctx, cart.ShippingOptions[0].ID)
store.SetPayment(ctx, "tok_test_123")
order, _ := store.Checkout(ctx, "")
```

## The shortcut past all of that

When a human is available to tap, skip the cart entirely:

```js
const { wallet_url } = await store.wallet(["118"]);
// Hand wallet_url to the shopper. The wallet sheet collects their email,
// address and card on their own device, so your agent never handles any of them.
```

## CLI

```
apparel-monster search "denim shirt" --limit 3 --text
apparel-monster ask "coat" --stream
apparel-monster product 118
apparel-monster buy 118 --text
apparel-monster watch 118 49.99
apparel-monster pricing
```

Output is JSON unless `--text` is passed, because the usual reader is a script
or an agent rather than a person.

## Errors tell you what to send

A bad call does not answer "invalid request". It names every field it needed,
every alias that field is accepted under, and an example:

```js
try {
  await store.setBilling({ line_one: "1 Market St" });
} catch (error) {
  error.code;      // "missing_fields"
  error.missing;   // [{ field: "city", accepts: ["city"], example: "San Francisco" },
                   //  { field: "postal_code", accepts: ["postal_code","zipcode","zip"], example: "94105" }]
  error.message;   // "...is missing 2 required fields: city, postal_code. Missing: city (send as city,
                   //  e.g. "San Francisco"); postal_code (send as postal_code or zipcode or zip, e.g. "94105")"
}
```

When a cart is in the wrong state rather than missing a field, the error carries
`next_call` — the exact call that unblocks it.

## Everything else on the surface

| Method | What it does |
|---|---|
| `search` / `product` | Catalog. Product ids and variant ids are both accepted |
| `addToCart` / `updateCartItem` / `removeFromCart` / `viewCart` | Cart lines |
| `setBilling` / `setShipping` / `applyCoupon` / `setPayment` | Checkout steps |
| `checkout` | Place the order. Idempotent per session |
| `wallet` | Hosted Apple Pay / Google Pay link |
| `watchPrice` / `priceWatch` | Be told when a price drops. 202 + poll URL, or a callback |
| `batch` | Up to 10 operations in one round trip, in order |
| `ask` / `askStream` | Natural language (NLWeb), JSON or SSE |
| `track` | Order history. The only call that needs OAuth |

## Other ways in

This repository is one of several front doors onto the same store, and none of
them is a reimplementation of another:

- **MCP, commerce**: `https://apparel.monster/mcp` — the tools an agent acts with
- **MCP, documentation**: `https://apparel.monster/mcp/docs` — read-only, answers questions
- **WebMCP**: injected into every storefront page, for in-browser agents
- **ACP / UCP**: checkout-session protocols at `/checkout_sessions` and `/ucp/v1`
- **OpenAPI**: <https://apparel.monster/api/v1/openapi.yaml>
- **Sandbox notes**: <https://apparel.monster/sandbox>

## Identification

Each client sends `User-Agent: apparel-monster-<language>[-cli]/<version>` and
`X-Agent-Client: <language>-(sdk|cli)/<version>`, so the store can distinguish
SDK traffic from a hand-rolled script, and tell a person exploring on the CLI
from an integration running in production. Orders carry the client that placed
them as a tag. Nothing personal travels in either header.

## Development

```
npm test                                   # Node
cd python && PYTHONPATH=src python -m unittest discover -s tests
cd ruby   && ruby -Ilib test/apparel_monster_test.rb
cd go     && go test ./...                 # -short skips the live calls
```

The tests run against the **live** API rather than fixtures. A mocked client
tests the mock; these assert the published contract, so CI going red means the
store moved — which is the thing an SDK maintainer wants to hear first. They
stop short of completing an order.

## Licence

MIT.
