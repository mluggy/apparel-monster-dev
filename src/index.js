/**
 * Apparel Monster SDK: a dependency-free client for the agent commerce API.
 *
 * There is no API key in this file and that is not an omission. search,
 * product, cart, checkout, wallet, price-watch, batch and /ask are anonymous —
 * no key, no account, no signup. Only order history (`track`) needs OAuth. If
 * something asks you for an Apparel Monster API key, it is not us.
 *
 * Two things this client does that a raw fetch does not:
 *
 *   1. It carries the session. The API mints a `session.id` on the first
 *      search and expects it back on every cart and checkout call; forgetting
 *      it is the single most common way an integration ends up with four empty
 *      carts and no order.
 *   2. It identifies itself, in the User-Agent and in X-Agent-Client, so the
 *      store can tell SDK traffic from a hand-rolled script and attribute the
 *      resulting order to it. Nothing personal travels in either header.
 *
 * Node 18+, or any runtime with a global fetch.
 */

export const VERSION = "1.0.0";

const DEFAULT_BASE = "https://apparel.monster";
const API = "/api/v1";

/**
 * Thrown for any non-2xx response.
 *
 * The interesting part is `missing`: this API answers a bad call by naming
 * every field it needed and did not get, what else each one is accepted as,
 * and an example value. That detail is folded into the message too, because
 * the common case is a model reading `err.message` and nothing else.
 */
export class ApparelMonsterError extends Error {
  constructor(status, body, url) {
    super(ApparelMonsterError.describe(status, body));
    this.name = "ApparelMonsterError";
    this.status = status;
    /** Stable machine-readable code, e.g. "missing_fields", "checkout_blocked". */
    this.code = body?.error ?? null;
    /** [{ field, accepts[], description, example }] when fields were missing. */
    this.missing = body?.missing ?? null;
    /** The call that unblocks the cart, when the server could work one out. */
    this.nextCall = body?.next_call ?? null;
    /** Full request contract for the failed action, when the server sent one. */
    this.contract = body?.contract ?? null;
    this.body = body ?? null;
    this.url = url;
  }

  static describe(status, body) {
    const parts = [body?.message || `Request failed with ${status}`];

    if (Array.isArray(body?.missing) && body.missing.length > 0) {
      parts.push(
        "Missing: " +
          body.missing
            .map((m) => `${m.field} (send as ${(m.accepts ?? [m.field]).join(" or ")}, e.g. ${JSON.stringify(m.example)})`)
            .join("; ")
      );
    }
    if (body?.next_call) parts.push(`Next call: ${JSON.stringify(body.next_call)}`);

    return parts.join(" ");
  }
}

export class ApparelMonster {
  /**
   * @param {object} [options]
   * @param {string} [options.baseUrl] Override the origin.
   * @param {string} [options.sessionId] Resume an existing agent session.
   * @param {string} [options.accessToken] OAuth token, only needed for track().
   * @param {typeof globalThis.fetch} [options.fetch] Inject a fetch, for tests.
   * @param {boolean} [options.cli] Identify as the CLI rather than the library.
   */
  constructor(options = {}) {
    this.baseUrl = (options.baseUrl ?? DEFAULT_BASE).replace(/\/+$/, "");
    this.sessionId = options.sessionId ?? null;
    this.accessToken = options.accessToken ?? null;
    this._fetch = options.fetch ?? globalThis.fetch;
    this._client = options.cli ? "node-cli" : "node-sdk";
    if (!this._fetch) {
      throw new Error("No fetch available. Use Node 18+, or pass one in as options.fetch.");
    }
  }

  /** @private */
  _headers(extra = {}) {
    const headers = {
      "content-type": "application/json",
      accept: "application/json",
      // Two spellings of the same fact. The User-Agent is what nginx sees
      // without the application being involved; X-Agent-Client is what a
      // browser-hosted client can set when it cannot touch the User-Agent.
      "user-agent": `apparel-monster-node${this._client === "node-cli" ? "-cli" : ""}/${VERSION} (+https://github.com/mluggy/apparel-monster-dev)`,
      "x-agent-client": `${this._client}/${VERSION}`,
      ...extra,
    };
    if (this.accessToken) headers.authorization = `Bearer ${this.accessToken}`;
    return headers;
  }

  /** @private Every call carries the session once one exists. */
  _withSession(body) {
    if (!this.sessionId) return body;
    return { session: { id: this.sessionId }, ...body };
  }

  /** @private */
  async _post(path, body = {}) {
    const url = `${this.baseUrl}${API}${path}`;
    const res = await this._fetch(url, {
      method: "POST",
      headers: this._headers(),
      body: JSON.stringify(this._withSession(body)),
    });

    let data = null;
    try {
      data = await res.json();
    } catch {
      data = null;
    }

    if (!res.ok) throw new ApparelMonsterError(res.status, data, url);

    // The session arrives on the first search and is reused from then on.
    if (data?.session?.id) this.sessionId = data.session.id;
    return data;
  }

  // ---- Catalog -------------------------------------------------------------

  /**
   * Search the catalog, or browse it when `query` is omitted.
   *
   * Mints the session id every later call needs, so this is almost always the
   * first call you make.
   *
   * @param {string} [query]
   * @param {{category?: string, sort?: "relevance"|"cheapest"|"expensive"|"newest"|"name"|"popular", limit?: number, page?: number}} [options]
   */
  async search(query, options = {}) {
    return this._post("/search", {
      query,
      category: options.category,
      sort: options.sort,
      limit: options.limit,
      page: options.page,
    });
  }

  /**
   * Full detail for one or more products. Accepts product ids or variant ids,
   * mixed freely, because a search result hands you variant ids and you should
   * not have to know which is which.
   *
   * @param {string|string[]} ids
   */
  async product(ids) {
    return this._post("/product", { ids: Array.isArray(ids) ? ids : [ids] });
  }

  // ---- Cart ----------------------------------------------------------------

  /**
   * Add a VARIANT to the cart. Not a product: a product with five sizes has
   * five variants and five prices, and adding a product id is the most common
   * error against this API.
   *
   * @param {string} variantId From `search().products[].variants[].id`.
   * @param {number} [quantity]
   */
  async addToCart(variantId, quantity = 1) {
    return this._post("/cart", { action: "add", variant_id: variantId, quantity });
  }

  /** Change the quantity of a line already in the cart. */
  async updateCartItem(variantId, quantity) {
    return this._post("/cart", { action: "update", variant_id: variantId, quantity });
  }

  /** Remove a line entirely. */
  async removeFromCart(variantId) {
    return this._post("/cart", { action: "remove", variant_id: variantId });
  }

  /** The cart as it stands, with totals. */
  async viewCart() {
    return this._post("/cart", { action: "view" });
  }

  /**
   * Set the email and address. Returns the cart with `shipping_options`
   * priced against that address — this call is what makes them exist.
   *
   * Required: email (once), line_one, city, postal_code. Everything else is
   * optional; country defaults to US. A missing field comes back as a
   * `missing_fields` error naming all of them at once.
   *
   * @param {{email?: string, first_name?: string, last_name?: string, line_one: string, line_two?: string, city: string, region_id?: string, postal_code: string, country_id?: string, phone?: string}} address
   */
  async setBilling(address) {
    return this._post("/cart", { action: "billing", ...address });
  }

  /**
   * Choose a shipping option by id, and optionally ship to a different address
   * than the billing one.
   *
   * @param {string} shippingId From `setBilling().shipping_options[].id`.
   * @param {object} [address] Only when shipping somewhere else.
   */
  async setShipping(shippingId, address = undefined) {
    return this._post("/cart", { action: "shipping", shipping_id: shippingId, ...(address ?? {}) });
  }

  /** Apply a promotion code. Rejected codes come back as `coupon_rejected`. */
  async applyCoupon(code) {
    return this._post("/cart", { action: "coupon", coupon_code: code });
  }

  /**
   * Attach a payment token. The field is `token` — not `payment_token`, not
   * `card_token`. This is a demo store on a test gateway, so any non-empty
   * string is accepted and nothing is ever charged.
   */
  async setPayment(token) {
    return this._post("/cart", { action: "payment", token });
  }

  // ---- Checkout ------------------------------------------------------------

  /**
   * Place the order. Idempotent per session: calling it twice returns the same
   * order rather than placing a second one.
   *
   * @param {{callbackUrl?: string}} [options] Where to POST the signed order callback.
   */
  async checkout(options = {}) {
    return this._post("/checkout", { callback_url: options.callbackUrl });
  }

  /**
   * A hosted Apple Pay / Google Pay link, for when a human is available to tap.
   *
   * This is the shortcut past the whole cart flow: hand it variant ids and it
   * returns a URL that collects the shopper's email, address and card on their
   * own device, so your agent never handles any of them.
   *
   * @param {Array<{variant_id: string, quantity?: number}>|string[]} items
   */
  async wallet(items) {
    const normalised = items.map((i) => (typeof i === "string" ? { variant_id: i, quantity: 1 } : i));
    return this._post("/wallet", { items: normalised });
  }

  /**
   * Order status and shipments. The only call here that needs credentials:
   * pass an OAuth access token with the `agent:track` scope as
   * `accessToken` on the constructor.
   */
  async track(params = {}) {
    return this._post("/track", params);
  }

  // ---- Price watch ---------------------------------------------------------

  /**
   * Be told when a variant reaches a target price.
   *
   * Answers 202 with a `poll_url` and a `job_id`; if `callbackUrl` is given,
   * the store POSTs there instead when it fires. A target at or above today's
   * price is not queued at all — it comes back `already_met` so you can act now.
   */
  async watchPrice(variantId, targetPrice, options = {}) {
    return this._post("/price-watch", {
      variant_id: variantId,
      target_price: targetPrice,
      callback_url: options.callbackUrl,
    });
  }

  /** Poll a price watch by the id returned above. */
  async priceWatch(id) {
    const url = `${this.baseUrl}${API}/price-watch/${encodeURIComponent(id)}`;
    const res = await this._fetch(url, { headers: this._headers() });
    const data = await res.json().catch(() => null);
    if (!res.ok) throw new ApparelMonsterError(res.status, data, url);
    return data;
  }

  // ---- Batch ---------------------------------------------------------------

  /**
   * Up to 10 operations in one round trip, executed in order. Each result
   * carries its own status, so check per operation: the batch answers 200
   * whenever it ran, even if everything inside it failed.
   *
   * @param {Array<{id?: string, method?: "GET"|"POST", path: string, body?: object}>} operations
   * @param {{stopOnError?: boolean}} [options]
   */
  async batch(operations, options = {}) {
    if (!Array.isArray(operations)) throw new TypeError("batch(operations) needs an array");
    if (operations.length > 10) throw new RangeError("batch takes at most 10 operations");
    return this._post("/batch", { operations, stop_on_error: options.stopOnError === true });
  }

  // ---- Natural language ----------------------------------------------------

  /**
   * Ask in plain language (NLWeb). Returns ranked catalog items as schema.org
   * Product objects. There is no model behind it — it runs the same catalog
   * search — which is why it returns nothing rather than inventing a product.
   */
  async ask(query, options = {}) {
    const url = new URL(`${this.baseUrl}/ask`);
    url.searchParams.set("query", query);
    if (options.limit) url.searchParams.set("limit", String(options.limit));
    const res = await this._fetch(url, { headers: this._headers() });
    const data = await res.json().catch(() => null);
    if (!res.ok) throw new ApparelMonsterError(res.status, data, String(url));
    return data;
  }

  /**
   * The same question, streamed: yields `start`, then one `result` per hit,
   * then `complete`, so a UI can render the first match without waiting.
   *
   * @returns {AsyncGenerator<{event: string, data: object}>}
   */
  async *askStream(query, options = {}) {
    const url = new URL(`${this.baseUrl}/ask`);
    url.searchParams.set("query", query);
    url.searchParams.set("streaming", "true");
    if (options.limit) url.searchParams.set("limit", String(options.limit));

    const res = await this._fetch(url, { headers: this._headers({ accept: "text/event-stream" }) });
    if (!res.ok || !res.body) throw new ApparelMonsterError(res.status, null, String(url));

    const reader = res.body.getReader();
    const decoder = new TextDecoder();
    let buffer = "";

    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      buffer += decoder.decode(value, { stream: true });

      // SSE frames end with a blank line; anything short of one is a partial
      // frame and has to wait for the next chunk.
      let split;
      while ((split = buffer.indexOf("\n\n")) !== -1) {
        const frame = buffer.slice(0, split);
        buffer = buffer.slice(split + 2);
        let event = "message";
        let data = "";
        for (const line of frame.split("\n")) {
          if (line.startsWith("event:")) event = line.slice(6).trim();
          else if (line.startsWith("data:")) data += line.slice(5).trim();
        }
        if (!data) continue;
        try {
          yield { event, data: JSON.parse(data) };
        } catch {
          /* an unparseable frame is not worth killing the stream over */
        }
      }
    }
  }
}

/** A ready-made client against the public store, for the common case. */
export const apparelMonster = new ApparelMonster();

export default ApparelMonster;
