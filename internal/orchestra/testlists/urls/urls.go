// Package urls queries orchestra test-lists/urls API
package urls

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	_ "github.com/lib/pq"

	"github.com/ooni/probe-engine/model"
)

// Config contains configs for querying tests-lists/urls
type Config struct {
	BaseURL           string
	CountryCode       string
	EnabledCategories []string
	HTTPClient        *http.Client
	Limit             int64
	Logger            model.Logger
	UserAgent         string
}

// Result contains the result returned by tests-lists/urls
type Result struct {
	Results []model.URLInfo `json:"results"`
}

// Query retrieves the test list for the specified country.
func Query(ctx context.Context, config Config) (*Result, error) {
	query := url.Values{}
	if config.CountryCode != "" {
		query.Set("probe_cc", config.CountryCode)
	}
	if config.Limit > 0 {
		query.Set("limit", fmt.Sprintf("%d", config.Limit))
	}
	if len(config.EnabledCategories) > 0 {
		query.Set("category_codes", strings.Join(config.EnabledCategories, ","))
	}
	pages, err := githubPages()
	if err != nil {
		return nil, err
	}
	return pages, nil
}

func githubPages() (*Result, error) {
	var results []model.URLInfo
	db, err := sql.Open("postgres", "postgres://postgres:postgres@db/cendet?sslmode=disable")
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.Query("SELECT 'GITHUB' as CategoryCode, 'XX' as CountryCode, LOWER(r.name) as URL FROM repository r GROUP BY r.name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		row := model.URLInfo{}
		err := rows.Scan(&row)
		if err != nil {
			// ignore
		}
		results = append(results, row)
	}
	response := Result{
		Results: results,
	}
	fmt.Printf("   • Queried %d results from database\n", len(response.Results))
	return &response, nil
}
