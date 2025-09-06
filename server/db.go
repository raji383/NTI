package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
)

type PortfolioItem struct {
	ID       int     `json:"id,omitempty"`
	Title    string  `json:"title"`
	Category string  `json:"category"`
	Prix     float64 `json:"prix"`
	Img      string  `json:"img"`
}

func InitDB(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, err
	}

	create := `CREATE TABLE IF NOT EXISTS portfolio (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		title TEXT,
		category TEXT,
		prix REAL,
		img TEXT
	);`

	if _, err := db.Exec(create); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func tableCount(db *sql.DB, tbl string) (int, error) {
	var n int
	q := fmt.Sprintf("SELECT COUNT(*) FROM %s", tbl)
	if err := db.QueryRow(q).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

// LoadJSONToDB reads the JSON file and inserts records if DB table is empty.
func LoadJSONToDB(db *sql.DB, jsonPath string) error {
	cnt, err := tableCount(db, "portfolio")
	if err != nil {
		return err
	}
	if cnt > 0 {
		// already populated
		return nil
	}

	data, err := ioutil.ReadFile(jsonPath)
	if err != nil {
		return err
	}

	var items []PortfolioItem
	if err := json.Unmarshal(data, &items); err != nil {
		return err
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	// ensure prix column exists
	if _, err := tx.Exec("ALTER TABLE portfolio ADD COLUMN prix REAL"); err != nil {
		// ignore error (likely column already exists)
	}

	stmt, err := tx.Prepare("INSERT INTO portfolio(title, category, prix, img) VALUES (?, ?, ?, ?)")
	if err != nil {
		tx.Rollback()
		return err
	}
	defer stmt.Close()
	// items: attempt to read numeric prix (or price); default to 0
	for _, raw := range items {
		// raw currently unmarshalled into PortfolioItem (without Prix), so re-marshal to generic map
		var m map[string]json.RawMessage
		b, _ := json.Marshal(raw)
		json.Unmarshal(b, &m)

		var prixVal float64 = 0
		// check for numeric `prix`
		if v, ok := m["prix"]; ok {
			// try number
			_ = json.Unmarshal(v, &prixVal)
		} else if v, ok := m["price"]; ok {
			_ = json.Unmarshal(v, &prixVal)
		} else if v, ok := m["prx"]; ok {
			// older prx might be array or number
			var maybeNum float64
			if err := json.Unmarshal(v, &maybeNum); err == nil {
				prixVal = maybeNum
			}
		}

		if _, err := stmt.Exec(raw.Title, raw.Category, prixVal, raw.Img); err != nil {
			tx.Rollback()
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	log.Printf("migrated %d portfolio items from %s", len(items), filepath.Base(jsonPath))
	return nil
}

func GetAllPortfolio(db *sql.DB) ([]PortfolioItem, error) {
	// ensure prix column exists
	if _, err := db.Exec("ALTER TABLE portfolio ADD COLUMN prix REAL"); err != nil {
		// ignore
	}

	rows, err := db.Query("SELECT id, title, category, prix, img FROM portfolio ORDER BY id ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PortfolioItem
	for rows.Next() {
		var it PortfolioItem
		var prixNull sql.NullFloat64
		if err := rows.Scan(&it.ID, &it.Title, &it.Category, &prixNull, &it.Img); err != nil {
			return nil, err
		}
		if prixNull.Valid {
			it.Prix = prixNull.Float64
		} else {
			it.Prix = 0
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// InsertPortfolio inserts a new portfolio item and updates the item's ID.
func InsertPortfolio(db *sql.DB, it *PortfolioItem) error {
	res, err := db.Exec("INSERT INTO portfolio(title, category, prix, img) VALUES (?, ?, ?, ?)", it.Title, it.Category, it.Prix, it.Img)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	it.ID = int(id)
	return nil
}
