package internal

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
)

var priceRe = regexp.MustCompile(`[\d,]+(?:\.\d{1,2})?`)

// extractPrice pulls the first numeric price out of a string like
// "₹21,849.00 incl. GST".
func extractPrice(text string) (float64, bool) {
	text = strings.NewReplacer("₹", "", "Rs.", "").Replace(text)
	m := priceRe.FindString(text)
	if m == "" {
		return 0, false
	}
	v, err := strconv.ParseFloat(strings.ReplaceAll(m, ",", ""), 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

var retryStatus = map[int]bool{408: true, 425: true, 429: true, 500: true, 502: true, 503: true, 504: true}

// getWithRetry fetches a URL with up to 3 retries on connection errors and
// 408/425/429/5xx, using exponential backoff + jitter and honouring Retry-After.
func getWithRetry(client *http.Client, url, userAgent string) (*http.Response, error) {
	var lastErr error
	for attempt := 0; attempt <= 3; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(1<<attempt)*time.Second + time.Duration(rand.Float64()*float64(time.Second))
			time.Sleep(backoff)
		}
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", userAgent)
		req.Header.Set("Accept-Language", "en-IN,en;q=0.9")

		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		if retryStatus[resp.StatusCode] {
			if ra := resp.Header.Get("Retry-After"); ra != "" {
				if secs, err := strconv.Atoi(ra); err == nil && secs <= 120 {
					time.Sleep(time.Duration(secs) * time.Second)
				}
			}
			resp.Body.Close()
			lastErr = fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
			continue
		}
		if resp.StatusCode >= 400 {
			resp.Body.Close()
			return nil, fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
		}
		return resp, nil
	}
	return nil, lastErr
}

// fetchPrice loads a source page and extracts its price via CSS selector
// (optionally reading an attribute instead of text).
func fetchPrice(client *http.Client, src Source, settings Settings) (float64, error) {
	ua := "go-price-scraper/1.0"
	if len(settings.UserAgents) > 0 {
		ua = settings.UserAgents[rand.Intn(len(settings.UserAgents))]
	}
	resp, err := getWithRetry(client, src.URL, ua)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return 0, err
	}
	el := doc.Find(src.Selector).First()
	if el.Length() == 0 {
		return 0, fmt.Errorf("selector matched nothing — page layout may have changed")
	}
	raw := ""
	if src.Attribute != "" {
		raw = el.AttrOr(src.Attribute, "")
	} else {
		raw = el.Text()
	}
	price, ok := extractPrice(raw)
	if !ok {
		return 0, fmt.Errorf("no price in %q", strings.TrimSpace(raw))
	}
	return price, nil
}

func sendAlert(webhook, message string) {
	if webhook == "" {
		return
	}
	payload, _ := json.Marshal(map[string]string{
		"content": message, "text": message, "message": message,
	})
	resp, err := http.Post(webhook, "application/json", bytes.NewReader(payload))
	if err != nil {
		log.Printf("    ! webhook failed: %v", err) // alert failure never kills the scraper
		return
	}
	resp.Body.Close()
}

// Scraper owns the periodic loop and serializes passes.
type Scraper struct {
	cfgPath string
	store   *Store

	mu        sync.Mutex
	running   bool
	lastPass  time.Time
	progDone  int
	progTotal int
}

func NewScraper(cfgPath string, store *Store) *Scraper {
	return &Scraper{cfgPath: cfgPath, store: store}
}

func (s *Scraper) Running() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

func (s *Scraper) LastPass() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastPass
}

func (s *Scraper) Progress() (done, total int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.progDone, s.progTotal
}

// TryRunOnce starts a pass in the background unless one is already running
// (treated as accepted — the caller's intent is satisfied) or the previous
// pass ended less than cooldown ago. Scheduled passes use RunOnce directly
// and are never subject to the cooldown.
func (s *Scraper) TryRunOnce(cooldown time.Duration) (started bool, wait time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		return true, 0
	}
	if !s.lastPass.IsZero() {
		if w := cooldown - time.Since(s.lastPass); w > 0 {
			return false, w
		}
	}
	go s.RunOnce()
	return true, 0
}

// RunOnce executes a single scrape pass; concurrent calls are dropped.
func (s *Scraper) RunOnce() {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.running = false
		s.lastPass = time.Now()
		s.mu.Unlock()
	}()

	cfg, err := LoadConfig(s.cfgPath)
	if err != nil {
		log.Printf("pass failed: %v", err)
		return
	}
	settings := cfg.Settings
	client := &http.Client{Timeout: time.Duration(settings.Timeout) * time.Second}
	lo, hi := settings.DelayBetweenRequests[0], settings.DelayBetweenRequests[1]

	total := 0
	for _, p := range cfg.Products {
		total += len(p.Sources)
	}
	s.mu.Lock()
	s.progDone, s.progTotal = 0, total
	s.mu.Unlock()

	for _, product := range cfg.Products {
		log.Printf("▸ %s", product.Name)
		for _, src := range product.Sources {
			price, err := fetchPrice(client, src, settings)
			s.mu.Lock()
			s.progDone++
			s.mu.Unlock()
			if err != nil {
				log.Printf("    %-14s ERROR: %v", src.Site, err)
				continue
			}
			if err := s.store.Save(product.Name, src.Site, price, src.URL); err != nil {
				log.Printf("    %-14s db error: %v", src.Site, err)
				continue
			}
			flag := ""
			if product.TargetPrice > 0 && price <= product.TargetPrice {
				flag = fmt.Sprintf("  🔔 at/below target ₹%s!", inr(product.TargetPrice))
				sendAlert(settings.AlertWebhook, fmt.Sprintf(
					"%s: ₹%s at %s (target ₹%s) %s",
					product.Name, inr(price), src.Site, inr(product.TargetPrice), src.URL))
			}
			log.Printf("    %-14s ₹%s%s", src.Site, inr(price), flag)

			time.Sleep(time.Duration((lo + rand.Float64()*(hi-lo)) * float64(time.Second)))
		}
	}
}

// Loop scrapes immediately on start, then repeats on the configured
// interval (re-read each pass so config edits apply without restart).
func (s *Scraper) Loop() {
	for {
		s.RunOnce()
		interval := 6 * time.Hour
		if cfg, err := LoadConfig(s.cfgPath); err == nil {
			if d, err := ParseInterval(cfg.Settings.Interval); err == nil {
				interval = d
			}
		}
		log.Printf("next pass in %s", interval)
		time.Sleep(interval)
	}
}
