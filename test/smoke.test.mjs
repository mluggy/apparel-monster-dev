/**
 * Smoke tests against the live API.
 *
 * Live, not mocked, and that is the point: a mocked client tests the mock. The
 * contract these assert is the one published in /api/v1/openapi.yaml, so when
 * CI goes red on the nightly run it means the store moved, which is exactly
 * what an SDK maintainer wants to hear first.
 *
 * Nothing here completes an order. The cart flow stops before checkout: this
 * is a demo store, but leaving a trail of orders behind every CI run would
 * make the store's own reporting useless.
 */

import { test } from "node:test";
import assert from "node:assert/strict";
import { ApparelMonster, ApparelMonsterError, VERSION } from "../src/index.js";

const store = new ApparelMonster();

test("search returns products with variants, and mints a session", async () => {
  const result = await store.search("denim shirt", { limit: 3 });

  assert.ok(result.session?.id, "no session id");
  assert.ok(Array.isArray(result.products), "products is not an array");
  assert.ok(result.products.length > 0, "no products for a known-good query");

  const product = result.products[0];
  assert.ok(product.title, "product has no title");
  assert.ok(product.url.startsWith("https://apparel.monster/"), "product url is not absolute");
  assert.ok(product.variants.length > 0, "product has no variants");
  assert.equal(typeof product.variants[0].price, "number", "variant price is not a number");

  // The session is carried from here on without the caller doing anything.
  assert.equal(store.sessionId, result.session.id);
});

test("identifies itself as node-sdk", async () => {
  let seen = null;
  const spy = new ApparelMonster({
    fetch: async (url, init) => {
      seen = init.headers;
      return new Response(JSON.stringify({ products: [] }), {
        status: 200,
        headers: { "content-type": "application/json" },
      });
    },
  });
  await spy.search("anything");

  assert.match(seen["user-agent"], /^apparel-monster-node\/\d+\.\d+\.\d+/);
  assert.equal(seen["x-agent-client"], `node-sdk/${VERSION}`);
});

test("the CLI identifies itself differently from the library", async () => {
  let seen = null;
  const cli = new ApparelMonster({
    cli: true,
    fetch: async (url, init) => {
      seen = init.headers;
      return new Response("{}", { status: 200, headers: { "content-type": "application/json" } });
    },
  });
  await cli.search("anything");

  assert.match(seen["user-agent"], /apparel-monster-node-cli\//);
  assert.equal(seen["x-agent-client"], `node-cli/${VERSION}`);
});

test("product() accepts variant ids as well as product ids", async () => {
  const found = await store.search("denim shirt", { limit: 1 });
  const variantId = found.products[0].variants[0].id;

  const byVariant = await store.product(variantId);
  assert.ok(byVariant.products.length > 0, "no product for a variant id");
});

test("cart: add, view, and a priced shipping option", async () => {
  const shop = new ApparelMonster();
  const found = await shop.search("denim shirt", { limit: 1 });
  const variantId = found.products[0].variants.find((v) => v.available)?.id ?? found.products[0].variants[0].id;

  const added = await shop.addToCart(variantId, 1);
  assert.ok(added.totals.total > 0, "cart total did not move after adding a line");

  const billed = await shop.setBilling({
    email: "sdk-ci@example.com",
    first_name: "CI",
    last_name: "Runner",
    line_one: "1 Market St",
    city: "San Francisco",
    region_id: "CA",
    postal_code: "94105",
    country_id: "US",
  });
  assert.ok(billed.shipping_options?.length > 0, "no shipping options after setting an address");
  assert.equal(typeof billed.shipping_options[0].price, "number");
});

test("a missing field is reported as a missing field, not a 500", async () => {
  const shop = new ApparelMonster();
  const found = await shop.search("denim shirt", { limit: 1 });
  await shop.addToCart(found.products[0].variants[0].id, 1);

  await assert.rejects(
    () => shop.setBilling({ line_one: "1 Market St" }),
    (error) => {
      assert.ok(error instanceof ApparelMonsterError);
      assert.equal(error.status, 422);
      assert.equal(error.code, "missing_fields");
      assert.ok(Array.isArray(error.missing), "error carried no field list");

      const fields = error.missing.map((m) => m.field);
      assert.ok(fields.includes("city"), `expected city in ${fields}`);
      assert.ok(fields.includes("postal_code"), `expected postal_code in ${fields}`);

      // The whole point: the message a model reads names them too.
      assert.match(error.message, /city/);
      assert.match(error.message, /postal_code/);
      return true;
    }
  );
});

test("ask returns schema.org products", async () => {
  const answer = await store.ask("denim shirt", { limit: 2 });

  assert.equal(answer._meta.response_type, "result_list");
  assert.equal(answer._meta.site, "https://apparel.monster");
  if (answer.results.length > 0) {
    const first = answer.results[0];
    assert.equal(first.schema_object["@type"], "Product");
    assert.ok(first.url.startsWith("https://apparel.monster/"));
  }
});

test("askStream yields start, results, then complete", async () => {
  const events = [];
  for await (const { event } of store.askStream("coat", { limit: 2 })) events.push(event);

  assert.equal(events[0], "start");
  assert.equal(events.at(-1), "complete");
});

test("batch runs several operations in one round trip", async () => {
  const result = await store.batch([
    { id: "a", path: "/api/v1/search", body: { query: "denim", limit: 1 } },
    { id: "b", path: "/api/v1/search", body: { query: "coat", limit: 1 } },
  ]);

  assert.equal(result.requested, 2);
  assert.equal(result.results.length, 2);
  assert.ok(result.results.every((r) => r.status === 200));
});

test("batch refuses more than the documented maximum before sending", async () => {
  const tooMany = Array.from({ length: 11 }, () => ({ path: "/api/v1/search", body: {} }));
  await assert.rejects(() => store.batch(tooMany), RangeError);
});
