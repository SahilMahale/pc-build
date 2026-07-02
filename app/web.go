package main

import (
	"fmt"
	"html/template"
	"strconv"
	"strings"
	"time"
)

// inr formats 1234567.5 as "12,34,567.50"? No — v1 used western grouping
// (matches the Python "{:,}" output), so keep "1,234,567.50".
func inr(v float64) string {
	s := strconv.FormatFloat(v, 'f', 2, 64)
	dot := strings.Index(s, ".")
	intPart, frac := s[:dot], s[dot:]
	var b strings.Builder
	for i, ch := range intPart {
		if i > 0 && (len(intPart)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(ch)
	}
	return b.String() + frac
}

// inr0 formats without decimals, for targets.
func inr0(v float64) string {
	s := inr(v)
	return strings.TrimSuffix(s, ".00")
}

// sparkline renders a small inline-SVG polyline of the price series.
func sparkline(prices []float64) template.HTML {
	const width, height = 110.0, 26.0
	if len(prices) < 2 {
		return `<span class="muted">—</span>`
	}
	lo, hi := prices[0], prices[0]
	for _, p := range prices {
		if p < lo {
			lo = p
		}
		if p > hi {
			hi = p
		}
	}
	span := hi - lo
	if span == 0 {
		span = 1
	}
	step := width / float64(len(prices)-1)
	var pts []string
	for i, p := range prices {
		x := float64(i) * step
		y := height - 3 - (p-lo)/span*(height-6)
		pts = append(pts, fmt.Sprintf("%.1f,%.1f", x, y))
	}
	return template.HTML(fmt.Sprintf(
		`<svg width="%.0f" height="%.0f" aria-label="price trend"><polyline points="%s" fill="none" stroke="currentColor" stroke-width="1.5"/></svg>`,
		width, height, strings.Join(pts, " ")))
}

type SiteRow struct {
	Site      string
	URL       string
	Price     float64
	HasDelta  bool
	Delta     float64 // positive = price went up since last pass
	Trend     template.HTML
	ScrapedAt string
	HitTarget bool
}

type ProductView struct {
	Name       string
	Target     float64
	AllTimeLow float64
	Rows       []SiteRow
}

type PageData struct {
	Products []ProductView
	Scraping bool
	LastPass string
	Interval string
}

// buildPageData assembles the dashboard model: latest price per configured
// product/site, delta vs previous pass, sparkline, all-time low.
func buildPageData(cfgPath string, store *Store, scraper *Scraper) (*PageData, error) {
	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		return nil, err
	}
	targets := map[string]float64{}
	order := map[string]int{}
	for i, p := range cfg.Products {
		targets[p.Name] = p.TargetPrice
		order[p.Name] = i
	}

	latest, err := store.Latest()
	if err != nil {
		return nil, err
	}

	views := make([]*ProductView, len(cfg.Products))
	for _, row := range latest {
		idx, ok := order[row.Product] // products dropped from config keep history but stay hidden
		if !ok {
			continue
		}
		if views[idx] == nil {
			views[idx] = &ProductView{Name: row.Product, Target: targets[row.Product], AllTimeLow: row.AllTimeLow}
		}
		trend, err := store.Trend(row.Product, row.Site, 60)
		if err != nil {
			return nil, err
		}
		sr := SiteRow{
			Site:      row.Site,
			URL:       row.URL,
			Price:     row.Price,
			Trend:     sparkline(trend),
			ScrapedAt: strings.Replace(row.ScrapedAt, "T", " ", 1),
			HitTarget: targets[row.Product] > 0 && row.Price <= targets[row.Product],
		}
		if len(trend) >= 2 && trend[len(trend)-2] != row.Price {
			sr.HasDelta = true
			sr.Delta = row.Price - trend[len(trend)-2]
		}
		views[idx].Rows = append(views[idx].Rows, sr)
	}

	data := &PageData{Scraping: scraper.Running(), Interval: cfg.Settings.Interval}
	if t := scraper.LastPass(); !t.IsZero() {
		data.LastPass = t.Format("15:04:05")
	}
	for _, v := range views {
		if v != nil {
			data.Products = append(data.Products, *v)
		}
	}
	return data, nil
}

type HistoryData struct {
	Product string
	Rows    []HistoryRow
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

func templateFuncs() template.FuncMap {
	return template.FuncMap{
		"inr":  inr,
		"inr0": inr0,
		"abs":  abs,
		"now":  func() string { return time.Now().Format("02 Jan 2006 15:04") },
	}
}
