package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
)

func main() {
	// Resolve repo root and data paths
	repoRoot, err := filepath.Abs("../")
	if err != nil {
		log.Fatalf("failed to resolve directory: %v", err)
	}

	// Initialize DB
	dbPath := filepath.Join(repoRoot, "server.db")
	db, err := InitDB(dbPath)
	if err != nil {
		log.Fatalf("db init: %v", err)
	}
	defer db.Close()

	// Migrate JSON into DB if empty
	jsonPath := filepath.Join(repoRoot, "assets", "data", "portfolio.json")
	if err := LoadJSONToDB(db, jsonPath); err != nil {
		log.Printf("warning: failed to load json to db: %v", err)
	}

	// API endpoint to list portfolio items
	http.HandleFunc("/api/portfolio", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			items, err := GetAllPortfolio(db)
			if err != nil {
				http.Error(w, "failed to query portfolio", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(items)
		case http.MethodPost:
			// accept JSON body to create a new portfolio item
			if err := r.ParseMultipartForm(10 << 20); err != nil { // 10MB limit
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}

			title := r.FormValue("title")
			category := r.FormValue("category")
			prixStr := r.FormValue("prix")

			var prix float64
			fmt.Sscanf(prixStr, "%f", &prix)

			file, handler, err := r.FormFile("img")
			if err != nil {
				http.Error(w, "Error retrieving the file", http.StatusBadRequest)
				return
			}
			defer file.Close()

			dst, err := os.Create("../assets/img/portfolio/" + handler.Filename)
			if err != nil {
				http.Error(w, "Error saving file", http.StatusInternalServerError)
				return
			}
			defer dst.Close()
			io.Copy(dst, file)

			// رجع JSON
			item := PortfolioItem{
				ID:       1,
				Title:    title,
				Category: category,
				Prix:     prix,
				Img:      "/assets/img/portfolio/" + handler.Filename, 
			}
			if item.Title == "" || item.Img == "" {
				http.Error(w, "missing title or img", http.StatusBadRequest)
				return
			}
			if err := InsertPortfolio(db, &item); err != nil {
				http.Error(w, "failed to insert item", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(item)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// Static file server for everything else
	fs := http.FileServer(http.Dir(repoRoot))
	http.Handle("/", fs)

	addr := ":8080"
	fmt.Printf("Serving %s on http://localhost%s\n", repoRoot, addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
