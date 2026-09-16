// Command apparel-monster is the store from a terminal.
//
// It identifies itself as go-cli rather than go-sdk, so the store can tell a
// person poking at the API from an integration running in production. Same
// client, one option apart.
//
// Output is JSON unless -text is passed: the usual reader here is a script or
// an agent, not a person.
//
//	go install github.com/mluggy/apparel-monster-dev/go/cmd/apparel-monster@latest
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"

	apparelmonster "github.com/mluggy/apparel-monster-dev/go"
)

const usage = `apparel-monster ` + apparelmonster.Version + ` — agent commerce CLI for apparel.monster

USAGE
  apparel-monster <command> [args] [flags]

COMMANDS
  search <query>              Search the catalog
  product <id...>             Full detail for product or variant ids
  ask <question>              Natural-language search (NLWeb)
  pricing                     Plans, limits and merchandise price ranges
  buy <variant-id>            Mint an Apple Pay / Google Pay link for one variant
  watch <variant-id> <price>  Watch a variant for a target price
  watch-status <id>           Poll a price watch

FLAGS
  -limit N         Results to return
  -category PATH   Browse a category instead of searching
  -sort SORT       relevance | cheapest | expensive | newest | name | popular
  -base URL        Point at another origin
  -text            Human-readable output instead of JSON
  -stream          Stream results as they arrive (ask)

The API needs no credentials. This is a demo store: orders are placed against a
test gateway, nothing ships and no money moves.`

func main() {
	if err := run(); err != nil {
		var apiError *apparelmonster.Error
		if errors.As(err, &apiError) {
			// The API's own message already names the missing fields and the
			// next call; repeating them here would only make it longer.
			code := apiError.Code
			if code == "" {
				code = "error"
			}
			fmt.Fprintf(os.Stderr, "%s: %s\n", code, apiError.Error())
		} else {
			fmt.Fprintln(os.Stderr, err)
		}
		os.Exit(1)
	}
}

func run() error {
	flags := flag.NewFlagSet("apparel-monster", flag.ContinueOnError)
	flags.Usage = func() { fmt.Println(usage) }

	limit := flags.Int("limit", 0, "results to return")
	category := flags.String("category", "", "browse a category")
	sortBy := flags.String("sort", "", "sort order")
	base := flags.String("base", "https://apparel.monster", "origin")
	text := flags.Bool("text", false, "human-readable output")
	stream := flags.Bool("stream", false, "stream results")
	version := flags.Bool("version", false, "print the version")

	args := os.Args[1:]
	if len(args) == 0 {
		fmt.Println(usage)
		return nil
	}
	command := args[0]

	// flag.Parse stops at the first non-flag argument, so
	//
	//	apparel-monster search "denim shirt" -limit 1 -text
	//
	// would leave -limit and -text unparsed and fold them into the query — the
	// search then looks for the literal string `denim shirt -limit 1 -text` and
	// finds nothing. Splitting first means flags work wherever they are typed,
	// which is how every other CLI in this repository behaves.
	flagArgs, rest := splitArgs(args[1:], map[string]bool{
		"text": true, "stream": true, "version": true, "help": true,
	})
	if err := flags.Parse(flagArgs); err != nil {
		return err
	}

	if *version || command == "version" {
		fmt.Println(apparelmonster.Version)
		return nil
	}
	if command == "help" || command == "-h" || command == "--help" {
		fmt.Println(usage)
		return nil
	}

	ctx := context.Background()
	store := apparelmonster.New(apparelmonster.WithBaseURL(*base), apparelmonster.AsCLI())

	switch command {
	case "search":
		found, err := store.Search(ctx, strings.Join(rest, " "), apparelmonster.SearchOptions{
			Category: *category, Sort: *sortBy, Limit: *limit,
		})
		if err != nil {
			return err
		}
		if *text {
			fmt.Println(humaniseProducts(found.Products))
			return nil
		}
		return emit(found)

	case "product":
		if len(rest) == 0 {
			return errors.New("product needs at least one id")
		}
		found, err := store.Product(ctx, rest...)
		if err != nil {
			return err
		}
		if *text {
			fmt.Println(humaniseProducts(found.Products))
			return nil
		}
		return emit(found)

	case "ask":
		if len(rest) == 0 {
			return errors.New("ask needs a question")
		}
		question := strings.Join(rest, " ")

		if *stream {
			events, err := store.AskStream(ctx, question, *limit)
			if err != nil {
				return err
			}
			for event := range events {
				if event.Event != "result" {
					continue
				}
				if *text {
					var hit apparelmonster.AskResult
					if err := json.Unmarshal(event.Data, &hit); err == nil {
						fmt.Printf("%s — %s\n", hit.Name, hit.URL)
					}
				} else {
					fmt.Println(string(event.Data))
				}
			}
			return nil
		}

		answer, err := store.Ask(ctx, question, *limit)
		if err != nil {
			return err
		}
		if *text {
			if len(answer.Results) == 0 {
				fmt.Printf("No match for %q. /ask matches keywords, not meaning — try fewer words (\"coat\" rather than \"warm winter coat\").\n", answer.Meta.Query)
				return nil
			}
			for _, hit := range answer.Results {
				fmt.Printf("%s\n  %s\n  %s\n\n", hit.Name, hit.Description, hit.URL)
			}
			return nil
		}
		return emit(answer)

	case "pricing":
		// The prose document rather than an endpoint: it is the canonical
		// answer to "what does this cost", and markdown reads fine here.
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, *base+"/pricing.md", nil)
		if err != nil {
			return err
		}
		request.Header.Set("User-Agent", "apparel-monster-go-cli/"+apparelmonster.Version)
		request.Header.Set("X-Agent-Client", "go-cli/"+apparelmonster.Version)
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			return err
		}
		defer response.Body.Close()
		body, err := io.ReadAll(response.Body)
		if err != nil {
			return err
		}
		fmt.Println(string(body))
		return nil

	case "buy":
		if len(rest) == 0 {
			return errors.New("buy needs a variant id — get one from `apparel-monster search`")
		}
		link, err := store.Wallet(ctx, rest[0])
		if err != nil {
			return err
		}
		if *text {
			fmt.Printf("Open this to pay:\n  %v\n", link["wallet_url"])
			return nil
		}
		return emit(link)

	case "watch":
		if len(rest) < 2 {
			return errors.New("watch needs a variant id and a target price")
		}
		target, err := strconv.ParseFloat(rest[1], 64)
		if err != nil {
			return fmt.Errorf("target price %q is not a number", rest[1])
		}
		watch, err := store.WatchPrice(ctx, rest[0], target, "")
		if err != nil {
			return err
		}
		if *text {
			if watch["status"] == "already_met" {
				fmt.Printf("Already %v — below your %v target.\n", watch["current_price"], watch["target_price"])
			} else {
				fmt.Printf("Watching. Poll: %v\n", watch["poll_url"])
			}
			return nil
		}
		return emit(watch)

	case "watch-status":
		if len(rest) == 0 {
			return errors.New("watch-status needs a watch id")
		}
		status, err := store.PriceWatch(ctx, rest[0])
		if err != nil {
			return err
		}
		return emit(status)

	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", command)
		fmt.Println(usage)
		os.Exit(1)
	}

	return nil
}

// splitArgs separates flag tokens from positional ones, so they can be typed in
// any order. booleans names the flags that do NOT consume the following token;
// everything else takes a value, either as -flag value or -flag=value.
func splitArgs(args []string, booleans map[string]bool) (flagArgs, positional []string) {
	for i := 0; i < len(args); i++ {
		arg := args[i]

		if arg == "--" {
			positional = append(positional, args[i+1:]...)
			return flagArgs, positional
		}

		if !strings.HasPrefix(arg, "-") || arg == "-" {
			positional = append(positional, arg)
			continue
		}

		flagArgs = append(flagArgs, arg)

		name := strings.TrimLeft(arg, "-")
		if strings.Contains(name, "=") || booleans[name] {
			continue
		}
		// Takes a value, and it is the next token.
		if i+1 < len(args) {
			i++
			flagArgs = append(flagArgs, args[i])
		}
	}
	return flagArgs, positional
}

func emit(value any) error {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(encoded))
	return nil
}

// humaniseProducts is just enough formatting to read a result without jq.
func humaniseProducts(products []apparelmonster.Product) string {
	if len(products) == 0 {
		return "No products matched. Try fewer words, or browse: apparel-monster search -category categories/men"
	}

	blocks := make([]string, 0, len(products))
	for _, product := range products {
		cheapest := 0.0
		sizes := map[string]bool{}
		for _, variant := range product.Variants {
			price := variant.EffectivePrice
			if price == 0 {
				price = variant.Price
			}
			if price > 0 && (cheapest == 0 || price < cheapest) {
				cheapest = price
			}
			if size, ok := variant.Options["Size"]; ok {
				sizes[size] = true
			}
		}

		lines := []string{product.Title}
		if cheapest > 0 {
			lines[0] = fmt.Sprintf("%s  $%.2f", product.Title, cheapest)
		}
		if product.Category != "" {
			lines = append(lines, "  "+product.Category)
		}
		if len(sizes) > 0 {
			names := make([]string, 0, len(sizes))
			for size := range sizes {
				names = append(names, size)
			}
			sort.Strings(names)
			lines = append(lines, "  sizes: "+strings.Join(names, ", "))
		}
		lines = append(lines, "  "+product.URL)
		blocks = append(blocks, strings.Join(lines, "\n"))
	}

	return strings.Join(blocks, "\n\n")
}
