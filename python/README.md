# apparel-monster (Python)

SDK and CLI for the [Apparel Monster](https://apparel.monster) agent commerce
API. Standard library only — no requests, no dependencies at all.

```
pip install apparel-monster
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

No API key: every call except order history is anonymous. Prices live on
variants, never on the product.

```
apparel-monster search "denim shirt" --limit 3 --text
apparel-monster ask "coat" --stream
apparel-monster buy 118 --text
```

Errors name what they needed:

```python
from apparel_monster import ApparelMonsterError

try:
    store.set_billing(line_one="1 Market St")
except ApparelMonsterError as error:
    error.code      # "missing_fields"
    error.missing   # [{"field": "city", ...}, {"field": "postal_code", "accepts": [...]}]
```

apparel.monster is a demo storefront: orders are placed against a test gateway,
nothing ships and no money moves.

Full documentation: <https://apparel.monster/developers> ·
Source: <https://github.com/mluggy/apparel-monster-dev>
