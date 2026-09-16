// Package apparelmonster is a dependency-free client for the Apparel Monster
// agent commerce API.
//
// No API key appears in this package and that is not an omission. Search,
// Product, Cart, Checkout, Wallet, PriceWatch, Batch and Ask are anonymous.
// Only order history (Track) needs OAuth. If something asks you for an Apparel
// Monster API key, it is not us.
//
// Standard library only, so adding this to a project pulls in nothing.
//
//	store := apparelmonster.New()
//	found, err := store.Search(ctx, "denim shirt", apparelmonster.SearchOptions{Limit: 1})
//	if err != nil {
//	    log.Fatal(err)
//	}
//	_, err = store.AddToCart(ctx, found.Products[0].Variants[0].ID, 1)
//
// apparel.monster is a demonstration storefront: orders are placed against a
// test gateway, nothing is fulfilled and no money moves.
package apparelmonster

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Version is reported in the User-Agent and X-Agent-Client headers.
const Version = "1.0.2"

const (
	defaultBaseURL = "https://apparel.monster"
	apiPrefix      = "/api/v1"
)

// MissingField describes one field the API needed and did not get.
//
// Accepts lists every name the field is taken under: this API answers to
// line_one or address1, postal_code or zipcode or zip, because agents arrive
// carrying schemas from other stores.
type MissingField struct {
	Field       string   `json:"field"`
	Accepts     []string `json:"accepts"`
	Description string   `json:"description"`
	Example     any      `json:"example"`
}

// Error is returned for any non-2xx response.
//
// Missing is the interesting part: rather than a generic 4xx, the API names
// every field it needed, and Error's message repeats them, because the common
// case is a caller logging err and nothing else.
type Error struct {
	Status   int
	Code     string
	Message  string
	Missing  []MissingField
	NextCall map[string]any
	Contract map[string]any
	URL      string
}

func (e *Error) Error() string {
	parts := []string{e.Message}
	if len(parts[0]) == 0 {
		parts[0] = fmt.Sprintf("request failed with %d", e.Status)
	}

	if len(e.Missing) > 0 {
		described := make([]string, 0, len(e.Missing))
		for _, field := range e.Missing {
			names := field.Accepts
			if len(names) == 0 {
				names = []string{field.Field}
			}
			described = append(described, fmt.Sprintf("%s (send as %s, e.g. %v)",
				field.Field, strings.Join(names, " or "), field.Example))
		}
		parts = append(parts, "Missing: "+strings.Join(described, "; "))
	}

	if e.NextCall != nil {
		if encoded, err := json.Marshal(e.NextCall); err == nil {
			parts = append(parts, "Next call: "+string(encoded))
		}
	}

	return strings.Join(parts, " ")
}

// Money mirrors the API's currency object.
type Money struct {
	ID string `json:"id"`
}

// Variant is a purchasable item. Prices live here, never on the Product: a
// product with five sizes has five variants and five prices.
type Variant struct {
	ID             string            `json:"id"`
	SKU            string            `json:"sku"`
	Title          string            `json:"title"`
	Price          float64           `json:"price"`
	EffectivePrice float64           `json:"effective_price"`
	Currency       Money             `json:"currency"`
	Available      bool              `json:"available"`
	Options        map[string]string `json:"options"`
	ImageURL       string            `json:"image_url"`
	URL            string            `json:"url"`
}

// Image is one product image.
type Image struct {
	URL string `json:"url"`
}

// Product is a catalog entry with its purchasable variants.
type Product struct {
	ID       string    `json:"id"`
	Title    string    `json:"title"`
	URL      string    `json:"url"`
	Category string    `json:"category"`
	Images   []Image   `json:"images"`
	Variants []Variant `json:"variants"`
}

// Session identifies one shopping session; the client carries it for you.
type Session struct {
	ID string `json:"id"`
}

// Totals is the money breakdown. Read Total; do not re-derive it.
type Totals struct {
	Subtotal float64 `json:"subtotal"`
	Discount float64 `json:"discount"`
	Tax      float64 `json:"tax"`
	Shipping float64 `json:"shipping"`
	Total    float64 `json:"total"`
}

// ShippingOption is one priced delivery choice.
type ShippingOption struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	Price      float64 `json:"price"`
	Delivery   string  `json:"delivery"`
	IsSelected bool    `json:"is_selected"`
}

// SearchResponse is the result of a catalog search.
type SearchResponse struct {
	Session  Session   `json:"session"`
	Products []Product `json:"products"`
	Page     int       `json:"page"`
	Limit    int       `json:"limit"`
	Total    int       `json:"total"`
	HasMore  bool      `json:"has_more"`
}

// CartResponse is the cart as it stands.
type CartResponse struct {
	Session         Session          `json:"session"`
	State           string           `json:"state"`
	Currency        Money            `json:"currency"`
	Products        []Product        `json:"products"`
	Totals          Totals           `json:"totals"`
	ShippingOptions []ShippingOption `json:"shipping_options"`
}

// Address is the shipping and billing address.
//
// Required: LineOne, City, PostalCode, and Email once per session. Country
// defaults to US. A missing field comes back as an *Error naming all of them.
type Address struct {
	Email      string `json:"email,omitempty"`
	FirstName  string `json:"first_name,omitempty"`
	LastName   string `json:"last_name,omitempty"`
	LineOne    string `json:"line_one,omitempty"`
	LineTwo    string `json:"line_two,omitempty"`
	City       string `json:"city,omitempty"`
	RegionID   string `json:"region_id,omitempty"`
	PostalCode string `json:"postal_code,omitempty"`
	CountryID  string `json:"country_id,omitempty"`
	Phone      string `json:"phone,omitempty"`
}

// AskResult is one NLWeb hit.
type AskResult struct {
	URL          string         `json:"url"`
	Name         string         `json:"name"`
	Site         string         `json:"site"`
	Score        float64        `json:"score"`
	Description  string         `json:"description"`
	SchemaObject map[string]any `json:"schema_object"`
}

// AskResponse is an NLWeb result list.
type AskResponse struct {
	Meta struct {
		ResponseType string `json:"response_type"`
		Version      string `json:"version"`
		Query        string `json:"query"`
		Site         string `json:"site"`
		Count        int    `json:"count"`
	} `json:"_meta"`
	Results []AskResult `json:"results"`
}

// BatchOperation is one request inside a batch.
type BatchOperation struct {
	ID     string         `json:"id,omitempty"`
	Method string         `json:"method,omitempty"`
	Path   string         `json:"path"`
	Body   map[string]any `json:"body,omitempty"`
}

// BatchResponse carries one result per operation, each with its own status.
type BatchResponse struct {
	Count        int  `json:"count"`
	Requested    int  `json:"requested"`
	Succeeded    int  `json:"succeeded"`
	StoppedEarly bool `json:"stopped_early"`
	Results      []struct {
		ID     string          `json:"id"`
		Status int             `json:"status"`
		Body   json.RawMessage `json:"body"`
	} `json:"results"`
}

// StreamEvent is one Server-Sent Event from Ask streaming.
type StreamEvent struct {
	Event string
	Data  json.RawMessage
}

// Client talks to one store, carrying one shopping session.
//
// Two things it does that a raw http.Client does not: it carries the session
// id the API mints on the first search (forgetting it is the usual way an
// integration ends up with four empty carts and no order), and it identifies
// itself so the store can attribute the resulting order to this SDK.
//
// A Client is safe for sequential use; the session it carries makes concurrent
// use across goroutines meaningless rather than merely unsafe. Use one Client
// per shopper.
type Client struct {
	BaseURL     string
	SessionID   string
	AccessToken string
	HTTPClient  *http.Client

	client string
}

// Option configures a Client.
type Option func(*Client)

// WithBaseURL points the client at another origin.
func WithBaseURL(base string) Option {
	return func(c *Client) { c.BaseURL = strings.TrimRight(base, "/") }
}

// WithSession resumes an existing agent session.
func WithSession(id string) Option {
	return func(c *Client) { c.SessionID = id }
}

// WithAccessToken supplies the OAuth token that Track needs.
func WithAccessToken(token string) Option {
	return func(c *Client) { c.AccessToken = token }
}

// WithHTTPClient injects an http.Client, for timeouts or tests.
func WithHTTPClient(client *http.Client) Option {
	return func(c *Client) { c.HTTPClient = client }
}

// AsCLI identifies this client as the command line tool rather than the
// library, so the store can tell a person poking at the API from an
// integration running in production.
func AsCLI() Option {
	return func(c *Client) { c.client = "go-cli" }
}

// New builds a client against the public store.
func New(options ...Option) *Client {
	client := &Client{
		BaseURL:    defaultBaseURL,
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
		client:     "go-sdk",
	}
	for _, option := range options {
		option(client)
	}
	return client
}

// SearchOptions narrows a catalog search.
type SearchOptions struct {
	Category string
	Sort     string
	Limit    int
	Page     int
}

// Search searches the catalog, or browses it when query is empty.
//
// It mints the session id every later call needs, so it is almost always the
// first call you make.
func (c *Client) Search(ctx context.Context, query string, options SearchOptions) (*SearchResponse, error) {
	body := map[string]any{}
	if query != "" {
		body["query"] = query
	}
	if options.Category != "" {
		body["category"] = options.Category
	}
	if options.Sort != "" {
		body["sort"] = options.Sort
	}
	if options.Limit > 0 {
		body["limit"] = options.Limit
	}
	if options.Page > 0 {
		body["page"] = options.Page
	}

	var out SearchResponse
	err := c.post(ctx, "/search", body, &out)
	return &out, err
}

// Product returns full detail for product ids or variant ids, mixed freely.
func (c *Client) Product(ctx context.Context, ids ...string) (*SearchResponse, error) {
	var out SearchResponse
	err := c.post(ctx, "/product", map[string]any{"ids": ids}, &out)
	return &out, err
}

// AddToCart adds a VARIANT — not a product. Adding a product id is the most
// common mistake made against this API.
func (c *Client) AddToCart(ctx context.Context, variantID string, quantity int) (*CartResponse, error) {
	if quantity <= 0 {
		quantity = 1
	}
	return c.cart(ctx, map[string]any{"action": "add", "variant_id": variantID, "quantity": quantity})
}

// UpdateCartItem changes the quantity of a line already in the cart.
func (c *Client) UpdateCartItem(ctx context.Context, variantID string, quantity int) (*CartResponse, error) {
	return c.cart(ctx, map[string]any{"action": "update", "variant_id": variantID, "quantity": quantity})
}

// RemoveFromCart removes a line entirely.
func (c *Client) RemoveFromCart(ctx context.Context, variantID string) (*CartResponse, error) {
	return c.cart(ctx, map[string]any{"action": "remove", "variant_id": variantID})
}

// ViewCart returns the cart with its totals.
func (c *Client) ViewCart(ctx context.Context) (*CartResponse, error) {
	return c.cart(ctx, map[string]any{"action": "view"})
}

// SetBilling sets the email and address, and returns the cart with
// ShippingOptions priced against it. This call is what makes them exist.
func (c *Client) SetBilling(ctx context.Context, address Address) (*CartResponse, error) {
	body, err := toMap(address)
	if err != nil {
		return nil, err
	}
	body["action"] = "billing"
	return c.cart(ctx, body)
}

// SetShipping chooses a shipping option by id.
func (c *Client) SetShipping(ctx context.Context, shippingID string) (*CartResponse, error) {
	return c.cart(ctx, map[string]any{"action": "shipping", "shipping_id": shippingID})
}

// ApplyCoupon applies a promotion code.
func (c *Client) ApplyCoupon(ctx context.Context, code string) (*CartResponse, error) {
	return c.cart(ctx, map[string]any{"action": "coupon", "coupon_code": code})
}

// SetPayment attaches a payment token.
//
// The field is token — not payment_token, not card_token. This is a demo store
// on a test gateway: any non-empty string is accepted and nothing is charged.
func (c *Client) SetPayment(ctx context.Context, token string) (*CartResponse, error) {
	return c.cart(ctx, map[string]any{"action": "payment", "token": token})
}

// Checkout places the order. Idempotent per session.
func (c *Client) Checkout(ctx context.Context, callbackURL string) (map[string]any, error) {
	body := map[string]any{}
	if callbackURL != "" {
		body["callback_url"] = callbackURL
	}
	var out map[string]any
	err := c.post(ctx, "/checkout", body, &out)
	return out, err
}

// Wallet mints a hosted Apple Pay / Google Pay link for the given variants.
//
// The shortcut past the whole cart flow: the returned URL collects the
// shopper's email, address and card on their own device, so your agent never
// handles any of them.
func (c *Client) Wallet(ctx context.Context, variantIDs ...string) (map[string]any, error) {
	items := make([]map[string]any, 0, len(variantIDs))
	for _, id := range variantIDs {
		items = append(items, map[string]any{"variant_id": id, "quantity": 1})
	}
	var out map[string]any
	err := c.post(ctx, "/wallet", map[string]any{"items": items}, &out)
	return out, err
}

// Track returns order status and shipments. The only call needing OAuth: pass
// WithAccessToken.
func (c *Client) Track(ctx context.Context, params map[string]any) (map[string]any, error) {
	var out map[string]any
	err := c.post(ctx, "/track", params, &out)
	return out, err
}

// WatchPrice asks to be told when a variant reaches a target price.
//
// Answers 202 with a poll_url and a job_id. A target at or above today's price
// is not queued at all: it comes back already_met, so act on it now.
func (c *Client) WatchPrice(ctx context.Context, variantID string, target float64, callbackURL string) (map[string]any, error) {
	body := map[string]any{"variant_id": variantID, "target_price": target}
	if callbackURL != "" {
		body["callback_url"] = callbackURL
	}
	var out map[string]any
	err := c.post(ctx, "/price-watch", body, &out)
	return out, err
}

// PriceWatch polls a price watch by id.
func (c *Client) PriceWatch(ctx context.Context, id string) (map[string]any, error) {
	var out map[string]any
	err := c.get(ctx, c.BaseURL+apiPrefix+"/price-watch/"+url.PathEscape(id), "application/json", &out)
	return out, err
}

// Batch runs up to ten operations in one round trip, in order.
//
// Each result carries its own status: the batch answers 200 whenever it ran,
// even if every operation inside it failed.
func (c *Client) Batch(ctx context.Context, operations []BatchOperation, stopOnError bool) (*BatchResponse, error) {
	if len(operations) > 10 {
		return nil, fmt.Errorf("apparelmonster: batch takes at most 10 operations, got %d", len(operations))
	}
	var out BatchResponse
	err := c.post(ctx, "/batch", map[string]any{"operations": operations, "stop_on_error": stopOnError}, &out)
	return &out, err
}

// Ask queries in plain language (NLWeb) and returns schema.org Products.
//
// There is no model behind it — it runs the same catalog search — which is why
// it returns nothing rather than inventing a product.
func (c *Client) Ask(ctx context.Context, query string, limit int) (*AskResponse, error) {
	params := url.Values{"query": {query}}
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	}
	var out AskResponse
	err := c.get(ctx, c.BaseURL+"/ask?"+params.Encode(), "application/json", &out)
	return &out, err
}

// AskStream is the same question, streamed: start, one result per hit, then
// complete. The channel is closed when the stream ends.
func (c *Client) AskStream(ctx context.Context, query string, limit int) (<-chan StreamEvent, error) {
	params := url.Values{"query": {query}, "streaming": {"true"}}
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	}

	request, err := c.newRequest(ctx, http.MethodGet, c.BaseURL+"/ask?"+params.Encode(), nil, "text/event-stream")
	if err != nil {
		return nil, err
	}

	response, err := c.HTTPClient.Do(request)
	if err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusOK {
		defer response.Body.Close()
		return nil, c.asError(response, c.BaseURL+"/ask")
	}

	events := make(chan StreamEvent)
	go func() {
		defer close(events)
		defer response.Body.Close()

		scanner := bufio.NewScanner(response.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

		event := "message"
		var data strings.Builder

		for scanner.Scan() {
			line := scanner.Text()
			switch {
			case strings.HasPrefix(line, "event:"):
				event = strings.TrimSpace(line[6:])
			case strings.HasPrefix(line, "data:"):
				data.WriteString(strings.TrimSpace(line[5:]))
			case line == "":
				// A blank line terminates an SSE frame; anything before one is
				// a partial frame and must not be emitted.
				if data.Len() > 0 {
					select {
					case events <- StreamEvent{Event: event, Data: json.RawMessage(data.String())}:
					case <-ctx.Done():
						return
					}
				}
				event = "message"
				data.Reset()
			}
		}
	}()

	return events, nil
}

// ---- plumbing --------------------------------------------------------------

func (c *Client) cart(ctx context.Context, body map[string]any) (*CartResponse, error) {
	var out CartResponse
	err := c.post(ctx, "/cart", body, &out)
	return &out, err
}

func (c *Client) userAgent() string {
	suffix := ""
	if c.client == "go-cli" {
		suffix = "-cli"
	}
	return fmt.Sprintf("apparel-monster-go%s/%s (+https://github.com/mluggy/apparel-monster-dev)", suffix, Version)
}

func (c *Client) newRequest(ctx context.Context, method, url string, body io.Reader, accept string) (*http.Request, error) {
	request, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, err
	}

	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", accept)
	// Two spellings of one fact: the User-Agent is what the edge sees without
	// the application being involved, X-Agent-Client is what survives an
	// environment that rewrites User-Agent.
	request.Header.Set("User-Agent", c.userAgent())
	request.Header.Set("X-Agent-Client", c.client+"/"+Version)
	if c.AccessToken != "" {
		request.Header.Set("Authorization", "Bearer "+c.AccessToken)
	}

	return request, nil
}

func (c *Client) post(ctx context.Context, path string, body map[string]any, out any) error {
	if body == nil {
		body = map[string]any{}
	}
	if c.SessionID != "" {
		if _, taken := body["session"]; !taken {
			body["session"] = map[string]any{"id": c.SessionID}
		}
	}

	encoded, err := json.Marshal(body)
	if err != nil {
		return err
	}

	full := c.BaseURL + apiPrefix + path
	request, err := c.newRequest(ctx, http.MethodPost, full, bytes.NewReader(encoded), "application/json")
	if err != nil {
		return err
	}

	return c.do(request, full, out)
}

func (c *Client) get(ctx context.Context, url, accept string, out any) error {
	request, err := c.newRequest(ctx, http.MethodGet, url, nil, accept)
	if err != nil {
		return err
	}
	return c.do(request, url, out)
}

func (c *Client) do(request *http.Request, url string, out any) error {
	response, err := c.HTTPClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	payload, err := io.ReadAll(response.Body)
	if err != nil {
		return err
	}

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return errorFromBody(response.StatusCode, payload, url)
	}

	if out != nil && len(payload) > 0 {
		if err := json.Unmarshal(payload, out); err != nil {
			return fmt.Errorf("apparelmonster: could not decode response from %s: %w", url, err)
		}
	}

	// The session arrives on the first search and is reused from then on.
	var envelope struct {
		Session Session `json:"session"`
	}
	if err := json.Unmarshal(payload, &envelope); err == nil && envelope.Session.ID != "" {
		c.SessionID = envelope.Session.ID
	}

	return nil
}

func (c *Client) asError(response *http.Response, url string) error {
	payload, _ := io.ReadAll(response.Body)
	return errorFromBody(response.StatusCode, payload, url)
}

func errorFromBody(status int, payload []byte, url string) error {
	var body struct {
		Error    string         `json:"error"`
		Message  string         `json:"message"`
		Missing  []MissingField `json:"missing"`
		NextCall map[string]any `json:"next_call"`
		Contract map[string]any `json:"contract"`
	}
	_ = json.Unmarshal(payload, &body)

	return &Error{
		Status:   status,
		Code:     body.Error,
		Message:  body.Message,
		Missing:  body.Missing,
		NextCall: body.NextCall,
		Contract: body.Contract,
		URL:      url,
	}
}

// toMap round-trips through JSON so the struct tags stay the single definition
// of what each field is called on the wire.
func toMap(value any) (map[string]any, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	out := map[string]any{}
	if err := json.Unmarshal(encoded, &out); err != nil {
		return nil, err
	}
	return out, nil
}
