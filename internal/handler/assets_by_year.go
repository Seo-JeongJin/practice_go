package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"sort"
	"time"
)

// purchaseAmountByYearQuery sums, per purchase year, floor(price on trade_date * quantity / 10000).
const purchaseAmountByYearQuery = `
SELECT YEAR(t.trade_date) AS yr, COALESCE(SUM(FLOOR(rp.reference_price * t.quantity / 10000)), 0) AS purchase_amount
FROM trade_history t
JOIN reference_prices rp
  ON rp.fund_id = t.fund_id AND rp.reference_price_date = t.trade_date
WHERE t.user_id = ?
GROUP BY YEAR(t.trade_date)`

// currentValueByYearQuery sums, per purchase year, floor(latest price * quantity bought that year / 10000).
const currentValueByYearQuery = `
SELECT held.yr, COALESCE(SUM(FLOOR(latest.reference_price * held.total_qty / 10000)), 0) AS current_value
FROM (
	SELECT YEAR(trade_date) AS yr, fund_id, SUM(quantity) AS total_qty
	FROM trade_history
	WHERE user_id = ?
	GROUP BY YEAR(trade_date), fund_id
) held
JOIN (
	SELECT fund_id, reference_price
	FROM (
		SELECT fund_id, reference_price,
		       ROW_NUMBER() OVER (PARTITION BY fund_id ORDER BY reference_price_date DESC) AS rn
		FROM reference_prices
		WHERE fund_id IN (SELECT DISTINCT fund_id FROM trade_history WHERE user_id = ?)
	) ranked
	WHERE rn = 1
) latest ON latest.fund_id = held.fund_id
GROUP BY held.yr`

type assetsByYearItem struct {
	Year         int   `json:"year"`
	CurrentValue int64 `json:"current_value"`
	CurrentPL    int64 `json:"current_pl"`
}

type assetsByYearResponse struct {
	Date   string             `json:"date"`
	Assets []assetsByYearItem `json:"assets"`
}

// AssetsByYear handles GET /{user_id}/assets/byYear, returning the current
// asset valuation and unrealized profit/loss grouped by purchase year,
// ordered by year descending.
func AssetsByYear(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := r.PathValue("user_id")
		ctx := r.Context()

		purchaseByYear, err := queryByYear(ctx, db, purchaseAmountByYearQuery, userID)
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		valueByYear, err := queryByYear(ctx, db, currentValueByYearQuery, userID, userID)
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		years := make(map[int]struct{}, len(valueByYear))
		for yr := range purchaseByYear {
			years[yr] = struct{}{}
		}
		for yr := range valueByYear {
			years[yr] = struct{}{}
		}

		assets := make([]assetsByYearItem, 0, len(years))
		for yr := range years {
			assets = append(assets, assetsByYearItem{
				Year:         yr,
				CurrentValue: valueByYear[yr],
				CurrentPL:    valueByYear[yr] - purchaseByYear[yr],
			})
		}
		sort.Slice(assets, func(i, j int) bool { return assets[i].Year > assets[j].Year })

		resp := assetsByYearResponse{
			Date:   time.Now().Format(dateLayout),
			Assets: assets,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}

// queryByYear runs a "YEAR(...), amount" query and collects the results into
// a map keyed by year.
func queryByYear(ctx context.Context, db *sql.DB, query string, args ...any) (map[int]int64, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := map[int]int64{}
	for rows.Next() {
		var yr int
		var amount int64
		if err := rows.Scan(&yr, &amount); err != nil {
			return nil, err
		}
		result[yr] = amount
	}
	return result, rows.Err()
}
