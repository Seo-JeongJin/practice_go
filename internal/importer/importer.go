// Package importer loads the reference_prices and trade_history CSV files
// into their corresponding MySQL tables.
package importer

import (
	"context"
	"database/sql"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	dateLayout = "2006-01-02"
	batchSize  = 1000
)

const createReferencePricesTable = `
CREATE TABLE IF NOT EXISTS reference_prices (
	fund_id               VARCHAR(6) NOT NULL,
	reference_price       INT NOT NULL,
	reference_price_date  DATE NOT NULL,
	INDEX idx_fund_date (fund_id, reference_price_date)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`

const createTradeHistoryTable = `
CREATE TABLE IF NOT EXISTS trade_history (
	user_id     VARCHAR(10) NOT NULL,
	fund_id     VARCHAR(6) NOT NULL,
	quantity    INT NOT NULL,
	trade_date  DATE NOT NULL,
	INDEX idx_user (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`

// ImportReferencePrices creates reference_prices if needed, truncates it and
// loads it from path.
func ImportReferencePrices(ctx context.Context, db *sql.DB, path string) (int, error) {
	columns := []string{"fund_id", "reference_price", "reference_price_date"}
	parse := func(row []string, lineNo int) ([]any, error) {
		price, err := strconv.Atoi(row[1])
		if err != nil {
			return nil, fmt.Errorf("line %d: invalid reference_price %q: %w", lineNo, row[1], err)
		}
		date, err := time.Parse(dateLayout, row[2])
		if err != nil {
			return nil, fmt.Errorf("line %d: invalid reference_price_date %q: %w", lineNo, row[2], err)
		}
		return []any{row[0], price, date}, nil
	}
	return runImport(ctx, db, path, "reference_prices", createReferencePricesTable, columns, parse)
}

// ImportTradeHistory creates trade_history if needed, truncates it and loads
// it from path.
func ImportTradeHistory(ctx context.Context, db *sql.DB, path string) (int, error) {
	columns := []string{"user_id", "fund_id", "quantity", "trade_date"}
	parse := func(row []string, lineNo int) ([]any, error) {
		quantity, err := strconv.Atoi(row[2])
		if err != nil {
			return nil, fmt.Errorf("line %d: invalid quantity %q: %w", lineNo, row[2], err)
		}
		date, err := time.Parse(dateLayout, row[3])
		if err != nil {
			return nil, fmt.Errorf("line %d: invalid trade_date %q: %w", lineNo, row[3], err)
		}
		return []any{row[0], row[1], quantity, date}, nil
	}
	return runImport(ctx, db, path, "trade_history", createTradeHistoryTable, columns, parse)
}

type rowParser func(row []string, lineNo int) ([]any, error)

// runImport ensures table exists, truncates it and bulk-inserts every row of
// the CSV at path, all inside a single transaction so a bad row leaves the
// table untouched.
func runImport(ctx context.Context, db *sql.DB, path, table, createTableSQL string, columns []string, parse rowParser) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.FieldsPerRecord = len(columns)

	header, err := r.Read()
	if err != nil {
		return 0, fmt.Errorf("read header of %s: %w", path, err)
	}
	if strings.Join(header, ",") != strings.Join(columns, ",") {
		return 0, fmt.Errorf("%s: unexpected header %v, want %v", path, header, columns)
	}

	if _, err := db.ExecContext(ctx, createTableSQL); err != nil {
		return 0, fmt.Errorf("create table %s: %w", table, err)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, "TRUNCATE TABLE "+table); err != nil {
		return 0, fmt.Errorf("truncate %s: %w", table, err)
	}

	rowPlaceholder := "(" + strings.TrimSuffix(strings.Repeat("?,", len(columns)), ",") + ")"
	insertPrefix := fmt.Sprintf("INSERT INTO %s (%s) VALUES ", table, strings.Join(columns, ","))

	placeholders := make([]string, 0, batchSize)
	args := make([]any, 0, batchSize*len(columns))
	count := 0
	lineNo := 1 // header consumed line 1

	flush := func() error {
		if len(placeholders) == 0 {
			return nil
		}
		query := insertPrefix + strings.Join(placeholders, ",")
		if _, err := tx.ExecContext(ctx, query, args...); err != nil {
			return fmt.Errorf("insert into %s: %w", table, err)
		}
		placeholders = placeholders[:0]
		args = args[:0]
		return nil
	}

	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		lineNo++
		if err != nil {
			return count, fmt.Errorf("read %s at line %d: %w", path, lineNo, err)
		}

		values, err := parse(record, lineNo)
		if err != nil {
			return count, err
		}

		placeholders = append(placeholders, rowPlaceholder)
		args = append(args, values...)
		count++

		if len(placeholders) >= batchSize {
			if err := flush(); err != nil {
				return count, err
			}
		}
	}
	if err := flush(); err != nil {
		return count, err
	}
	if err := tx.Commit(); err != nil {
		return count, fmt.Errorf("commit %s: %w", table, err)
	}
	return count, nil
}
