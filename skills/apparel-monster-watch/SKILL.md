---
name: apparel-monster-watch
description: "Watch an apparel.monster product for a price drop and be notified — by callback or by polling. Use when a shopper likes something but wants it cheaper."
---

# Watching a price on Apparel Monster

## How

```
npx apparel-monster watch 118 49.99
npx apparel-monster watch-status pw_5c15upqhdjashijV5qHRo5ig
```

```js
const watch = await store.watchPrice("118", 49.99, {
  callbackUrl: "https://example.com/hooks/price",   // optional
});
watch.job_id;    // "pw_..."
watch.poll_url;  // poll this if you gave no callback
```

REST: `POST /api/v1/price-watch`. Over MCP: `watch_price`.

## What the response means

- **202 Accepted** — registered. `Location` and `poll_url` point at the status
  resource; `poll_after_seconds` says how long to wait before the first poll.
- **`status: already_met`** — the target is at or above today's price, so
  nothing was queued and `watch_id` is null. Tell the shopper to buy now rather
  than waiting for a drop that has already happened.
- With a `callback_url`, the store POSTs there when it fires, signed — verify
  it against `https://apparel.monster/.well-known/jwks.json`.

## What not to do

Do not poll faster than `poll_after_seconds`; it only burns the rate limit.
And be honest about the mechanism on this store: prices here never actually
move, so a watch fires on a fabricated drop 1–10 minutes after registration, to
make the callback path demonstrable. Do not describe that as a real sale.
