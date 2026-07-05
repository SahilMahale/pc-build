package internal

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
)

type Source struct {
	Site      string `yaml:"site"`
	URL       string `yaml:"url"`
	Selector  string `yaml:"selector"`
	Attribute string `yaml:"attribute"`
}

type Product struct {
	Name        string   `yaml:"name"`
	TargetPrice float64  `yaml:"target_price"`
	Quantity    int      `yaml:"quantity"`
	Sources     []Source `yaml:"sources"`
}

type Settings struct {
	Interval             string    `yaml:"interval"`
	DelayBetweenRequests []float64 `yaml:"delay_between_requests"`
	Timeout              int       `yaml:"timeout"`
	Database             string    `yaml:"database"`
	UserAgents           []string  `yaml:"user_agents"`
	AlertWebhook         string    `yaml:"alert_webhook"`
	Listen               string    `yaml:"listen"`
	ScrapeCooldown       string    `yaml:"scrape_cooldown"`
}

type Config struct {
	Settings Settings  `yaml:"settings"`
	Products []Product `yaml:"products"`
}

func LoadConfig(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if cfg.Settings.Timeout == 0 {
		cfg.Settings.Timeout = 20
	}
	if len(cfg.Settings.DelayBetweenRequests) != 2 {
		cfg.Settings.DelayBetweenRequests = []float64{3, 8}
	}
	if cfg.Settings.Database == "" {
		cfg.Settings.Database = "prices.db"
	}
	if cfg.Settings.Interval == "" {
		cfg.Settings.Interval = "6h"
	}
	if cfg.Settings.ScrapeCooldown == "" {
		cfg.Settings.ScrapeCooldown = "10m"
	}
	if cfg.Settings.Listen == "" {
		host := os.Getenv("HOST")
		if host == "" {
			host = "0.0.0.0"
		}
		port := os.Getenv("PORT")
		if port == "" {
			port = "8090"
		}
		cfg.Settings.Listen = host + ":" + port
	}
	for i := range cfg.Products {
		if cfg.Products[i].Quantity == 0 {
			cfg.Products[i].Quantity = 1
		}
	}
	return &cfg, nil
}

var intervalRe = regexp.MustCompile(`^(\d+)([smhd]?)$`)

// ParseInterval: "30m" → 30min, "6h" → 6h, "1d" → 24h, "3600" → 3600s.
func ParseInterval(raw string) (time.Duration, error) {
	m := intervalRe.FindStringSubmatch(raw)
	if m == nil {
		return 0, fmt.Errorf("bad interval %q (use e.g. \"30m\", \"6h\", \"1d\")", raw)
	}
	n, _ := strconv.Atoi(m[1])
	unit := map[string]time.Duration{
		"": time.Second, "s": time.Second, "m": time.Minute,
		"h": time.Hour, "d": 24 * time.Hour,
	}[m[2]]
	return time.Duration(n) * unit, nil
}
