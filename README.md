# wlog — a Wanderlog CLI for AI agents

A small, dependency-free Go CLI that lets an AI agent build trips on
[Wanderlog](https://wanderlog.com): find destinations, search places, create
trips, and add places to specific days.

Wanderlog has no public API. This drives the private web-client API and can
break without notice. Use it only with your own account.

## Install

```bash
go install github.com/KRamdath/wanderlog-cli/cmd/wlog@latest
```

Or build from source:

```bash
go build -o wlog ./cmd/wlog
```

Requires Go 1.22+. No third-party dependencies.

## Authentication

Wanderlog authenticates with an Express session cookie (`connect.sid`). There
is no API key and no OAuth app. See [docs/AUTH.md](docs/AUTH.md) for the full
picture.

**Email/password accounts** — the CLI logs in on its own:

```bash
export WANDERLOG_PASSWORD='...'
wlog auth login --email you@example.com
```

**Google/Apple/Facebook accounts** have no password. Copy `connect.sid` from a
signed-in browser (DevTools → Application → Cookies → wanderlog.com) and:

```bash
wlog auth login --cookie 's%3A...'
```

Either way the session is stored at `~/.config/wanderlog/credentials.json`
(mode `0600`). To avoid touching disk, set `WANDERLOG_SESSION` instead — the
preferred route for CI and sandboxed agents.

```bash
wlog auth status
```

## Building a trip

```bash
# 1. Find the destination (this works without logging in).
wlog geo search "kyoto"
# → id 12345, "Kyoto, Japan", lat 35.0116, lng 135.7681

# 2. Create the trip. A destination is required — Wanderlog refuses a
#    trip with no geo. Omit --title and it names itself "Trip to Kyoto".
wlog trip create --geo kyoto --title "Kyoto in spring" --start 2026-04-01 --end 2026-04-05
# → { "key": "abc123xyz", "url": "https://wanderlog.com/plan/abc123xyz" }

# 3. List the days so you have section ids. A date range auto-creates
#    one section per day, plus "Places to visit" and "Notes".
wlog trip sections abc123xyz
# → { "id": 391150968, "heading": "Thursday, October 1st", "type": "normal" }

# 4. Add places to a specific day.
wlog trip add-place abc123xyz --place "Fushimi Inari Taisha" --geo kyoto \
    --section <sectionId> --note "go at sunrise to beat the crowds"

# Omit --section and it lands in "Places to visit".
wlog trip add-place abc123xyz --place "Nishiki Market" --geo kyoto
```

`--place` accepts either a Google place id (`ChIJ...`) or a plain name. A name
needs `--geo NAME` or `--near LAT,LNG` to disambiguate — "Eiffel Tower" matches
Paris, Lahore, and Las Vegas.

To pick the exact place yourself:

```bash
wlog place search "ramen" --geo kyoto --limit 5
wlog trip add-place abc123xyz --place ChIJ... --section <sectionId>
```

## Commands

| Command | Purpose |
|---|---|
| `auth login` | Log in via password or cookie |
| `auth status` | Show the signed-in user |
| `auth logout` | Clear the session |
| `geo search <q>` | Find a destination |
| `place search <q>` | Find places (needs `--geo` or `--near`) |
| `place get <placeId>` | Full place details |
| `trip list` | List your trips |
| `trip create` | Create a trip (requires `--geo`) |
| `trip get <key>` | Raw trip document |
| `trip sections <key>` | List days/sections |
| `trip add-place <key>` | Add a place |
| `trip remove-place <key>` | Remove places |
| `trip delete <key>` | Delete a trip (requires `--yes`) |
| `api <METHOD> <path>` | Call any endpoint directly |

Run `wlog help` for full flags.

## Notes for agents

**Output is always JSON** on stdout; errors go to stderr. Nothing is
interactive and nothing prompts.

**Exit codes are stable:**

| Code | Meaning |
|---|---|
| 0 | Success |
| 1 | Runtime or API error |
| 2 | Usage error |
| 3 | Not logged in — re-authenticate |

Branch on `3` to trigger re-auth rather than parsing error text.

**Flags may appear anywhere,** before or after positional arguments. Both of
these work:

```bash
wlog place search "louvre" --geo paris
wlog place search --geo paris "louvre"
```

**`wlog api` is the escape hatch.** The private API is larger than the modelled
commands and shifts over time:

```bash
wlog api GET /api/tripPlans/home
wlog api POST /api/tripPlans/abc123/like --data '{"liked":true}'
```

Endpoints are catalogued in [docs/API.md](docs/API.md), including the
low-level `applyOps` channel used for fine-grained itinerary edits.

**Destructive actions need confirmation.** `trip delete` refuses without
`--yes`.

## Development

```bash
go test ./...
go build ./...
```

Tests cover argument permutation, path normalisation, the API client's envelope
handling, and the exact create/section payload shapes the server requires. They
use `httptest` and never touch the network.

Every command has been exercised against a real Wanderlog account: create,
read, list, sections, add-place (by name and by place id, with and without a
section), remove-place, and delete.

## Licence

MIT
