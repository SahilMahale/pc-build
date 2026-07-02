package main

import (
	"flag"
	"log"
	"net/url"
	"os"
	"path/filepath"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/template/html/v2"
)

func main() {
	cfgPath := flag.String("config", "", "path to config.yaml (default: ./config.yaml, then ../config.yaml)")
	flag.Parse()

	if *cfgPath == "" {
		for _, cand := range []string{"config.yaml", "../config.yaml"} {
			if _, err := os.Stat(cand); err == nil {
				*cfgPath = cand
				break
			}
		}
	}
	if *cfgPath == "" {
		log.Fatal("no config.yaml found (looked in . and ..) — pass -config")
	}

	cfg, err := LoadConfig(*cfgPath)
	if err != nil {
		log.Fatal(err)
	}

	// DB lives next to the config file (shared with the Python v1 scraper).
	dbPath := cfg.Settings.Database
	if !filepath.IsAbs(dbPath) {
		dbPath = filepath.Join(filepath.Dir(*cfgPath), dbPath)
	}
	store, err := OpenStore(dbPath)
	if err != nil {
		log.Fatal(err)
	}

	scraper := NewScraper(*cfgPath, store)
	go scraper.Loop() // first pass starts immediately

	exeDir := "."
	if exe, err := os.Executable(); err == nil {
		if _, err := os.Stat(filepath.Join(filepath.Dir(exe), "templates")); err == nil {
			exeDir = filepath.Dir(exe)
		}
	}
	engine := html.New(filepath.Join(exeDir, "templates"), ".html")
	engine.AddFuncMap(templateFuncs())

	app := fiber.New(fiber.Config{Views: engine, DisableStartupMessage: true})
	app.Static("/static", filepath.Join(exeDir, "static"))

	app.Get("/", func(c *fiber.Ctx) error {
		data, err := buildPageData(*cfgPath, store, scraper)
		if err != nil {
			return err
		}
		return c.Render("index", data)
	})

	// HTMX fragment: the prices panel (auto-refresh + post-scrape swap target)
	app.Get("/prices", func(c *fiber.Ctx) error {
		data, err := buildPageData(*cfgPath, store, scraper)
		if err != nil {
			return err
		}
		return c.Render("prices", data)
	})

	// HTMX action: kick off a pass now (no-op if one is already running)
	app.Post("/scrape", func(c *fiber.Ctx) error {
		go scraper.RunOnce()
		data, err := buildPageData(*cfgPath, store, scraper)
		if err != nil {
			return err
		}
		data.Scraping = true
		return c.Render("prices", data)
	})

	app.Get("/history/:product", func(c *fiber.Ctx) error {
		product, err := url.PathUnescape(c.Params("product"))
		if err != nil {
			product = c.Params("product")
		}
		rows, err := store.History(product)
		if err != nil {
			return err
		}
		return c.Render("history", HistoryData{Product: product, Rows: rows})
	})

	log.Printf("serving on %s (config: %s, db: %s)", cfg.Settings.Listen, *cfgPath, dbPath)
	log.Fatal(app.Listen(cfg.Settings.Listen))
}
