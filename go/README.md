# apparel-monster (Go)

SDK and CLI for the [Apparel Monster](https://apparel.monster) agent commerce
API. Standard library only.

```
go get github.com/mluggy/apparel-monster-dev/go
go install github.com/mluggy/apparel-monster-dev/go/cmd/apparel-monster@latest
```

```go
store := apparelmonster.New()

found, err := store.Search(ctx, "denim shirt", apparelmonster.SearchOptions{Limit: 3})
if err != nil {
    log.Fatal(err)
}

// Add a VARIANT, never a product: prices live on variants.
if _, err := store.AddToCart(ctx, found.Products[0].Variants[0].ID, 1); err != nil {
    log.Fatal(err)
}

cart, err := store.SetBilling(ctx, apparelmonster.Address{
    Email: "shopper@example.com", LineOne: "1 Market St",
    City: "San Francisco", RegionID: "CA", PostalCode: "94105",
})
```

Errors carry the fields they needed:

```go
var apiError *apparelmonster.Error
if errors.As(err, &apiError) {
    apiError.Code     // "missing_fields"
    apiError.Missing  // []MissingField{{Field: "city", Accepts: []string{"city"}, ...}}
    apiError.NextCall // the call that unblocks a stuck cart
}
```

`go test -short ./...` skips the tests that hit the live API.

apparel.monster is a demo storefront: orders are placed against a test gateway,
nothing ships and no money moves.
