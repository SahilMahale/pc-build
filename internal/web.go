package internal

import (
	"bytes"
	"fmt"
	"html/template"
	"log"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
)

type Server struct {
	app     *fiber.App
	tpl     *template.Template
	cfgPath string
	store   *Store
	scraper *Scraper
}

func NewServer(cfgPath string, store *Store, scraper *Scraper, exeDir string) (*Server, error) {
	tpl, err := template.New("").Funcs(templateFuncs()).ParseGlob(filepath.Join(exeDir, "templates", "*.html"))
	if err != nil {
		return nil, err
	}
	s := &Server{
		app:     fiber.New(fiber.Config{DisableStartupMessage: true}),
		tpl:     tpl,
		cfgPath: cfgPath,
		store:   store,
		scraper: scraper,
	}
	s.app.Static("/static", filepath.Join(exeDir, "static"))
	s.app.Get("/", s.index)
	s.app.Get("/prices", s.prices)
	s.app.Post("/scrape", s.scrapeNow)
	s.app.Get("/history/:product", s.history)
	return s, nil
}

func (s *Server) Listen(addr string) error {
	return s.app.Listen(addr)
}

func (s *Server) render(c *fiber.Ctx, name string, data any) error {
	var buf bytes.Buffer
	if err := s.tpl.ExecuteTemplate(&buf, name, data); err != nil {
		log.Printf("render %s: %v", name, err)
		return c.Status(500).SendString("template error")
	}
	c.Set("Content-Type", "text/html; charset=utf-8")
	return c.Send(buf.Bytes())
}

func (s *Server) index(c *fiber.Ctx) error {
	data, err := s.pageData()
	if err != nil {
		return c.Status(500).SendString(err.Error())
	}
	return s.render(c, "index.html", data)
}

func (s *Server) prices(c *fiber.Ctx) error {
	data, err := s.pageData()
	if err != nil {
		return c.Status(500).SendString(err.Error())
	}
	return s.render(c, "prices.html", data)
}

func (s *Server) scrapeNow(c *fiber.Ctx) error {
	go s.scraper.RunOnce()
	time.Sleep(50 * time.Millisecond) // let scraper set running flag
	if c.Get("HX-Request") == "" {
		return c.Redirect("/", 303)
	}
	data, err := s.pageData()
	if err != nil {
		return c.Status(500).SendString(err.Error())
	}
	return s.render(c, "prices.html", data)
}

func (s *Server) history(c *fiber.Ctx) error {
	product := c.Params("product")
	rows, err := s.store.History(product)
	if err != nil {
		return c.Status(500).SendString(err.Error())
	}
	return s.render(c, "history.html", HistoryData{Product: product, Rows: rows})
}

type SiteRow struct {
	Site        string
	URL         string
	Price       float64
	HasDelta    bool
	Delta       float64
	Trend       template.HTML
	ScrapedAt   string
	ScrapedAgo  string
	HitTarget   bool
}

type ProductView struct {
	Name       string
	Quantity   int
	Target     float64
	AllTimeLow float64
	Rows       []SiteRow
}

type PageData struct {
	Products    []ProductView
	Scraping    bool
	LastPass    string
	LastPassAgo string
	Interval    string
	ProgDone    int
	ProgTotal   int
	Total       float64
	TotalHave   int
	TotalAll    int
}

type HistoryData struct {
	Product string
	Rows    []HistoryRow
}

func (s *Server) pageData() (*PageData, error) {
	cfg, err := LoadConfig(s.cfgPath)
	if err != nil {
		return nil, err
	}
	order := map[string]int{}
	for i, p := range cfg.Products {
		order[p.Name] = i
	}

	latest, err := s.store.Latest()
	if err != nil {
		return nil, err
	}

	views := make([]*ProductView, len(cfg.Products))
	for _, row := range latest {
		idx, ok := order[row.Product]
		if !ok {
			continue
		}
		p := cfg.Products[idx]
		if views[idx] == nil {
			views[idx] = &ProductView{Name: p.Name, Quantity: p.Quantity, Target: p.TargetPrice, AllTimeLow: row.AllTimeLow}
		}
		trend, err := s.store.Trend(row.Product, row.Site, 60)
		if err != nil {
			return nil, err
		}
		scrapedTime, _ := time.Parse("2006-01-02T15:04:05", row.ScrapedAt)
		sr := SiteRow{
			Site:       row.Site,
			URL:        row.URL,
			Price:      row.Price,
			Trend:      sparkline(trend),
			ScrapedAt:  strings.Replace(row.ScrapedAt, "T", " ", 1),
			ScrapedAgo: formatAgo(time.Since(scrapedTime)),
			HitTarget:  p.TargetPrice > 0 && row.Price <= p.TargetPrice,
		}
		if len(trend) >= 2 && trend[len(trend)-2] != row.Price {
			sr.HasDelta = true
			sr.Delta = row.Price - trend[len(trend)-2]
		}
		views[idx].Rows = append(views[idx].Rows, sr)
	}

	data := &PageData{
		Scraping: s.scraper.Running(),
		Interval: cfg.Settings.Interval,
		TotalAll: len(cfg.Products),
	}
	if data.Scraping {
		data.ProgDone, data.ProgTotal = s.scraper.Progress()
	}
	if t := s.scraper.LastPass(); !t.IsZero() {
		data.LastPass = t.Format("15:04:05")
		data.LastPassAgo = formatAgo(time.Since(t))
	}
	for _, v := range views {
		if v == nil {
			continue
		}
		data.Products = append(data.Products, *v)
		data.TotalHave++
		data.Total += v.Rows[0].Price * float64(v.Quantity)
	}
	return data, nil
}

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

func inr0(v float64) string {
	return strings.TrimSuffix(inr(v), ".00")
}

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

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

func templateFuncs() template.FuncMap {
	return template.FuncMap{
		"inr":     inr,
		"inr0":    inr0,
		"abs":     abs,
		"now":     func() string { return time.Now().Format("02 Jan 2006 15:04") },
		"mul":     func(a, b float64) float64 { return a * b },
		"div":     func(a, b float64) float64 { return a / b },
		"float64": func(i int) float64 { return float64(i) },
	}
}

func formatAgo(d time.Duration) string {
	if d < time.Minute {
		return "just now"
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
	return fmt.Sprintf("%dd ago", int(d.Hours()/24))
}
