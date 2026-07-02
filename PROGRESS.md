# Project Progress

_Last updated: 02 July 2026_

## Where things stand

**v1 (Python) — working.** A YAML-configured price scraper tracks all 8
components of the July 2026 build ([pc-specs.md](pc-specs.md)) across Indian
retailers: 20 sources, every one verified live on 02 Jul 2026. `./scraper.py`
starts the periodic loop immediately (first pass on launch), stores history
in SQLite (`prices.db`), writes `report.html` after every pass, and can POST
webhook alerts when a target price is hit.

**Next: v2 (Go) rewrite** — a fullstack service (Fiber + HTMX) that runs the
same scraping loop and serves the dashboard over HTTP instead of writing a
static report file.

## Architecture (v1)

```
config.yaml ──> scraper.py ──> prices.db (SQLite)
   (hot-reloaded       │
    every pass)        ├──> report.html   (regenerated every pass)
                       └──> webhook alert (target price hit)
```

- **Scheduling:** in-process loop, first pass fires at startup; interval from
  `settings.interval` ("30m" / "6h" / "1d" / seconds).
- **Politeness:** 3–8 s randomized delay between requests, rotating
  user-agents, 20 s timeout.
- **Retries:** `urllib3.util.Retry` on a `requests.Session` — 3 attempts,
  exponential backoff + jitter, only on connection errors / 408 / 425 / 429 /
  5xx, honours `Retry-After`.
- **Schema:** `prices(id, product, site, price, url, scraped_at)` with an
  index on `(product, scraped_at)`. The Go rewrite should reuse this exact
  schema/db so history carries over.
- **Report:** dependency-free HTML — per-product tables, Δ vs previous pass,
  all-time low, target highlighting, inline-SVG sparklines. Only products in
  the current config are shown (retired products keep their DB history).

## Verified selectors per site

| Site | Selector | Notes |
|---|---|---|
| mdcomputers | `.price .ins .amount, h2.special-price` | **Two page templates in the wild** — old OpenCart markup on some pages, `h2.special-price` on others. The comma selector covers both. |
| vedant | `div.product-price-new` | Unique to the main product. |
| elitehubs | `meta[itemprop=price]` + `attribute: content` | Shopify store; the meta tag is theme-independent. |
| primeabgb | `p.price .woocommerce-Price-amount bdi` | WooCommerce. |

## Site quirks (hard-won — don't relearn these)

- **mdcomputers silently redirects** unknown product URLs to the homepage
  with HTTP 200 — a wrong URL looks like a selector failure, not a 404.
- **mdcomputers' `product:price:amount` meta lies** — it disagreed with the
  displayed price on the CPU page (₹48,000 vs ₹51,200 shown). Use the visible
  price elements, not the meta.
- **vedant product pages embed sidebar/related products** whose
  `.price-new` elements appear *before* the main product's in the DOM.
  A naive `.price-new` selector returns a sidebar item. JSON-LD `Product`
  block also carries the true price if the DOM breaks.
- **elitehubs is Shopify** — `https://elitehubs.com/search/suggest.json?q=…`
  is the fastest way to find product URLs + prices programmatically.
- **mdcomputers / vedant are OpenCart** — search via
  `index.php?route=product/search&search=…`.
- **Amazon**: dropped. The old config's ASIN 404'd, and Amazon resists
  scraping anyway (captchas/503s). Keepa is the honest tool there.

## Product/source decisions

- **RAM = 2 × Corsair Vengeance 16GB DDR5-6000 CL36 (CMK16GX5M1E6000Z36)**,
  tracked per stick on all three sites. Config target ₹5,750/stick is the
  old ₹11.5k/32GB budget — far below the current DDR5-shortage market
  (~₹22k/stick as of Jul 2026); revisit the target.
- **Single-source components** (verified, not an oversight):
  Noctua NH-D15 chromax.black → vedant only (md/elitehubs stock only other
  Noctua models incl. the newer D15 **G2** — a different product).
  Adata XPG S70 Blade 1TB → vedant only.
- Targets set: CPU ₹38k, RAM ₹5,750/stick, GPU ₹55k. Other components have
  no `target_price` yet, so they never alert.

## Price snapshot (02 Jul 2026, best across sites)

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

## v2 (Go) — built 02 Jul 2026

Single service in [app/](app/): scrape loop (same behavior as v1) + HTTP
server. Verified end-to-end — a full 20-source pass produced prices
identical to v1's.

- Stack: Fiber v2, HTMX (server-rendered fragments, vendored
  `static/htmx.min.js`), goquery for selectors, `modernc.org/sqlite`
  (pure Go — cross-compiles to the Fedora box with plain `GOOS=linux
  GOARCH=amd64 go build`), `yaml.v3`.
- Reuses `config.yaml` (new `listen:` setting) and `prices.db` — v1 history
  carried over, either version can keep writing.
- Routes: `/` dashboard, `/prices` HTMX fragment (auto-swap every 30 s),
  `POST /scrape` scrape-now (drops concurrent passes), `/history/:product`.
- Engine parity with v1: hot-reload config each pass, first pass on launch,
  3–8 s polite delays, UA rotation, retry w/ backoff+jitter on
  408/425/429/5xx honouring Retry-After, webhook alerts on target hit.
- v1 (`scraper.py`) kept as the lightweight CLI alternative.
