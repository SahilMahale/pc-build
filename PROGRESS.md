# Project Progress

_Last updated: 02 July 2026_

## Current State

**Go web service (Fiber + HTMX)** — A fullstack price tracker that scrapes all 8 components of the July 2026 build ([pc-specs.md](pc-specs.md)) across 20 Indian retailer sources, stores history in SQLite, alerts on target prices, and serves a live dashboard with real-time progress tracking.

Verified end-to-end on 02 Jul 2026 — all selectors working, progress bar operational, mobile-responsive.

## Architecture

```
config.yaml ──> main.go ──> internal/
                   │          ├── config.go   (YAML loading, env vars)
                   │          ├── scrape.go   (HTTP client, retries, alerts)
                   │          ├── store.go    (SQLite queries)
                   │          └── web.go      (Fiber routes, HTMX rendering)
                   │
                   ├──> templates/  (*.html)
                   ├──> static/     (htmx.min.js)
                   └──> prices.db   (SQLite history)
```

- **Scheduling:** in-process goroutine loop, first pass fires at startup; interval from `settings.interval` ("30m" / "6h" / "1d" / seconds)
- **Politeness:** 3–8s randomized delay between requests, rotating user-agents, 20s timeout
- **Retries:** 3 attempts with exponential backoff + jitter on connection errors / 408 / 425 / 429 / 5xx, honours `Retry-After`
- **Schema:** `prices(id, product, site, price, url, scraped_at)` with an index on `(product, scraped_at)`
- **Dashboard:** per-product tables, Δ vs previous pass, all-time low, target highlighting, inline-SVG sparklines, real-time progress bar during scraping

## Verified Selectors Per Site

| Site | Selector | Notes |
|---|---|---|
| mdcomputers | `.price .ins .amount, h2.special-price` | **Two page templates in the wild** — old OpenCart markup on some pages, `h2.special-price` on others. The comma selector covers both. |
| vedant | `div.product-price-new` | Unique to the main product. |
| elitehubs | `meta[itemprop=price]` + `attribute: content` | Shopify store; the meta tag is theme-independent. |
| primeabgb | `p.price .woocommerce-Price-amount bdi` | WooCommerce. |

## Site Quirks

- **mdcomputers silently redirects** unknown product URLs to the homepage with HTTP 200 — a wrong URL looks like a selector failure, not a 404
- **mdcomputers' `product:price:amount` meta lies** — it disagreed with the displayed price on the CPU page (₹48,000 vs ₹51,200 shown). Use the visible price elements, not the meta
- **vedant product pages embed sidebar/related products** whose `.price-new` elements appear *before* the main product's in the DOM. A naive `.price-new` selector returns a sidebar item. JSON-LD `Product` block also carries the true price if the DOM breaks
- **elitehubs is Shopify** — `https://elitehubs.com/search/suggest.json?q=…` is the fastest way to find product URLs + prices programmatically
- **mdcomputers / vedant are OpenCart** — search via `index.php?route=product/search&search=…`
- **Amazon**: dropped. The old config's ASIN 404'd, and Amazon resists scraping anyway (captchas/503s). Keepa is the honest tool there

## Product/Source Decisions

- **RAM = 2 × Corsair Vengeance 16GB DDR5-6000 CL36 (CMK16GX5M1E6000Z36)**, tracked per stick on all three sites. Config target ₹5,750/stick is the old ₹11.5k/32GB budget — far below the current DDR5-shortage market (~₹22k/stick as of Jul 2026); revisit the target
- **Single-source components** (verified, not an oversight):
  - Noctua NH-D15 chromax.black → vedant only (md/elitehubs stock only other Noctua models incl. the newer D15 **G2** — a different product)
  - Adata XPG S70 Blade 1TB → vedant only
- Targets set: CPU ₹38k, RAM ₹5,750/stick, GPU ₹55k. Other components have no `target_price` yet, so they never alert

## Price Snapshot (02 Jul 2026, best across sites)

| Component | Best | Site |
|---|---|---|
| Ryzen 7 9800X3D | ₹47,469 | vedant |
| Noctua NH-D15 chromax.black | ₹10,800 | vedant |
| MSI B850 Tomahawk Max WiFi | ₹23,988 | elitehubs |
| Vengeance 16GB DDR5-6000 CL36 (per stick) | ₹22,150 | vedant |
| Adata XPG S70 Blade 1TB | ₹17,800 | vedant |
| ASRock RX 9070 Challenger | ₹57,549 | vedant |
| Lian Li Vector V100 White | ₹8,190 | vedant |
| Corsair RM750e ATX 3.1 | ₹9,400 | vedant |

**Estimated optimised total: ₹219,496** (cheapest site per component, quantities included)

## Features

- ✅ Real-time progress bar during scraping (done/total sources)
- ✅ Last scrape timestamp with relative time ("5m ago", "2h ago")
- ✅ Auto-refresh panel (2s during scraping, 30s idle)
- ✅ Mobile responsive (tested on iPhone)
- ✅ All product links open in new tabs
- ✅ Environment variable config for HOST/PORT
- ✅ HTMX-driven no-JavaScript-required UX (progressive enhancement)

## Tech Stack

- **Go 1.26+** — stdlib net/http patterns dropped for Fiber v2
- **[Fiber v2](https://gofiber.io/)** — Express-inspired web framework
- **[HTMX](https://htmx.org/)** — HTML-over-the-wire fragments
- **[goquery](https://github.com/PuerkitoBio/goquery)** — CSS selector scraping
- **[modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite)** — pure Go SQLite (cross-compiles cleanly)
- **[gopkg.in/yaml.v3](https://gopkg.in/yaml.v3)** — config parsing

## Build

```bash
go build -o pc-price-server
./pc-price-server
```

Cross-compile for Linux:
```bash
GOOS=linux GOARCH=amd64 go build -o pc-price-server
```
