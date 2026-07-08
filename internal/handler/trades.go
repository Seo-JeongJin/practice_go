package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"
)

// Trades handles GET /{user_id}/trades, returning how many trades the user made.
func Trades(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := r.PathValue("user_id")

		var count int
		err := db.QueryRowContext(r.Context(),
			"SELECT COUNT(*) FROM trade_history WHERE user_id = ?", userID,
		).Scan(&count)
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]int{"count": count})
	}
}
