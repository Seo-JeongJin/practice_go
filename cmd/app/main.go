// Command app is the single entry point for the trust-trading assignment.
// It dispatches on a subcommand, e.g. `app import`.
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"finatext_internship/internal/config"
	"finatext_internship/internal/db"
	"finatext_internship/internal/importer"
	"finatext_internship/internal/server"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: app <import|serve>")
		os.Exit(1)
	}

	config.LoadDotEnv(".env")

	switch os.Args[1] {
	case "import":
		if err := runImport(); err != nil {
			fmt.Fprintln(os.Stderr, "import failed:", err)
			os.Exit(1)
		}
	case "serve":
		if err := runServe(); err != nil {
			fmt.Fprintln(os.Stderr, "serve failed:", err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", os.Args[1])
		os.Exit(1)
	}
}

func runImport() error {
	conn, err := db.Open()
	if err != nil {
		return err
	}
	defer conn.Close()

	ctx := context.Background()

	n, err := importer.ImportReferencePrices(ctx, conn, "data/reference_prices.csv")
	if err != nil {
		return fmt.Errorf("reference_prices: %w", err)
	}
	fmt.Printf("reference_prices: imported %d rows\n", n)

	n, err = importer.ImportTradeHistory(ctx, conn, "data/trade_history.csv")
	if err != nil {
		return fmt.Errorf("trade_history: %w", err)
	}
	fmt.Printf("trade_history: imported %d rows\n", n)

	return nil
}

func runServe() error {
	conn, err := db.Open()
	if err != nil {
		return err
	}
	defer conn.Close()

	srv := server.New(conn)
	fmt.Println("listening on :8080")
	return http.ListenAndServe(":8080", srv)
}
