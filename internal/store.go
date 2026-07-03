package internal

import (
	"database/sql"
	"time"

	_ "modernc.org/sqlite"
)

// Same schema as the Python v1 scraper — prices.db carries over unchanged.
const schema = `
CREATE TABLE IF NOT EXISTS prices (
    id        INTEGER PRIMARY KEY AUTOINCREMENT,
    product   TEXT NOT NULL,
    site      TEXT NOT NULL,
    price     REAL NOT NULL,
    url       TEXT NOT NULL,
    scraped_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_product_time ON prices(product, scraped_at);
`

type Store struct{ db *sql.DB }

func OpenStore(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // serialize writers; plenty for this workload
	if _, err := db.Exec(schema); err != nil {
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) Save(product, site string, price float64, url string) error {
	_, err := s.db.Exec(
		"INSERT INTO prices (product, site, price, url, scraped_at) VALUES (?,?,?,?,?)",
		product, site, price, url, time.Now().Format("2006-01-02T15:04:05"),
	)
	return err
}

type LatestRow struct {
	Product    string
	Site       string
	Price      float64
	URL        string
	ScrapedAt  string
	AllTimeLow float64
}

// Latest returns the most recent price per (product, site) plus the
// all-time low per product, cheapest site first.
func (s *Store) Latest() ([]LatestRow, error) {
	rows, err := s.db.Query(`
		SELECT product, site, price, url, scraped_at,
		       MIN(price) OVER (PARTITION BY product) AS all_time_low
		FROM prices p1
		WHERE scraped_at = (
		    SELECT MAX(scraped_at) FROM prices p2
		    WHERE p2.product = p1.product AND p2.site = p1.site
		)
		ORDER BY product, price`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LatestRow
	for rows.Next() {
		var r LatestRow
		if err := rows.Scan(&r.Product, &r.Site, &r.Price, &r.URL, &r.ScrapedAt, &r.AllTimeLow); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// Trend returns up to limit most recent prices for one product+site,
// oldest first (for sparklines / delta-vs-last-pass).
func (s *Store) Trend(product, site string, limit int) ([]float64, error) {
	rows, err := s.db.Query(`
		SELECT price FROM (
		    SELECT price, scraped_at FROM prices
		    WHERE product = ? AND site = ?
		    ORDER BY scraped_at DESC LIMIT ?
		) ORDER BY scraped_at`, product, site, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []float64
	for rows.Next() {
		var p float64
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

type HistoryRow struct {
	ScrapedAt string
	Site      string
	Price     float64
}

func (s *Store) History(product string) ([]HistoryRow, error) {
	rows, err := s.db.Query(
		"SELECT scraped_at, site, price FROM prices WHERE product = ? ORDER BY scraped_at",
		product)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []HistoryRow
	for rows.Next() {
		var r HistoryRow
		if err := rows.Scan(&r.ScrapedAt, &r.Site, &r.Price); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
