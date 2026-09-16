"""Apparel Monster SDK: a dependency-free client for the agent commerce API.

No API key appears in this file and that is not an omission. search, product,
cart, checkout, wallet, price-watch, batch and /ask are anonymous. Only order
history (``track``) needs OAuth. If something asks you for an Apparel Monster
API key, it is not us.

Standard library only — urllib, not requests — so installing this pulls in
nothing and it works inside an agent sandbox with no package index.
"""

from __future__ import annotations

import json
import urllib.error
import urllib.parse
import urllib.request
from typing import Any, Iterator

VERSION = "1.0.2"

DEFAULT_BASE = "https://apparel.monster"
API = "/api/v1"


class ApparelMonsterError(Exception):
    """Raised for any non-2xx response.

    The interesting attribute is :attr:`missing`: the API answers a bad call by
    naming every field it needed and did not get, what else each one is
    accepted as, and an example value. That detail is folded into ``str(error)``
    as well, because the common case is a model reading the message and nothing
    else.
    """

    def __init__(self, status: int, body: dict[str, Any] | None, url: str) -> None:
        super().__init__(self._describe(status, body))
        self.status = status
        self.code = (body or {}).get("error")
        self.missing = (body or {}).get("missing")
        self.next_call = (body or {}).get("next_call")
        self.contract = (body or {}).get("contract")
        self.body = body
        self.url = url

    @staticmethod
    def _describe(status: int, body: dict[str, Any] | None) -> str:
        body = body or {}
        parts = [body.get("message") or f"Request failed with {status}"]

        missing = body.get("missing")
        if isinstance(missing, list) and missing:
            described = "; ".join(
                f"{m.get('field')} (send as {' or '.join(m.get('accepts') or [m.get('field')])}, "
                f"e.g. {json.dumps(m.get('example'))})"
                for m in missing
            )
            parts.append(f"Missing: {described}")

        if body.get("next_call"):
            parts.append(f"Next call: {json.dumps(body['next_call'])}")

        return " ".join(parts)


class ApparelMonster:
    """A client for one shopping session.

    Two things this does that a raw request does not: it carries the session id
    the API mints on the first search (forgetting it is the usual way an
    integration ends up with four empty carts and no order), and it identifies
    itself so the store can attribute the order to this SDK.
    """

    def __init__(
        self,
        base_url: str = DEFAULT_BASE,
        session_id: str | None = None,
        access_token: str | None = None,
        timeout: float = 30.0,
        cli: bool = False,
    ) -> None:
        self.base_url = base_url.rstrip("/")
        self.session_id = session_id
        self.access_token = access_token
        self.timeout = timeout
        self._client = "python-cli" if cli else "python-sdk"

    # ---- plumbing --------------------------------------------------------

    def _headers(self, accept: str = "application/json") -> dict[str, str]:
        suffix = "-cli" if self._client == "python-cli" else ""
        headers = {
            "Content-Type": "application/json",
            "Accept": accept,
            # Two spellings of one fact: the User-Agent is what the edge sees
            # without the application being involved, X-Agent-Client is what
            # survives an environment that rewrites User-Agent.
            "User-Agent": (
                f"apparel-monster-python{suffix}/{VERSION} "
                "(+https://github.com/mluggy/apparel-monster-dev)"
            ),
            "X-Agent-Client": f"{self._client}/{VERSION}",
        }
        if self.access_token:
            headers["Authorization"] = f"Bearer {self.access_token}"
        return headers

    def _with_session(self, body: dict[str, Any]) -> dict[str, Any]:
        if not self.session_id:
            return body
        return {"session": {"id": self.session_id}, **body}

    def _request(self, url: str, data: bytes | None = None, accept: str = "application/json") -> Any:
        request = urllib.request.Request(url, data=data, headers=self._headers(accept), method="POST" if data else "GET")
        try:
            with urllib.request.urlopen(request, timeout=self.timeout) as response:
                payload = json.loads(response.read().decode("utf-8"))
        except urllib.error.HTTPError as error:
            raw = error.read().decode("utf-8", "replace")
            try:
                body = json.loads(raw)
            except ValueError:
                body = {"message": raw[:500]}
            raise ApparelMonsterError(error.code, body, url) from None

        if isinstance(payload, dict):
            session = payload.get("session")
            if isinstance(session, dict) and session.get("id"):
                self.session_id = session["id"]
        return payload

    def _post(self, path: str, body: dict[str, Any] | None = None) -> Any:
        payload = {k: v for k, v in self._with_session(body or {}).items() if v is not None}
        return self._request(f"{self.base_url}{API}{path}", json.dumps(payload).encode("utf-8"))

    # ---- catalog ---------------------------------------------------------

    def search(
        self,
        query: str | None = None,
        *,
        category: str | None = None,
        sort: str | None = None,
        limit: int | None = None,
        page: int | None = None,
    ) -> dict[str, Any]:
        """Search the catalog, or browse it when ``query`` is omitted.

        Mints the session id every later call needs, so this is almost always
        the first call you make.
        """
        return self._post("/search", {"query": query, "category": category, "sort": sort, "limit": limit, "page": page})

    def product(self, ids: str | list[str]) -> dict[str, Any]:
        """Full detail for product ids or variant ids, mixed freely."""
        return self._post("/product", {"ids": [ids] if isinstance(ids, str) else list(ids)})

    # ---- cart ------------------------------------------------------------

    def add_to_cart(self, variant_id: str, quantity: int = 1) -> dict[str, Any]:
        """Add a VARIANT — not a product.

        A product with five sizes has five variants and five prices; adding a
        product id is the most common mistake made against this API.
        """
        return self._post("/cart", {"action": "add", "variant_id": variant_id, "quantity": quantity})

    def update_cart_item(self, variant_id: str, quantity: int) -> dict[str, Any]:
        return self._post("/cart", {"action": "update", "variant_id": variant_id, "quantity": quantity})

    def remove_from_cart(self, variant_id: str) -> dict[str, Any]:
        return self._post("/cart", {"action": "remove", "variant_id": variant_id})

    def view_cart(self) -> dict[str, Any]:
        return self._post("/cart", {"action": "view"})

    def set_billing(self, **address: Any) -> dict[str, Any]:
        """Set the email and address; returns the cart with ``shipping_options``.

        Required: ``email`` (once), ``line_one``, ``city``, ``postal_code``.
        A missing one raises :class:`ApparelMonsterError` naming all of them at
        once, with the aliases each is accepted under.
        """
        return self._post("/cart", {"action": "billing", **address})

    def set_shipping(self, shipping_id: str, **address: Any) -> dict[str, Any]:
        return self._post("/cart", {"action": "shipping", "shipping_id": shipping_id, **address})

    def apply_coupon(self, code: str) -> dict[str, Any]:
        return self._post("/cart", {"action": "coupon", "coupon_code": code})

    def set_payment(self, token: str) -> dict[str, Any]:
        """Attach a payment token.

        The field is ``token`` — not ``payment_token``, not ``card_token``.
        This is a demo store on a test gateway: any non-empty string is
        accepted and nothing is ever charged.
        """
        return self._post("/cart", {"action": "payment", "token": token})

    # ---- checkout --------------------------------------------------------

    def checkout(self, *, callback_url: str | None = None) -> dict[str, Any]:
        """Place the order. Idempotent per session."""
        return self._post("/checkout", {"callback_url": callback_url})

    def wallet(self, items: list[Any]) -> dict[str, Any]:
        """A hosted Apple Pay / Google Pay link, for when a human can tap.

        The shortcut past the whole cart flow: hand it variant ids and it
        returns a URL that collects the shopper's email, address and card on
        their own device, so your agent never handles any of them.
        """
        normalised = [{"variant_id": i, "quantity": 1} if isinstance(i, str) else i for i in items]
        return self._post("/wallet", {"items": normalised})

    def track(self, **params: Any) -> dict[str, Any]:
        """Order status and shipments. The only call needing OAuth."""
        return self._post("/track", params)

    # ---- price watch -----------------------------------------------------

    def watch_price(self, variant_id: str, target_price: float, *, callback_url: str | None = None) -> dict[str, Any]:
        """Be told when a variant reaches a target price.

        Answers 202 with ``poll_url`` and ``job_id``. A target at or above
        today's price is not queued at all: it comes back ``already_met``.
        """
        return self._post(
            "/price-watch",
            {"variant_id": variant_id, "target_price": target_price, "callback_url": callback_url},
        )

    def price_watch(self, watch_id: str) -> dict[str, Any]:
        return self._request(f"{self.base_url}{API}/price-watch/{urllib.parse.quote(watch_id)}")

    # ---- batch -----------------------------------------------------------

    def batch(self, operations: list[dict[str, Any]], *, stop_on_error: bool = False) -> dict[str, Any]:
        """Up to 10 operations in one round trip, executed in order.

        Each result carries its own status: the batch answers 200 whenever it
        ran, even if every operation inside it failed.
        """
        if len(operations) > 10:
            raise ValueError("batch takes at most 10 operations")
        return self._post("/batch", {"operations": operations, "stop_on_error": stop_on_error})

    # ---- natural language ------------------------------------------------

    def ask(self, query: str, *, limit: int | None = None) -> dict[str, Any]:
        """Ask in plain language (NLWeb); returns schema.org ``Product`` items.

        There is no model behind it — it runs the same catalog search — which
        is why it returns nothing rather than inventing a product.
        """
        params = {"query": query}
        if limit:
            params["limit"] = str(limit)
        return self._request(f"{self.base_url}/ask?{urllib.parse.urlencode(params)}")

    def ask_stream(self, query: str, *, limit: int | None = None) -> Iterator[tuple[str, dict[str, Any]]]:
        """The same question, streamed: ``start``, one ``result`` per hit, ``complete``."""
        params = {"query": query, "streaming": "true"}
        if limit:
            params["limit"] = str(limit)
        url = f"{self.base_url}/ask?{urllib.parse.urlencode(params)}"
        request = urllib.request.Request(url, headers=self._headers("text/event-stream"))

        with urllib.request.urlopen(request, timeout=self.timeout) as response:
            event = "message"
            data = ""
            for raw in response:
                line = raw.decode("utf-8").rstrip("\n")
                if line.startswith("event:"):
                    event = line[6:].strip()
                elif line.startswith("data:"):
                    data += line[5:].strip()
                elif line == "":
                    # A blank line terminates an SSE frame; anything before one
                    # is a partial frame and must not be yielded.
                    if data:
                        try:
                            yield event, json.loads(data)
                        except ValueError:
                            pass
                    event, data = "message", ""
