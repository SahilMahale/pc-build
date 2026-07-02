package main

import (
	"flag"
	"log"
	"os"
	"path/filepath"

	"github.com/sahilm/pc-build/internal"
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

	cfg, err := internal.LoadConfig(*cfgPath)
	if err != nil {
		log.Fatal(err)
	}

	dbPath := cfg.Settings.Database
	if !filepath.IsAbs(dbPath) {
		dbPath = filepath.Join(filepath.Dir(*cfgPath), dbPath)
	}
	store, err := internal.OpenStore(dbPath)
	if err != nil {
		log.Fatal(err)
	}

	scraper := internal.NewScraper(*cfgPath, store)
	go scraper.Loop()

	exeDir := "."
	if exe, err := os.Executable(); err == nil {
		if _, err := os.Stat(filepath.Join(filepath.Dir(exe), "templates")); err == nil {
			exeDir = filepath.Dir(exe)
		}
	}

	srv, err := internal.NewServer(*cfgPath, store, scraper, exeDir)
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("serving on %s (config: %s, db: %s)", cfg.Settings.Listen, *cfgPath, dbPath)
	log.Fatal(srv.Listen(cfg.Settings.Listen))
}
