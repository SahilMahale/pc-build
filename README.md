# pc-build

Everything for my July 2026 PC build:

- **[pc-specs.md](pc-specs.md)** — the full parts list, power budget, build checklist, and Linux (Fedora 42 + Hyprland) compatibility notes
- **[app/](app/)** — a Go web service (Fiber + HTMX) that scrapes component prices across Indian retailers on a schedule, stores history in SQLite, alerts on target prices, and serves a live dashboard
- **[scraper.py](scraper.py)** — the original Python CLI version of the scraper (v1, still works; shares the same `config.yaml` and `prices.db`)
- **[PROGRESS.md](PROGRESS.md)** — project state, verified selectors, and hard-won site quirks

## Go web app (v2)

```bash
cd app
go build -o pc-price-server .
./pc-price-server                # finds ../config.yaml, serves on :8090
```

Scraping starts immediately on launch and repeats on `settings.interval`.
Open <http://localhost:8090>:

- Latest price per site with links, Δ since last pass, all-time low,
  trend sparklines, and target highlighting
- The panel auto-refreshes every 30 s (HTMX fragment swap)
- **Scrape now** button triggers an immediate pass
- Click a row's timestamp for the product's full price history

Settings live in the same [config.yaml](config.yaml) as v1 (`listen:`
controls the port). The DB path resolves relative to the config file, so Go
and Python versions share one `prices.db` — history carries over both ways.

Systemd user service (Fedora):

```ini
# ~/.config/systemd/user/pc-price-server.service
[Unit]
Description=PC component price tracker

[Service]
WorkingDirectory=%h/personal/pc-build
ExecStart=%h/personal/pc-build/app/pc-price-server
Restart=on-failure

[Install]
WantedBy=default.target
```

## Python scraper (v1)

### Setup

```bash
pip install -r requirements.txt
```

### Usage

```bash
./scraper.py                              # scrape forever on the configured interval (default)
./scraper.py once                         # single pass, good for testing selectors
./scraper.py report                       # terminal report: latest price per site + all-time low
./scraper.py history "9800X3D"            # full history (substring match)
```

Running with no arguments starts the periodic loop immediately — the first
pass fires on launch. If a `.venv` exists next to the script it is used
automatically; no need to activate it.

Every scrape pass (loop or `once`) writes **report.html** — open it in a
browser for current prices, change since last pass, all-time lows, and a
price-trend sparkline per site. The page auto-refreshes on the scrape
interval, so you can just leave the tab open.

Config is hot-reloaded every pass while the loop is active — edit
`config.yaml` to add products without restarting. Requests retry on
connection errors / 408 / 429 / 5xx with exponential backoff + jitter and
honour `Retry-After`.

### Adding a product

1. Open the product page in your browser
2. Right-click the price → Inspect → right-click the element → Copy selector
3. Trim it to something short and stable (prefer a class like `span.price-new`
   over auto-generated `#maincontent > div:nth-child(3) > ...`)
4. Add an entry under `products:` in `config.yaml`
5. Verify with `python scraper.py once`

If a price shows "selector matched nothing", the site changed its layout or the
page is rendered by JavaScript (see below).

### Alerts

Set `alert_webhook` in config to a Discord webhook, Slack webhook, or an
ntfy.sh topic URL (`https://ntfy.sh/your-topic`). The payload includes
`content` / `text` / `message` keys so all three services accept it.
ntfy is the quickest path to push notifications on your iPhone — install the
app, subscribe to your topic, done.

### Limitations & notes

- **JavaScript-rendered prices** (some Amazon layouts, Flipkart) won't appear
  in plain HTML. Options: find the price inside an embedded
  `<script type="application/ld+json">` block (add a selector for it),
  or swap `requests` for Playwright for those sites.
- **Amazon actively resists scraping** — expect intermittent captchas/503s at
  higher frequencies. Keep the interval ≥ a few hours and treat failures as
  transient. For serious Amazon tracking, Keepa's API is the honest tool.
- **Be polite:** the default 6h interval and randomized delays are deliberate.
  Hammering small Indian retailers helps nobody and gets your IP blocked.
- Respect each site's robots.txt / terms of service.
