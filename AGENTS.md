# Working in this repository

Four clients for one API, in Node, Python, Ruby and Go, plus a CLI in each. The
API they wrap is <https://apparel.monster/api/v1>; its contract is
<https://apparel.monster/api/v1/openapi.yaml>, and that document is the source
of truth, not this code.

## The rule that matters

**Every change lands in all four languages or in none of them.** A method that
exists in the Node SDK and not the Ruby one is worse than a method that exists
nowhere: it makes the docs wrong for three quarters of users, and it is
invisible until someone hits it. The same applies to behaviour — if the Python
client retries something, so must Go.

Parity is on purpose down to the method names, allowing for each language's
conventions: `addToCart`, `add_to_cart`, `add_to_cart`, `AddToCart`.

## Layout

```
src/                 Node SDK (index.js), types (index.d.ts), CLI (cli.js)
test/                Node tests
python/src/apparel_monster/   _client.py, cli.py
python/tests/
ruby/lib/            apparel_monster.rb
ruby/exe/            the CLI executable
ruby/test/
go/                  apparelmonster.go, cmd/apparel-monster/
skills/              SKILL.md files, published to skills.sh
plugin.json          Agent Plugins manifest
mcp.json             MCP server pointers for agent runtimes
```

## Non-negotiables

**Zero runtime dependencies.** `fetch`, `urllib`, `net/http`, `net/http`.
Someone installing a shopping client should not inherit a transitive tree, and
these clients are meant to work inside an agent sandbox with no package index
reachable.

**No API key anywhere.** Everything except `track` is anonymous. If you find
yourself adding an auth parameter to `search`, the API changed and this
repository is not where that decision gets made.

**Identify the client.** Every request sends
`User-Agent: apparel-monster-<language>[-cli]/<version>` and
`X-Agent-Client: <language>-(sdk|cli)/<version>`. The store uses both to
attribute orders, so a new client or a renamed one has to be added to the
server's allowlist (`AgentApi::Client::KNOWN`) in the same change, or its
traffic silently records as anonymous.

**Carry the session.** The API mints `session.id` on the first search and
expects it on every cart and checkout call. The client does this so callers
cannot forget; do not add a method that bypasses it.

**Surface the error detail.** This API answers a bad call by naming every field
it needed, the aliases each is accepted under, and an example. Clients expose
that as structured data (`error.missing`) *and* fold it into the error message,
because the common case is a model reading the message and nothing else. Do not
collapse an error to its status code.

## Tests

They run against the live API on purpose. A mocked client tests the mock; these
assert the published contract, so a red nightly build means the store moved —
which is the first thing an SDK maintainer wants to know.

They stop short of completing an order. apparel.monster is a demo store, but a
trail of CI orders would make its own reporting useless. If you need to exercise
checkout, do it by hand and say so in the PR.

```
npm test
cd python && PYTHONPATH=src python -m unittest discover -s tests
cd ruby   && ruby -Ilib test/apparel_monster_test.rb
cd go     && go test ./...      # -short skips live calls
```

## Releasing

Versions move together across all four packages, including the `VERSION`
constant each client sends in its headers. In order: bump, run every suite,
tag `vX.Y.Z`, then `npm publish`, `python -m build && twine upload`,
`gem build && gem push`, and for Go the tag *is* the release — `go install`
resolves it from the repository, with no registry step.

## Style

Comments explain why, not what. The reason a line exists — a spec requirement,
a protocol quirk, a mistake that was made once already — is the part a reader
cannot recover from the code.
