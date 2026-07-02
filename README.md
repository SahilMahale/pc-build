# pc-build

Everything for my July 2026 PC build:

- **[pc-specs.md](pc-specs.md)** — the full parts list, power budget, build checklist, and Linux (Fedora 42 + Hyprland) compatibility notes
- **[PROGRESS.md](PROGRESS.md)** — project state, verified selectors, and hard-won site quirks

## Price Tracker

A Go web service (Fiber + HTMX) that scrapes component prices across Indian retailers on a schedule, stores history in SQLite, alerts on target prices, and serves a live dashboard.

### Quick Start

```bash
go build -o pc-price-server
./pc-price-server                # finds config.yaml, serves on 0.0.0.0:8090
```

Scraping starts immediately on launch and repeats on `settings.interval`.
Open <http://localhost:8090>:

- **Estimated optimised total** — cheapest listed site per component, quantities included (`quantity:` in config, e.g. 2× RAM sticks)
- Latest price per site with links, Δ since last pass, all-time low, trend sparklines, and target highlighting
- The panel auto-refreshes every 2s during scraping, 30s otherwise (HTMX polling)
- **Scrape now** button triggers an immediate pass
- Progress bar shows scraping status
- Click a row's timestamp for the product's full price history

### Configuration

Settings live in [config.yaml](config.yaml):

```yaml
settings:
  interval: "6h"                    # scrape frequency (30m, 6h, 1d, etc.)
  delay_between_requests: [3, 8]    # random delay range in seconds
  timeout: 20                       # HTTP timeout in seconds
  database: "prices.db"             # SQLite database path
  listen: "0.0.0.0:8090"            # server address (overridden by HOST/PORT env vars)
  alert_webhook: ""                 # Discord/Slack/ntfy.sh webhook URL
  user_agents: []                   # rotating user agents (optional)

products:
  - name: "Product Name"
    target_price: 50000             # alert when price <= this (optional)
    quantity: 2                     # units needed (default: 1)
    sources:
      - site: "SiteName"
        url: "https://..."
        selector: "span.price"      # CSS selector
        attribute: ""               # extract from attribute instead of text (optional)
```

### Environment Variables

- `HOST` — bind address (default: `0.0.0.0`)
- `PORT` — bind port (default: `8090`)

Example for local network access:
```bash
HOST=0.0.0.0 PORT=8090 ./pc-price-server
```

### Deployment

Systemd user service (Fedora):

```ini
# ~/.config/systemd/user/pc-price-server.service
[Unit]
Description=PC component price tracker

[Service]
WorkingDirectory=%h/personal/pc-build
ExecStart=%h/personal/pc-build/pc-price-server
Restart=on-failure

[Install]
WantedBy=default.target
```

```bash
systemctl --user enable --now pc-price-server
systemctl --user status pc-price-server
```

### Adding a Product

1. Open the product page in your browser
2. Right-click the price → Inspect → right-click the element → Copy selector
3. Trim it to something short and stable (prefer a class like `span.price-new` over auto-generated `#maincontent > div:nth-child(3) > ...`)
4. Add an entry under `products:` in `config.yaml`
5. Verify with a single scrape pass

If a price shows "selector matched nothing", the site changed its layout or the page is rendered by JavaScript.

### Alerts

Set `alert_webhook` in config to a Discord webhook, Slack webhook, or an ntfy.sh topic URL (`https://ntfy.sh/your-topic`). The payload includes `content` / `text` / `message` keys so all three services accept it.

ntfy is the quickest path to push notifications on your iPhone — install the app, subscribe to your topic, done.

### Limitations & Notes

- **JavaScript-rendered prices** (some Amazon layouts, Flipkart) won't appear in plain HTML. Options: find the price inside an embedded `<script type="application/ld+json">` block (add a selector for it), or swap the HTTP client for a headless browser.
- **Amazon actively resists scraping** — expect intermittent captchas/503s at higher frequencies. Keep the interval ≥ a few hours and treat failures as transient. For serious Amazon tracking, Keepa's API is the honest tool.
- **Be polite:** the default 6h interval and randomized delays are deliberate. Hammering small Indian retailers helps nobody and gets your IP blocked.
- Respect each site's robots.txt / terms of service.

### Architecture

```
config.yaml ──> scraper loop ──> prices.db (SQLite)
                    │
                    └──> HTTP server (Fiber + HTMX dashboard)
                    └──> webhook alert (target price hit)
```

- **Scheduling:** in-process goroutine loop, first pass fires at startup; interval from `settings.interval`
- **Politeness:** 3–8s randomized delay between requests, rotating user-agents, 20s timeout
- **Retries:** 3 attempts with exponential backoff + jitter on connection errors / 408 / 425 / 429 / 5xx, honours `Retry-After`
- **Schema:** `prices(id, product, site, price, url, scraped_at)` with an index on `(product, scraped_at)`
- **Dashboard:** dependency-free HTML — per-product tables, Δ vs previous pass, all-time low, target highlighting, inline-SVG sparklines, real-time progress bar

### Tech Stack

- **Backend:** Go 1.26+ stdlib + [Fiber v2](https://gofiber.io/)
- **Frontend:** HTMX fragments, vanilla CSS
- **Scraping:** [goquery](https://github.com/PuerkitoBio/goquery) for CSS selectors
- **Database:** [modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite) (pure Go, cross-compiles easily)
- **Config:** YAML via [gopkg.in/yaml.v3](https://gopkg.in/yaml.v3)
