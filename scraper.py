#!/usr/bin/env python3
"""
price-scraper — YAML-configured component price tracker.

Usage:
    ./scraper.py                        # scrape forever on the configured interval (default)
    ./scraper.py once                   # single scrape pass
    ./scraper.py report                 # terminal report: latest price + all-time low
    ./scraper.py history "<name>"       # full history for one product

Every scrape pass also writes an HTML report (settings.html_report,
default report.html) — open it in a browser to see current prices,
deltas, all-time lows, and price trends.
"""

from __future__ import annotations

import html
import json
import os
import random
import re
import sqlite3
import sys
import time
import urllib.request
from datetime import datetime
from pathlib import Path

# Re-exec into the project venv so `./scraper.py` works without activating it.
_VENV_PYTHON = Path(__file__).resolve().parent / ".venv" / "bin" / "python"
if _VENV_PYTHON.exists() and not sys.executable.startswith(str(_VENV_PYTHON.parents[1]) + os.sep):
    os.execv(str(_VENV_PYTHON), [str(_VENV_PYTHON), *sys.argv])

import requests
import yaml
from bs4 import BeautifulSoup
from requests.adapters import HTTPAdapter
from urllib3.util.retry import Retry

CONFIG_PATH = Path(__file__).parent / "config.yaml"

# ── config ────────────────────────────────────────────────────────────


def load_config() -> dict:
    with open(CONFIG_PATH) as f:
        return yaml.safe_load(f)


def parse_interval(raw: str | int) -> int:
    """'30m' → 1800, '6h' → 21600, '1d' → 86400, '3600' → 3600."""
    if isinstance(raw, int):
        return raw
    raw = str(raw).strip().lower()
    match = re.fullmatch(r"(\d+)([smhd]?)", raw)
    if not match:
        raise ValueError(f"Bad interval: {raw!r} (use e.g. '30m', '6h', '1d')")
    value, unit = int(match.group(1)), match.group(2) or "s"
    return value * {"s": 1, "m": 60, "h": 3600, "d": 86400}[unit]


# ── storage ───────────────────────────────────────────────────────────


def db_connect(path: str) -> sqlite3.Connection:
    conn = sqlite3.connect(path)
    conn.execute(
        """
        CREATE TABLE IF NOT EXISTS prices (
            id        INTEGER PRIMARY KEY AUTOINCREMENT,
            product   TEXT NOT NULL,
            site      TEXT NOT NULL,
            price     REAL NOT NULL,
            url       TEXT NOT NULL,
            scraped_at TEXT NOT NULL
        )
        """
    )
    conn.execute(
        "CREATE INDEX IF NOT EXISTS idx_product_time ON prices(product, scraped_at)"
    )
    return conn


def save_price(conn, product: str, site: str, price: float, url: str) -> None:
    conn.execute(
        "INSERT INTO prices (product, site, price, url, scraped_at) VALUES (?,?,?,?,?)",
        (product, site, price, url, datetime.now().isoformat(timespec="seconds")),
    )
    conn.commit()


# ── scraping ──────────────────────────────────────────────────────────

PRICE_RE = re.compile(r"[\d,]+(?:\.\d{1,2})?")


def make_session() -> requests.Session:
    """Session with polite retries: only connection errors / 408 / 429 / 5xx,
    exponential backoff with jitter, honours Retry-After."""
    retry = Retry(
        total=3,
        backoff_factor=2,
        backoff_jitter=1,
        status_forcelist=[408, 425, 429, 500, 502, 503, 504],
        allowed_methods=["GET"],
        respect_retry_after_header=True,
    )
    session = requests.Session()
    session.mount("https://", HTTPAdapter(max_retries=retry))
    session.mount("http://", HTTPAdapter(max_retries=retry))
    return session


def extract_price(text: str) -> float | None:
    """Pull the first numeric price out of a string like '₹21,849.00 incl. GST'."""
    match = PRICE_RE.search(text.replace("₹", "").replace("Rs.", ""))
    if not match:
        return None
    try:
        return float(match.group(0).replace(",", ""))
    except ValueError:
        return None


def fetch_price(source: dict, settings: dict, session: requests.Session | None = None) -> float | None:
    headers = {
        "User-Agent": random.choice(settings.get("user_agents") or ["python-price-scraper/1.0"]),
        "Accept-Language": "en-IN,en;q=0.9",
    }
    http = session or requests
    resp = http.get(source["url"], headers=headers, timeout=settings.get("timeout", 20))
    resp.raise_for_status()

    soup = BeautifulSoup(resp.text, "html.parser")
    el = soup.select_one(source["selector"])
    if el is None:
        return None

    raw = el.get(source["attribute"], "") if source.get("attribute") else el.get_text(" ", strip=True)
    return extract_price(raw)


def send_alert(webhook: str, message: str) -> None:
    if not webhook:
        return
    try:
        req = urllib.request.Request(
            webhook,
            data=json.dumps({"content": message, "text": message, "message": message}).encode(),
            headers={"Content-Type": "application/json"},
        )
        urllib.request.urlopen(req, timeout=10)
    except Exception as exc:  # alert failure should never kill the scraper
        print(f"    ! webhook failed: {exc}")


def scrape_all(config: dict, conn) -> None:
    settings = config.get("settings", {})
    lo, hi = settings.get("delay_between_requests", [3, 8])
    session = make_session()

    for product in config.get("products", []):
        name = product["name"]
        target = product.get("target_price")
        print(f"\n▸ {name}")

        for source in product.get("sources", []):
            site = source.get("site", "unknown")
            try:
                price = fetch_price(source, settings, session)
            except requests.RequestException as exc:
                print(f"    {site:<14} ERROR: {exc}")
                continue

            if price is None:
                print(f"    {site:<14} selector matched nothing — page layout may have changed")
                continue

            save_price(conn, name, site, price, source["url"])
            flag = ""
            if target and price <= target:
                flag = f"  🔔 at/below target ₹{target:,.0f}!"
                send_alert(
                    settings.get("alert_webhook", ""),
                    f"{name}: ₹{price:,.0f} at {site} (target ₹{target:,.0f}) {source['url']}",
                )
            print(f"    {site:<14} ₹{price:,.2f}{flag}")

            time.sleep(random.uniform(lo, hi))


# ── reporting ─────────────────────────────────────────────────────────

LATEST_SQL = """
    SELECT product,
           site,
           price,
           url,
           scraped_at,
           MIN(price) OVER (PARTITION BY product) AS all_time_low
    FROM prices p1
    WHERE scraped_at = (
        SELECT MAX(scraped_at) FROM prices p2
        WHERE p2.product = p1.product AND p2.site = p1.site
    )
    ORDER BY product, price
"""


def report(conn) -> None:
    rows = conn.execute(LATEST_SQL).fetchall()

    if not rows:
        print("No data yet — run `./scraper.py once` first.")
        return

    current = None
    for product, site, price, _url, ts, low in rows:
        if product != current:
            print(f"\n▸ {product}   (all-time low: ₹{low:,.2f})")
            current = product
        print(f"    {site:<14} ₹{price:,.2f}   as of {ts}")


def history(conn, product: str) -> None:
    rows = conn.execute(
        "SELECT scraped_at, site, price FROM prices WHERE product LIKE ? ORDER BY scraped_at",
        (f"%{product}%",),
    ).fetchall()
    if not rows:
        print(f"No history matching {product!r}.")
        return
    for ts, site, price in rows:
        print(f"{ts}  {site:<14} ₹{price:,.2f}")


# ── html report ───────────────────────────────────────────────────────

REPORT_CSS = """
  :root { color-scheme: light dark; }
  body { font: 15px/1.5 -apple-system, "Segoe UI", Roboto, sans-serif;
         max-width: 860px; margin: 2rem auto; padding: 0 1rem; }
  h1 { font-size: 1.4rem; } h1 small { font-weight: normal; opacity: .6; font-size: .9rem; }
  h2 { font-size: 1.05rem; margin: 2rem 0 .3rem; }
  h2 .target { font-weight: normal; opacity: .65; font-size: .85rem; margin-left: .6rem; }
  table { border-collapse: collapse; width: 100%; }
  th, td { text-align: left; padding: .45rem .8rem .45rem 0; border-bottom: 1px solid rgba(128,128,128,.25); }
  th { font-size: .8rem; text-transform: uppercase; letter-spacing: .04em; opacity: .6; }
  td.num { font-variant-numeric: tabular-nums; }
  .hit { color: #14894e; font-weight: 600; }
  .down { color: #14894e; } .up { color: #c0392b; }
  .muted { opacity: .55; font-size: .85rem; }
  svg { vertical-align: middle; opacity: .8; }
  a { color: inherit; }
"""


def sparkline(prices: list[float], width: int = 110, height: int = 26) -> str:
    if len(prices) < 2:
        return '<span class="muted">—</span>'
    lo, hi = min(prices), max(prices)
    span = (hi - lo) or 1.0
    step = width / (len(prices) - 1)
    points = " ".join(
        f"{i * step:.1f},{height - 3 - (p - lo) / span * (height - 6):.1f}"
        for i, p in enumerate(prices)
    )
    return (
        f'<svg width="{width}" height="{height}" aria-label="price trend">'
        f'<polyline points="{points}" fill="none" stroke="currentColor" stroke-width="1.5"/></svg>'
    )


def generate_html_report(conn, config: dict) -> Path:
    settings = config.get("settings", {})
    out_path = Path(__file__).parent / settings.get("html_report", "report.html")
    targets = {p["name"]: p.get("target_price") for p in config.get("products", [])}

    sections = []
    products: dict[str, list] = {}
    for product, site, price, url, ts, low in conn.execute(LATEST_SQL).fetchall():
        if product not in targets:  # dropped from config — keep history, hide from report
            continue
        products.setdefault(product, []).append((site, price, url, ts, low))

    for product, sites in products.items():
        target = targets.get(product)
        low = sites[0][4]
        target_note = f'<span class="target">target ₹{target:,.0f}</span>' if target else ""
        body = []
        for site, price, url, ts, _low in sites:
            trend = [
                r[0]
                for r in conn.execute(
                    """
                    SELECT price FROM (
                        SELECT price, scraped_at FROM prices
                        WHERE product = ? AND site = ?
                        ORDER BY scraped_at DESC LIMIT 60
                    ) ORDER BY scraped_at
                    """,
                    (product, site),
                ).fetchall()
            ]
            prev = trend[-2] if len(trend) >= 2 else None
            if prev is None or prev == price:
                delta = '<span class="muted">—</span>'
            else:
                cls, sign = ("down", "▼") if price < prev else ("up", "▲")
                delta = f'<span class="{cls}">{sign} ₹{abs(price - prev):,.0f}</span>'
            hit = ' class="hit"' if target and price <= target else ""
            body.append(
                f"<tr><td><a href=\"{html.escape(url, quote=True)}\">{html.escape(site)}</a></td>"
                f"<td class=\"num\"{hit}>₹{price:,.2f}</td>"
                f"<td class=\"num\">{delta}</td>"
                f"<td>{sparkline(trend)}</td>"
                f"<td class=\"muted\">{html.escape(ts)}</td></tr>"
            )
        sections.append(
            f"<h2>{html.escape(product)}{target_note}</h2>"
            f'<p class="muted">all-time low ₹{low:,.2f}</p>'
            "<table><tr><th>Site</th><th>Price</th><th>Δ last pass</th><th>Trend</th><th>Scraped</th></tr>"
            + "".join(body)
            + "</table>"
        )

    if not sections:
        sections.append("<p>No data yet — the first scrape pass hasn't stored anything.</p>")

    refresh = parse_interval(settings.get("interval", "6h"))
    doc = (
        "<!DOCTYPE html><html lang=\"en\"><head><meta charset=\"utf-8\">"
        f"<meta http-equiv=\"refresh\" content=\"{refresh}\">"
        "<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">"
        "<title>PC component prices</title>"
        f"<style>{REPORT_CSS}</style></head><body>"
        f"<h1>PC component prices <small>generated {datetime.now():%d %b %Y %H:%M}</small></h1>"
        + "".join(sections)
        + "</body></html>"
    )
    out_path.write_text(doc, encoding="utf-8")
    return out_path


# ── main ──────────────────────────────────────────────────────────────


def main() -> None:
    # Line-buffer stdout so logs appear promptly under systemd/redirects
    sys.stdout.reconfigure(line_buffering=True)

    cmd = sys.argv[1] if len(sys.argv) > 1 else "run"
    config = load_config()
    conn = db_connect(config.get("settings", {}).get("database", "prices.db"))

    if cmd == "once":
        scrape_all(config, conn)
        print(f"\nHTML report → {generate_html_report(conn, config)}")
    elif cmd == "run":
        interval = parse_interval(config.get("settings", {}).get("interval", "6h"))
        print(f"Scraping every {interval}s, starting now. Ctrl+C to stop.")
        while True:
            print(f"\n{'=' * 60}\nPass started {datetime.now():%Y-%m-%d %H:%M:%S}")
            try:
                config = load_config()  # hot-reload config each pass
                scrape_all(config, conn)
                print(f"\nHTML report → {generate_html_report(conn, config)}")
            except Exception as exc:
                print(f"Pass failed: {exc}")
            time.sleep(interval)
    elif cmd == "report":
        report(conn)
    elif cmd == "history":
        if len(sys.argv) < 3:
            sys.exit("Usage: ./scraper.py history \"<product name>\"")
        history(conn, sys.argv[2])
    else:
        sys.exit(__doc__)


if __name__ == "__main__":
    main()
