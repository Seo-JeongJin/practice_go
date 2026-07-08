package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"time"
)

// purchaseAmountQuery sums, per trade up to asOfDate, floor(price on trade_date * quantity / 10000).
const purchaseAmountQuery = `
SELECT COALESCE(SUM(FLOOR(rp.reference_price * t.quantity / 10000)), 0)
FROM trade_history t
JOIN reference_prices rp
  ON rp.fund_id = t.fund_id AND rp.reference_price_date = t.trade_date
WHERE t.user_id = ? AND t.trade_date <= ?`

// currentValueQuery sums, per fund the user holds as of asOfDate, floor(price as
// of asOfDate * held quantity / 10000).
const currentValueQuery = `
SELECT COALESCE(SUM(FLOOR(latest.reference_price * held.total_qty / 10000)), 0)
FROM (
	SELECT fund_id, SUM(quantity) AS total_qty
	FROM trade_history
	WHERE user_id = ? AND trade_date <= ?
	GROUP BY fund_id
) held
JOIN (
	SELECT fund_id, reference_price
	FROM (
		SELECT fund_id, reference_price,
		       ROW_NUMBER() OVER (PARTITION BY fund_id ORDER BY reference_price_date DESC) AS rn
		FROM reference_prices
		WHERE reference_price_date <= ?
		  AND fund_id IN (SELECT DISTINCT fund_id FROM trade_history WHERE user_id = ?)
	) ranked
	WHERE rn = 1
) latest ON latest.fund_id = held.fund_id`

type assetsResponse struct {
	Date         string `json:"date"`
	CurrentValue int64  `json:"current_value"`
	CurrentPL    int64  `json:"current_pl"`
}

const dateLayout = "2006-01-02"

// Assets handles GET /{user_id}/assets[?date=YYYY-MM-DD], returning the
// user's asset valuation and unrealized profit/loss as of the given date
// (today, if the date query parameter is omitted).
func Assets(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := r.PathValue("user_id")
		ctx := r.Context()

		asOf := time.Now()
		if dateParam := r.URL.Query().Get("date"); dateParam != "" {
			parsed, err := time.Parse(dateLayout, dateParam)
			if err != nil {
				http.Error(w, "invalid date, expected YYYY-MM-DD", http.StatusBadRequest)
				return
			}
			asOf = parsed
		}
		asOfDate := asOf.Format(dateLayout)

		var purchaseAmount int64
		if err := db.QueryRowContext(ctx, purchaseAmountQuery, userID, asOfDate).Scan(&purchaseAmount); err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		var currentValue int64
		if err := db.QueryRowContext(ctx, currentValueQuery, userID, asOfDate, asOfDate, userID).Scan(&currentValue); err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		resp := assetsResponse{
			Date:         asOfDate,
			CurrentValue: currentValue,
			CurrentPL:    currentValue - purchaseAmount,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}
