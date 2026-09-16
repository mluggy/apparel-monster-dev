#!/usr/bin/env node
/**
 * apparel-monster — the store from a terminal.
 *
 * Identifies itself as node-cli rather than node-sdk, so the store can tell a
 * person poking at the API from an integration running in production. Same
 * client, one flag apart.
 *
 * Every command prints JSON unless --text is passed, because the usual reader
 * here is a script or an agent, not a person.
 */

import { ApparelMonster, ApparelMonsterError, VERSION } from "./index.js";

const USAGE = `apparel-monster ${VERSION} — agent commerce CLI for apparel.monster

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
  tools                       List the MCP tools the store exposes
  version                     Print the version

FLAGS
  --limit <n>       Results to return (search, ask)
  --category <path> Browse a category instead of searching
  --sort <sort>     relevance | cheapest | expensive | newest | name | popular
  --base <url>      Point at another origin
  --text            Human-readable output instead of JSON
  --stream          Stream results as they arrive (ask)

EXAMPLES
  apparel-monster search "denim shirt" --limit 3 --text
  apparel-monster ask "warm winter coat under $150" --stream
  apparel-monster buy 118

The API needs no credentials. This is a demo store: orders are placed against a
test gateway, nothing ships and no money moves.`;

function parseArgs(argv) {
  const args = [];
  const flags = {};
  for (let i = 0; i < argv.length; i++) {
    const arg = argv[i];
    if (!arg.startsWith("--")) {
      args.push(arg);
      continue;
    }
    const name = arg.slice(2);
    // Boolean flags take no value; everything else consumes the next token.
    if (["text", "stream", "help", "version"].includes(name)) flags[name] = true;
    else flags[name] = argv[++i];
  }
  return { args, flags };
}

function out(data, flags) {
  if (!flags.text) {
    console.log(JSON.stringify(data, null, 2));
    return;
  }
  console.log(humanise(data));
}

/** Just enough formatting to read a result in a terminal without jq. */
function humanise(data) {
  if (Array.isArray(data?.products)) {
    return data.products
      .map((p) => {
        const prices = (p.variants ?? []).map((v) => v.effective_price ?? v.price).filter(Boolean);
        const from = prices.length ? `$${Math.min(...prices).toFixed(2)}` : "price on request";
        const sizes = [...new Set((p.variants ?? []).map((v) => v.options?.Size).filter(Boolean))];
        return [
          `${p.title}  ${from}`,
          `  ${p.category ?? ""}`,
          sizes.length ? `  sizes: ${sizes.join(", ")}` : null,
          `  ${p.url}`,
        ]
          .filter(Boolean)
          .join("\n");
      })
      .join("\n\n");
  }
  if (Array.isArray(data?.results)) {
    // An empty result set printed as an empty string looks like the command
    // failed. /ask matches keywords, not meaning, so the useful advice is
    // fewer words rather than a rephrasing.
    if (data.results.length === 0) {
      return `No match for ${JSON.stringify(data._meta?.query ?? "")}. /ask matches keywords, not meaning — try fewer words ("coat" rather than "warm winter coat"), or browse with: apparel-monster search --category categories/women`;
    }
    return data.results.map((r) => `${r.name}\n  ${r.description}\n  ${r.url}`).join("\n\n");
  }
  if (Array.isArray(data?.products) && data.products.length === 0) {
    return `No products matched. Try fewer words, or browse a category: apparel-monster search --category categories/men`;
  }
  return JSON.stringify(data, null, 2);
}

async function main() {
  const { args, flags } = parseArgs(process.argv.slice(2));
  const [command, ...rest] = args;

  if (!command || flags.help || command === "help") {
    console.log(USAGE);
    return;
  }
  if (command === "version" || flags.version) {
    console.log(VERSION);
    return;
  }

  const store = new ApparelMonster({ baseUrl: flags.base, cli: true });
  const limit = flags.limit ? Number(flags.limit) : undefined;

  switch (command) {
    case "search": {
      const result = await store.search(rest.join(" ") || undefined, {
        limit,
        category: flags.category,
        sort: flags.sort,
      });
      out(result, flags);
      break;
    }

    case "product": {
      if (rest.length === 0) throw new Error("product needs at least one id");
      out(await store.product(rest), flags);
      break;
    }

    case "ask": {
      const question = rest.join(" ");
      if (!question) throw new Error("ask needs a question");

      if (flags.stream) {
        for await (const { event, data } of store.askStream(question, { limit })) {
          if (event === "result") console.log(flags.text ? `${data.name} — ${data.url}` : JSON.stringify(data));
          else if (event === "complete" && !flags.text) console.log(JSON.stringify({ event, data }));
        }
        break;
      }
      out(await store.ask(question, { limit }), flags);
      break;
    }

    case "pricing": {
      // The prose document rather than an endpoint: it is the canonical answer
      // to "what does this cost", and it is markdown, which reads fine here.
      const res = await fetch(`${store.baseUrl}/pricing.md`, {
        headers: { "user-agent": `apparel-monster-node-cli/${VERSION}`, "x-agent-client": `node-cli/${VERSION}` },
      });
      console.log(await res.text());
      break;
    }

    case "buy": {
      const variant = rest[0];
      if (!variant) throw new Error("buy needs a variant id — get one from `apparel-monster search`");
      const result = await store.wallet([{ variant_id: variant, quantity: 1 }]);
      if (flags.text) {
        console.log(`Open this to pay:\n  ${result.wallet_url}\n\n${result.instructions ?? ""}`);
      } else {
        out(result, flags);
      }
      break;
    }

    case "watch": {
      const [variant, price] = rest;
      if (!variant || !price) throw new Error("watch needs a variant id and a target price");
      const result = await store.watchPrice(variant, Number(price));
      if (flags.text) {
        console.log(
          result.status === "already_met"
            ? `Already ${result.current_price} — below your ${result.target_price} target. Nothing to wait for.`
            : `Watching. Poll: ${result.poll_url}`
        );
      } else {
        out(result, flags);
      }
      break;
    }

    case "watch-status": {
      if (!rest[0]) throw new Error("watch-status needs a watch id");
      out(await store.priceWatch(rest[0]), flags);
      break;
    }

    case "tools": {
      const res = await fetch(`${store.baseUrl}/mcp`, {
        method: "POST",
        headers: {
          "content-type": "application/json",
          "user-agent": `apparel-monster-node-cli/${VERSION}`,
          "x-agent-client": `node-cli/${VERSION}`,
        },
        body: JSON.stringify({ jsonrpc: "2.0", id: 1, method: "tools/list", params: {} }),
      });
      const data = await res.json();
      const tools = data?.result?.tools ?? [];
      if (flags.text) console.log(tools.map((t) => `${t.name}\n  ${t.description}`).join("\n\n"));
      else out(tools, flags);
      break;
    }

    default:
      console.error(`Unknown command: ${command}\n`);
      console.log(USAGE);
      process.exitCode = 1;
  }
}

main().catch((error) => {
  if (error instanceof ApparelMonsterError) {
    // The API's own message already names the missing fields and the next
    // call; repeating them here would only make it longer.
    console.error(`${error.code ?? "error"}: ${error.message}`);
  } else {
    console.error(error.message);
  }
  process.exitCode = 1;
});
