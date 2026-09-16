// Smoke tests against the live API.
//
// Live, not mocked. A mocked client tests the mock; these assert the contract
// published at /api/v1/openapi.yaml, so a nightly red build means the store
// moved — which is what an SDK maintainer wants to hear first.
//
// Nothing here completes an order. The cart flow stops before checkout: this
// is a demo store, but a trail of CI orders would make its own reporting
// useless.
//
// Run the offline subset only with: go test -short ./...
package apparelmonster

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newTestClient() *Client { return New() }

func TestIdentifiesItself(t *testing.T) {
	var seen http.Header

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Clone()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"products":[]}`))
	}))
	defer server.Close()

	store := New(WithBaseURL(server.URL))
	if _, err := store.Search(context.Background(), "anything", SearchOptions{}); err != nil {
		t.Fatalf("search: %v", err)
	}

	if got := seen.Get("User-Agent"); !strings.HasPrefix(got, "apparel-monster-go/") {
		t.Errorf("User-Agent = %q, want an apparel-monster-go/ prefix", got)
	}
	if got, want := seen.Get("X-Agent-Client"), "go-sdk/"+Version; got != want {
		t.Errorf("X-Agent-Client = %q, want %q", got, want)
	}
}

func TestCLIIdentifiesItselfDifferently(t *testing.T) {
	var seen http.Header

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Clone()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	store := New(WithBaseURL(server.URL), AsCLI())
	if _, err := store.Search(context.Background(), "anything", SearchOptions{}); err != nil {
		t.Fatalf("search: %v", err)
	}

	if got := seen.Get("User-Agent"); !strings.Contains(got, "apparel-monster-go-cli/") {
		t.Errorf("User-Agent = %q, want an apparel-monster-go-cli/ marker", got)
	}
	if got, want := seen.Get("X-Agent-Client"), "go-cli/"+Version; got != want {
		t.Errorf("X-Agent-Client = %q, want %q", got, want)
	}
}

func TestBatchRefusesMoreThanTheMaximumBeforeSending(t *testing.T) {
	operations := make([]BatchOperation, 11)
	for i := range operations {
		operations[i] = BatchOperation{Path: "/api/v1/search"}
	}

	if _, err := New().Batch(context.Background(), operations, false); err == nil {
		t.Fatal("expected an error for 11 operations, got nil")
	}
}

func TestErrorMessageNamesMissingFields(t *testing.T) {
	err := &Error{
		Status:  422,
		Code:    "missing_fields",
		Message: "billing is missing 2 required fields: city, postal_code.",
		Missing: []MissingField{
			{Field: "city", Accepts: []string{"city"}, Example: "San Francisco"},
			{Field: "postal_code", Accepts: []string{"postal_code", "zipcode", "zip"}, Example: "94105"},
		},
	}

	message := err.Error()
	for _, want := range []string{"city", "postal_code", "zipcode", "94105"} {
		if !strings.Contains(message, want) {
			t.Errorf("Error() = %q, want it to mention %q", message, want)
		}
	}
}

func TestSearchReturnsProductsAndMintsASession(t *testing.T) {
	if testing.Short() {
		t.Skip("hits the live API")
	}

	store := newTestClient()
	found, err := store.Search(context.Background(), "denim shirt", SearchOptions{Limit: 3})
	if err != nil {
		t.Fatalf("search: %v", err)
	}

	if found.Session.ID == "" {
		t.Fatal("no session id")
	}
	if len(found.Products) == 0 {
		t.Fatal("no products for a known-good query")
	}

	product := found.Products[0]
	if product.Title == "" {
		t.Error("product has no title")
	}
	if !strings.HasPrefix(product.URL, "https://apparel.monster/") {
		t.Errorf("product url %q is not absolute", product.URL)
	}
	if len(product.Variants) == 0 {
		t.Fatal("product has no variants")
	}
	if product.Variants[0].Price <= 0 {
		t.Error("variant price is not positive")
	}

	// Carried from here on without the caller doing anything.
	if store.SessionID != found.Session.ID {
		t.Errorf("SessionID = %q, want %q", store.SessionID, found.Session.ID)
	}
}

func TestCartAddThenPriceShipping(t *testing.T) {
	if testing.Short() {
		t.Skip("hits the live API")
	}

	ctx := context.Background()
	store := newTestClient()

	found, err := store.Search(ctx, "denim shirt", SearchOptions{Limit: 1})
	if err != nil {
		t.Fatalf("search: %v", err)
	}

	variantID := found.Products[0].Variants[0].ID
	for _, variant := range found.Products[0].Variants {
		if variant.Available {
			variantID = variant.ID
			break
		}
	}

	added, err := store.AddToCart(ctx, variantID, 1)
	if err != nil {
		t.Fatalf("add to cart: %v", err)
	}
	if added.Totals.Total <= 0 {
		t.Error("cart total did not move after adding a line")
	}

	billed, err := store.SetBilling(ctx, Address{
		Email:      "sdk-ci@example.com",
		FirstName:  "CI",
		LastName:   "Runner",
		LineOne:    "1 Market St",
		City:       "San Francisco",
		RegionID:   "CA",
		PostalCode: "94105",
		CountryID:  "US",
	})
	if err != nil {
		t.Fatalf("set billing: %v", err)
	}
	if len(billed.ShippingOptions) == 0 {
		t.Error("no shipping options after setting an address")
	}
}

func TestMissingFieldIsReportedAsAMissingField(t *testing.T) {
	if testing.Short() {
		t.Skip("hits the live API")
	}

	ctx := context.Background()
	store := newTestClient()

	found, err := store.Search(ctx, "denim shirt", SearchOptions{Limit: 1})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if _, err := store.AddToCart(ctx, found.Products[0].Variants[0].ID, 1); err != nil {
		t.Fatalf("add to cart: %v", err)
	}

	_, err = store.SetBilling(ctx, Address{LineOne: "1 Market St"})
	if err == nil {
		t.Fatal("expected an error for an incomplete address")
	}

	var apiError *Error
	if !errors.As(err, &apiError) {
		t.Fatalf("error is %T, want *Error", err)
	}
	if apiError.Status != 422 {
		t.Errorf("status = %d, want 422", apiError.Status)
	}
	if apiError.Code != "missing_fields" {
		t.Errorf("code = %q, want missing_fields", apiError.Code)
	}

	fields := map[string]bool{}
	for _, field := range apiError.Missing {
		fields[field.Field] = true
	}
	for _, want := range []string{"city", "postal_code"} {
		if !fields[want] {
			t.Errorf("missing fields %v, want them to include %q", fields, want)
		}
	}
}

func TestAskReturnsSchemaOrgProducts(t *testing.T) {
	if testing.Short() {
		t.Skip("hits the live API")
	}

	answer, err := newTestClient().Ask(context.Background(), "denim shirt", 2)
	if err != nil {
		t.Fatalf("ask: %v", err)
	}

	if answer.Meta.ResponseType != "result_list" {
		t.Errorf("response_type = %q, want result_list", answer.Meta.ResponseType)
	}
	if answer.Meta.Site != "https://apparel.monster" {
		t.Errorf("site = %q", answer.Meta.Site)
	}
	if len(answer.Results) > 0 {
		if got := answer.Results[0].SchemaObject["@type"]; got != "Product" {
			t.Errorf("@type = %v, want Product", got)
		}
	}
}

func TestAskStreamStartsAndCompletes(t *testing.T) {
	if testing.Short() {
		t.Skip("hits the live API")
	}

	events, err := newTestClient().AskStream(context.Background(), "coat", 2)
	if err != nil {
		t.Fatalf("ask stream: %v", err)
	}

	var seen []string
	for event := range events {
		seen = append(seen, event.Event)
	}

	if len(seen) == 0 {
		t.Fatal("no events")
	}
	if seen[0] != "start" {
		t.Errorf("first event = %q, want start", seen[0])
	}
	if seen[len(seen)-1] != "complete" {
		t.Errorf("last event = %q, want complete", seen[len(seen)-1])
	}
}

func TestBatchRunsSeveralOperations(t *testing.T) {
	if testing.Short() {
		t.Skip("hits the live API")
	}

	result, err := newTestClient().Batch(context.Background(), []BatchOperation{
		{ID: "a", Path: "/api/v1/search", Body: map[string]any{"query": "denim", "limit": 1}},
		{ID: "b", Path: "/api/v1/search", Body: map[string]any{"query": "coat", "limit": 1}},
	}, false)
	if err != nil {
		t.Fatalf("batch: %v", err)
	}

	if result.Requested != 2 {
		t.Errorf("requested = %d, want 2", result.Requested)
	}
	for _, operation := range result.Results {
		if operation.Status != 200 {
			t.Errorf("operation %s status = %d, want 200", operation.ID, operation.Status)
		}
	}
}
