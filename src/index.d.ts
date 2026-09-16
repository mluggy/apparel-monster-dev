/**
 * Type definitions for the Apparel Monster SDK.
 *
 * The response shapes are described loosely on purpose. This is a live
 * storefront API that adds fields inside v1 without notice, and a type that
 * forbids unknown keys would make every additive change a breaking one for
 * TypeScript users. The shapes you act on are pinned; the rest is open.
 */

export declare const VERSION: string;

export interface Money {
  id: string;
}

export interface Variant {
  id: string;
  sku: string;
  title: string;
  price: number;
  effective_price?: number;
  currency: Money;
  available: boolean;
  options?: Record<string, string>;
  image_url?: string;
  url?: string;
  [key: string]: unknown;
}

export interface Product {
  id: string;
  title: string;
  url: string;
  category?: string;
  images?: Array<{ url: string }>;
  variants: Variant[];
  [key: string]: unknown;
}

export interface Session {
  id: string;
}

export interface Totals {
  subtotal: number;
  discount: number;
  tax: number;
  shipping: number;
  total: number;
}

export interface ShippingOption {
  id: string;
  name?: string;
  price?: number;
  delivery?: string;
  is_selected?: boolean;
  [key: string]: unknown;
}

export interface SearchResponse {
  session: Session;
  products: Product[];
  page: number;
  limit: number;
  total: number;
  has_more: boolean;
}

export interface CartResponse {
  session: Session;
  state: string;
  currency: Money;
  products: Product[];
  totals: Totals;
  shipping_options?: ShippingOption[];
  [key: string]: unknown;
}

export interface Address {
  email?: string;
  first_name?: string;
  last_name?: string;
  /** Street address. Also accepted as `address1`. */
  line_one: string;
  line_two?: string;
  city: string;
  /** State or province abbreviation. Also accepted as `state`. */
  region_id?: string;
  /** Also accepted as `zipcode` or `zip`. */
  postal_code: string;
  /** ISO country code. Defaults to US. Also accepted as `country`. */
  country_id?: string;
  phone?: string;
}

export interface MissingField {
  field: string;
  /** Every name this field is accepted under. */
  accepts: string[];
  description: string;
  example: unknown;
}

export interface AskResult {
  url: string;
  name: string;
  site: string;
  score: number;
  description: string;
  schema_object: Record<string, unknown>;
}

export interface AskResponse {
  _meta: {
    response_type: string;
    version: string;
    query: string;
    site: string;
    count: number;
  };
  results: AskResult[];
}

export interface BatchOperation {
  id?: string;
  method?: "GET" | "POST";
  path: string;
  body?: Record<string, unknown>;
}

export interface BatchResponse {
  count: number;
  requested: number;
  succeeded: number;
  stopped_early: boolean;
  results: Array<{ id: string; status: number; body: unknown }>;
}

export declare class ApparelMonsterError extends Error {
  status: number;
  code: string | null;
  /** Present on a `missing_fields` error: every field the call needed. */
  missing: MissingField[] | null;
  /** The call that unblocks the cart, when the server could work one out. */
  nextCall: Record<string, unknown> | null;
  contract: Record<string, unknown> | null;
  body: Record<string, unknown> | null;
  url: string;
}

export interface ApparelMonsterOptions {
  baseUrl?: string;
  sessionId?: string;
  accessToken?: string;
  fetch?: typeof globalThis.fetch;
  /** Identify as the CLI rather than the library. */
  cli?: boolean;
}

export declare class ApparelMonster {
  constructor(options?: ApparelMonsterOptions);

  baseUrl: string;
  /** Set from the first search response, then sent on every later call. */
  sessionId: string | null;
  accessToken: string | null;

  search(
    query?: string,
    options?: {
      category?: string;
      sort?: "relevance" | "cheapest" | "expensive" | "newest" | "name" | "popular";
      limit?: number;
      page?: number;
    }
  ): Promise<SearchResponse>;

  product(ids: string | string[]): Promise<{ products: Product[] }>;

  addToCart(variantId: string, quantity?: number): Promise<CartResponse>;
  updateCartItem(variantId: string, quantity: number): Promise<CartResponse>;
  removeFromCart(variantId: string): Promise<CartResponse>;
  viewCart(): Promise<CartResponse>;
  setBilling(address: Address): Promise<CartResponse>;
  setShipping(shippingId: string, address?: Partial<Address>): Promise<CartResponse>;
  applyCoupon(code: string): Promise<CartResponse>;
  setPayment(token: string): Promise<CartResponse>;

  checkout(options?: { callbackUrl?: string }): Promise<Record<string, unknown>>;
  wallet(items: Array<{ variant_id: string; quantity?: number }> | string[]): Promise<Record<string, unknown>>;
  track(params?: Record<string, unknown>): Promise<Record<string, unknown>>;

  watchPrice(
    variantId: string,
    targetPrice: number,
    options?: { callbackUrl?: string }
  ): Promise<Record<string, unknown>>;
  priceWatch(id: string): Promise<Record<string, unknown>>;

  batch(operations: BatchOperation[], options?: { stopOnError?: boolean }): Promise<BatchResponse>;

  ask(query: string, options?: { limit?: number }): Promise<AskResponse>;
  askStream(
    query: string,
    options?: { limit?: number }
  ): AsyncGenerator<{ event: string; data: Record<string, unknown> }>;
}

export declare const apparelMonster: ApparelMonster;
export default ApparelMonster;
