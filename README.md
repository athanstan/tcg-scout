![tcg-scout banner](assets/tcg-scout-image.png)

# tcg-scout

Go CLI and HTTP API for multi-game TCG scraping. One registry drives discovery, execution, and output across adapters.

| Layer | Path |
| --- | --- |
| Entry | `cmd/tcg-scout` |
| Registry | `internal/app` |
| CLI | `internal/cli` |
| API | `internal/api` |
| SVE cards | `internal/tcg/sve/cards` |
| SVE decks | `internal/tcg/sve/decks` |

**Requires:** Go 1.24+

## Quick start

```bash
go mod tidy
go run ./cmd/tcg-scout                    # interactive (TTY)
go run ./cmd/tcg-scout games              # list games
make build                                # bin/tcg-scout with version info
```

## CLI

### Command shape

```text
tcg-scout <game> <resource> [official] <action> [flags]
```

When a resource has one scraper (`official`), both scraper and default action are optional.

| Global flag | Default | Purpose |
| --- | --- | --- |
| `--output` | `auto` | `auto`, `table`, `plain`, `json` |
| `--log-level` | `info` | `debug`, `info`, `warn`, `error` |
| `--config` | — | Path to `.tcg-scout.yaml` |

### Discovery

| Command | Example |
| --- | --- |
| Games | `tcg-scout games` |
| Resources | `tcg-scout resources sve` |
| Scrapers | `tcg-scout scrapers sve cards` |

```bash
go run ./cmd/tcg-scout games --output json
```

```json
{
  "games": [
    { "id": "sve", "name": "Shadowverse Evolve", "summary": "..." }
  ]
}
```

### Interactive

```bash
go run ./cmd/tcg-scout
go run ./cmd/tcg-scout interactive
```

Deck scrapes prompt for a tournament URL when needed.

---

## Games

### Shadowverse Evolve (`sve`)

English Shadowverse Evolve workflows via the official site and Decklog.

#### Cards

Scrape card metadata and WEBP art from the official search pages.

| Action | Command | Writes files |
| --- | --- | --- |
| `scrape` (default) | `tcg-scout sve cards` | `output/cards.json`, `output/images/` |
| `list` | `tcg-scout sve cards list` | No |
| `images` | `tcg-scout sve cards images` | Rebuilds images from existing JSON |

```bash
go run ./cmd/tcg-scout sve cards list --output plain
```

```text
C-001   Arisa
C-002   Erika
```

| Flag | Default | Purpose |
| --- | --- | --- |
| `--search-url` | BP search URL | Card search page |
| `--output-json` | `output/cards.json` | Cards JSON path |
| `--images-dir` | `output/images` | WEBP output dir |
| `--user-agent` | `tcg-scout/1.0` | HTTP user agent |
| `--webp-quality` | `100` | WEBP quality (0–100) |
| `--image-concurrency` | `8` | Concurrent image downloads |
| `--page-concurrency` | `4` | Concurrent page fetches |
| `--allowed-domain` | — | Optional domain allowlist |

```bash
go run ./cmd/tcg-scout sve cards scrape \
  --search-url "https://en.shadowverse-evolve.com/cards/searchresults/?expansion_name=BP17" \
  --webp-quality 90
```

Scrape summary (`--output json`):

```json
{
  "game": "sve",
  "resource": "cards",
  "action": "scrape",
  "card_count": 248,
  "message": "scraped 248 cards"
}
```

#### Decks

Crawl tournament deck lists from event report pages. Deck IDs come from `data-deck-id` in the HTML; payloads are fetched from [Decklog](https://decklog-en.bushiroad.com/).

| Action | Command | Writes files |
| --- | --- | --- |
| `scrape` (default) | `tcg-scout sve decks --tournament-url URL` | `output/decks/{slug}-decks.json` |
| `list` | `tcg-scout sve decks list --tournament-url URL` | No |

```bash
go run ./cmd/tcg-scout sve decks \
  --tournament-url "https://en.shadowverse-evolve.com/decks/bcs2526-sydney/"
```

| Flag | Default | Purpose |
| --- | --- | --- |
| `--tournament-url` | — | Tournament page URL (**required**) |
| `--output-dir` | `output/decks` | JSON output directory |
| `--deck-concurrency` | `4` | Concurrent Decklog fetches |
| `--user-agent` | `tcg-scout/1.0` | HTTP user agent |

Partial success is allowed — failed decks are logged, included in output, and the run continues:

```text
scraped 10/12 decks for bcs2526-spain (solo); 2 failed
```

Solo output (abbreviated):

```json
{
  "title": "Bushiroad Championship Series 25/26 Event Report—Sydney, Australia",
  "link": "https://en.shadowverse-evolve.com/decks/bcs2526-sydney/",
  "date": "Apr. 6, 2026",
  "identifier": "bcs2526-sydney",
  "tournament": {
    "format": "solo",
    "location": "Sydney, Australia",
    "entry_count": 32,
    "decks_total": 32,
    "decks_scraped": 32,
    "crawled_at": "2026-07-11T09:00:00Z"
  },
  "decks": [
    {
      "ranking": 1,
      "player": "JasonZ",
      "craft": "Dragoncraft",
      "wins": 7,
      "standing_label": "1st place · 7 wins",
      "deck_id": "5RC61",
      "deck": {
        "title": "大哥龙",
        "craft": "Dragoncraft",
        "cards": [
          {
            "card_number": "BP01-LD07EN",
            "name": "Rowen",
            "total": 1,
            "rarity": "Leader",
            "craft": "Dragoncraft"
          }
        ]
      }
    }
  ]
}
```

Team output uses the same top-level keys; `decks` contains team groups, each with nested player `decks`:

```json
{
  "title": "Bushiroad Summer Festival 2026 Event Report—Manila, Philippines",
  "link": "https://en.shadowverse-evolve.com/decks/bsf2026-philippines/",
  "date": "Jul. 6, 2026",
  "identifier": "bsf2026-philippines",
  "tournament": {
    "format": "team",
    "location": "Manila, Philippines",
    "team_count": 8,
    "decks_total": 24,
    "decks_scraped": 24
  },
  "decks": [
    {
      "rank": 1,
      "team_name": "UMR",
      "team_wins": 4,
      "standing_label": "1st place team · 4 team wins",
      "decks": [
        { "player": "Soar", "craft": "Forestcraft", "deck_id": "17B23", "deck": {} }
      ]
    }
  ]
}
```

Failed fetch on a solo row:

```json
{
  "player": "Example",
  "deck_id": "Not Registered",
  "deck_fetch_error": "empty decklog response"
}
```

`failed_decks` summarizes any Decklog fetch failures at the file level.

| Format | Source | Notes |
| --- | --- | --- |
| Solo | BCS regionals | One deck per player; `win_scope: "player"` |
| Team | BSF team events | `#team-table` only; `win_scope: "team"` |

---

## Configuration

Precedence: **flags → env (`TCG_SCOUT_*`) → `.tcg-scout.yaml` → defaults**

```bash
export TCG_SCOUT_OUTPUT=json
export TCG_SCOUT_TOURNAMENT_URL="https://en.shadowverse-evolve.com/decks/bcs2526-sydney/"
```

```yaml
# ~/.tcg-scout.yaml
log-level: info
search-url: https://en.shadowverse-evolve.com/cards/searchresults/?expansion_name=BP17
tournament-url: https://en.shadowverse-evolve.com/decks/bcs2526-sydney/
output-dir: output/decks
```

## Legacy aliases

| Old invocation | Rewritten to |
| --- | --- |
| `-names-only` | `sve cards official list` |
| `images-only` | `sve cards official images` |
| `sve cards scrape` | `sve cards official scrape` |
| `sve decks scrape` | `sve decks official scrape` |
| flags only (`--output json`) | `sve cards official scrape` |

## API

```bash
go run ./cmd/tcg-scout serve --addr :8080
curl http://localhost:8080/healthz
```

| Endpoint | Method | Purpose |
| --- | --- | --- |
| `/v1/games` | GET | List games |
| `/v1/games/{game}/resources` | GET | List resources |
| `/v1/games/{game}/resources/{resource}/scrapers` | GET | List scrapers |
| `/v1/runs` | POST | Execute an action |

List cards:

```bash
curl -X POST http://localhost:8080/v1/runs \
  -H "Content-Type: application/json" \
  -d '{"game":"sve","resource":"cards","scraper":"official","action":"list"}'
```

List decks:

```bash
curl -X POST http://localhost:8080/v1/runs \
  -H "Content-Type: application/json" \
  -d '{
    "game": "sve",
    "resource": "decks",
    "scraper": "official",
    "action": "list",
    "options": {
      "tournament_url": "https://en.shadowverse-evolve.com/decks/bcs2526-sydney/"
    }
  }'
```

## Development

```bash
go test ./...
go run ./cmd/tcg-scout completion bash
make build    # injects version, commit, date via ldflags
```

### Add a game or scraper

1. Implement `internal/app.Runner` in `internal/tcg/<game>/<resource>/`.
2. Register it in `cmd/tcg-scout/main.go`.
3. Document it under **Games** in this README.
