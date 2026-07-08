// Package server wires up the HTTP routes for the trust-trading API.
package server

import (
	"database/sql"
	"net/http"

	"finatext_internship/internal/handler"
)

// New builds the HTTP handler for the application, routing each endpoint to
// its handler.
func New(db *sql.DB) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{user_id}/trades", handler.Trades(db))
	mux.HandleFunc("GET /{user_id}/assets", handler.Assets(db))
	mux.HandleFunc("GET /{user_id}/assets/byYear", handler.AssetsByYear(db))
	return mux
}
