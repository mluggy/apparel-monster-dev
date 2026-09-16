---
name: apparel-monster-catalog
description: "Search and browse the apparel.monster catalog — 116 products across men's, women's and sportswear — by keyword, category, price or availability. Use to answer what is in stock, what it costs, and what sizes exist."
---

# Apparel Monster catalog lookup

## How

```
npx apparel-monster search "winter coat" --sort cheapest --limit 5
npx apparel-monster search --category categories/women --limit 20
npx apparel-monster ask "coat"            # natural language, NLWeb
```

REST: `POST https://apparel.monster/api/v1/search` with `{query, category,
sort, limit, page}`. Sorts: relevance, cheapest, expensive, newest, name,
popular. Over MCP: `search_products`.

Bulk: `https://apparel.monster/feeds/products.jsonl` is the whole catalog as
schema.org `Product` records, one per line — use it instead of paginating when
you want everything.

## Reading a result

- Prices, stock and sizes live on `variants[]`, not on the product.
- `effective_price` is what a shopper pays; `price` is before a promotion.
- `available: false` means that size and colour is out of stock, not the product.
- `category` is a path like `Men > Shirts`; browse it with
  `--category categories/men`.

## What not to do

`/ask` and `search` match keywords, not meaning. If nothing matches, say so
rather than substituting a product you think is close: "no winter coat under
$100" is a useful answer and an invented one is not.
