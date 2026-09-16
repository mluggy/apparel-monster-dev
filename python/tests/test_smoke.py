"""Smoke tests against the live API.

Live, not mocked. A mocked client tests the mock; these assert the contract
published at /api/v1/openapi.yaml, so a nightly red build means the store moved
— which is what an SDK maintainer wants to hear first.

Nothing here completes an order. The cart flow stops before checkout: this is a
demo store, but a trail of CI orders would make its own reporting useless.
"""

from __future__ import annotations

import json
import unittest

from apparel_monster import VERSION, ApparelMonster, ApparelMonsterError


class SearchTest(unittest.TestCase):
    def setUp(self) -> None:
        self.store = ApparelMonster()

    def test_search_returns_products_and_mints_a_session(self) -> None:
        result = self.store.search("denim shirt", limit=3)

        self.assertTrue(result["session"]["id"], "no session id")
        self.assertTrue(result["products"], "no products for a known-good query")

        product = result["products"][0]
        self.assertTrue(product["title"])
        self.assertTrue(product["url"].startswith("https://apparel.monster/"))
        self.assertTrue(product["variants"])
        self.assertIsInstance(product["variants"][0]["price"], (int, float))

        # Carried from here on without the caller doing anything.
        self.assertEqual(self.store.session_id, result["session"]["id"])

    def test_identifies_itself(self) -> None:
        headers = self.store._headers()
        self.assertTrue(headers["User-Agent"].startswith("apparel-monster-python/"))
        self.assertEqual(headers["X-Agent-Client"], f"python-sdk/{VERSION}")

    def test_cli_identifies_itself_differently(self) -> None:
        headers = ApparelMonster(cli=True)._headers()
        self.assertIn("apparel-monster-python-cli/", headers["User-Agent"])
        self.assertEqual(headers["X-Agent-Client"], f"python-cli/{VERSION}")

    def test_product_accepts_a_variant_id(self) -> None:
        found = self.store.search("denim shirt", limit=1)
        variant_id = found["products"][0]["variants"][0]["id"]

        self.assertTrue(self.store.product(variant_id)["products"])


class CartTest(unittest.TestCase):
    def test_add_then_price_shipping(self) -> None:
        store = ApparelMonster()
        found = store.search("denim shirt", limit=1)
        variants = found["products"][0]["variants"]
        variant_id = next((v["id"] for v in variants if v.get("available")), variants[0]["id"])

        added = store.add_to_cart(variant_id, 1)
        self.assertGreater(added["totals"]["total"], 0)

        billed = store.set_billing(
            email="sdk-ci@example.com",
            first_name="CI",
            last_name="Runner",
            line_one="1 Market St",
            city="San Francisco",
            region_id="CA",
            postal_code="94105",
            country_id="US",
        )
        self.assertTrue(billed["shipping_options"], "no shipping options after an address")

    def test_missing_field_is_reported_as_a_missing_field(self) -> None:
        store = ApparelMonster()
        found = store.search("denim shirt", limit=1)
        store.add_to_cart(found["products"][0]["variants"][0]["id"], 1)

        with self.assertRaises(ApparelMonsterError) as caught:
            store.set_billing(line_one="1 Market St")

        error = caught.exception
        self.assertEqual(error.status, 422)
        self.assertEqual(error.code, "missing_fields")

        fields = [m["field"] for m in error.missing]
        self.assertIn("city", fields)
        self.assertIn("postal_code", fields)

        # The whole point: the message a model reads names them too.
        self.assertIn("city", str(error))
        self.assertIn("postal_code", str(error))


class AskTest(unittest.TestCase):
    def test_ask_returns_schema_org_products(self) -> None:
        answer = ApparelMonster().ask("denim shirt", limit=2)

        self.assertEqual(answer["_meta"]["response_type"], "result_list")
        self.assertEqual(answer["_meta"]["site"], "https://apparel.monster")
        if answer["results"]:
            self.assertEqual(answer["results"][0]["schema_object"]["@type"], "Product")

    def test_ask_stream_starts_and_completes(self) -> None:
        events = [event for event, _ in ApparelMonster().ask_stream("coat", limit=2)]

        self.assertEqual(events[0], "start")
        self.assertEqual(events[-1], "complete")


class BatchTest(unittest.TestCase):
    def test_batch_runs_several_operations(self) -> None:
        result = ApparelMonster().batch(
            [
                {"id": "a", "path": "/api/v1/search", "body": {"query": "denim", "limit": 1}},
                {"id": "b", "path": "/api/v1/search", "body": {"query": "coat", "limit": 1}},
            ]
        )

        self.assertEqual(result["requested"], 2)
        self.assertTrue(all(r["status"] == 200 for r in result["results"]))

    def test_batch_refuses_more_than_the_maximum_before_sending(self) -> None:
        with self.assertRaises(ValueError):
            ApparelMonster().batch([{"path": "/api/v1/search", "body": {}}] * 11)


if __name__ == "__main__":
    unittest.main()
