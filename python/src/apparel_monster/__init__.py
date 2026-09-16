"""Apparel Monster SDK and CLI.

    from apparel_monster import ApparelMonster

    store = ApparelMonster()
    hit = store.search("denim shirt", limit=1)["products"][0]
    store.add_to_cart(hit["variants"][0]["id"])

No API key: everything except order history is anonymous.
"""

from ._client import VERSION, ApparelMonster, ApparelMonsterError

__all__ = ["ApparelMonster", "ApparelMonsterError", "VERSION"]
__version__ = VERSION
