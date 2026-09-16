"""apparel-monster — the store from a terminal.

Identifies itself as python-cli rather than python-sdk, so the store can tell a
person poking at the API from an integration running in production. Same
client, one flag apart.

Prints JSON unless ``--text`` is passed: the usual reader here is a script or
an agent, not a person.
"""

from __future__ import annotations

import argparse
import json
import sys
from typing import Any

from ._client import VERSION, ApparelMonster, ApparelMonsterError

EPILOG = """examples:
  apparel-monster search "denim shirt" --limit 3 --text
  apparel-monster ask "coat" --stream
  apparel-monster buy 118

The API needs no credentials. This is a demo store: orders are placed against a
test gateway, nothing ships and no money moves."""


def _print(data: Any, text: bool) -> None:
    if not text:
        print(json.dumps(data, indent=2))
        return
    print(_humanise(data))


def _humanise(data: Any) -> str:
    """Just enough formatting to read a result without piping through jq."""
    if isinstance(data, dict) and isinstance(data.get("products"), list):
        if not data["products"]:
            return "No products matched. Try fewer words, or browse: apparel-monster search --category categories/men"
        blocks = []
        for product in data["products"]:
            prices = [v.get("effective_price") or v.get("price") for v in product.get("variants", [])]
            prices = [p for p in prices if p]
            sizes = sorted({v.get("options", {}).get("Size") for v in product.get("variants", []) if v.get("options")} - {None})
            lines = [f"{product['title']}  ${min(prices):.2f}" if prices else product["title"]]
            if product.get("category"):
                lines.append(f"  {product['category']}")
            if sizes:
                lines.append(f"  sizes: {', '.join(sizes)}")
            lines.append(f"  {product['url']}")
            blocks.append("\n".join(lines))
        return "\n\n".join(blocks)

    if isinstance(data, dict) and isinstance(data.get("results"), list):
        if not data["results"]:
            query = data.get("_meta", {}).get("query", "")
            return (
                f"No match for {query!r}. /ask matches keywords, not meaning — try fewer words "
                '("coat" rather than "warm winter coat").'
            )
        return "\n\n".join(f"{r['name']}\n  {r['description']}\n  {r['url']}" for r in data["results"])

    return json.dumps(data, indent=2)


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        prog="apparel-monster",
        description=f"Agent commerce CLI for apparel.monster (v{VERSION})",
        epilog=EPILOG,
        formatter_class=argparse.RawDescriptionHelpFormatter,
    )
    parser.add_argument("--version", action="version", version=VERSION)

    # Flags that belong to every sub-command, attached to both the top level and
    # each sub-parser. argparse otherwise accepts them only BEFORE the
    # sub-command, which is not where anyone types them:
    #   apparel-monster search "denim shirt" --text   <- the natural order
    common = argparse.ArgumentParser(add_help=False)
    common.add_argument("--base", default="https://apparel.monster", help="point at another origin")
    common.add_argument("--text", action="store_true", help="human-readable output instead of JSON")
    parser.add_argument("--base", default="https://apparel.monster", help=argparse.SUPPRESS)
    parser.add_argument("--text", action="store_true", help=argparse.SUPPRESS)

    sub = parser.add_subparsers(dest="command", required=True)

    search = sub.add_parser("search", help="search the catalog", parents=[common])
    search.add_argument("query", nargs="*")
    search.add_argument("--limit", type=int)
    search.add_argument("--category")
    search.add_argument("--sort", choices=["relevance", "cheapest", "expensive", "newest", "name", "popular"])

    product = sub.add_parser("product", help="full detail for product or variant ids", parents=[common])
    product.add_argument("ids", nargs="+")

    ask = sub.add_parser("ask", help="natural-language search (NLWeb)", parents=[common])
    ask.add_argument("question", nargs="+")
    ask.add_argument("--limit", type=int)
    ask.add_argument("--stream", action="store_true")

    sub.add_parser("pricing", help="plans, limits and merchandise price ranges", parents=[common])

    buy = sub.add_parser("buy", help="mint an Apple Pay / Google Pay link for one variant", parents=[common])
    buy.add_argument("variant_id")

    watch = sub.add_parser("watch", help="watch a variant for a target price", parents=[common])
    watch.add_argument("variant_id")
    watch.add_argument("target_price", type=float)

    status = sub.add_parser("watch-status", help="poll a price watch", parents=[common])
    status.add_argument("watch_id")

    return parser


def main(argv: list[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    store = ApparelMonster(base_url=args.base, cli=True)

    try:
        if args.command == "search":
            _print(
                store.search(" ".join(args.query) or None, limit=args.limit, category=args.category, sort=args.sort),
                args.text,
            )

        elif args.command == "product":
            _print(store.product(args.ids), args.text)

        elif args.command == "ask":
            question = " ".join(args.question)
            if args.stream:
                for event, data in store.ask_stream(question, limit=args.limit):
                    if event == "result":
                        print(f"{data['name']} — {data['url']}" if args.text else json.dumps(data))
            else:
                _print(store.ask(question, limit=args.limit), args.text)

        elif args.command == "pricing":
            # The prose document rather than an endpoint: it is the canonical
            # answer to "what does this cost", and markdown reads fine here.
            import urllib.request

            request = urllib.request.Request(f"{store.base_url}/pricing.md", headers=store._headers("text/markdown"))
            with urllib.request.urlopen(request, timeout=store.timeout) as response:
                print(response.read().decode("utf-8"))

        elif args.command == "buy":
            result = store.wallet([{"variant_id": args.variant_id, "quantity": 1}])
            if args.text:
                print(f"Open this to pay:\n  {result.get('wallet_url')}")
            else:
                _print(result, args.text)

        elif args.command == "watch":
            result = store.watch_price(args.variant_id, args.target_price)
            if args.text:
                if result.get("status") == "already_met":
                    print(f"Already {result['current_price']} — below your {result['target_price']} target.")
                else:
                    print(f"Watching. Poll: {result.get('poll_url')}")
            else:
                _print(result, args.text)

        elif args.command == "watch-status":
            _print(store.price_watch(args.watch_id), args.text)

    except ApparelMonsterError as error:
        # The API's own message already names the missing fields and the next
        # call; repeating them here would only make it longer.
        print(f"{error.code or 'error'}: {error}", file=sys.stderr)
        return 1

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
