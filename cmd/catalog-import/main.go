// Command catalog-import stages, validates and activates a catalog DB snapshot.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/scmbr/smart-search/internal/application/catalogimport"
	"github.com/scmbr/smart-search/internal/catalog"
	domain "github.com/scmbr/smart-search/internal/domain/catalog"
	"github.com/scmbr/smart-search/internal/infrastructure/feed"
	"github.com/scmbr/smart-search/internal/infrastructure/postgres"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if err := godotenv.Load(); err != nil && !os.IsNotExist(err) {
		fmt.Fprintln(stderr, "cannot read .env")
		return 1
	}
	flags := flag.NewFlagSet("catalog-import", flag.ContinueOnError)
	flags.SetOutput(stderr)
	path := flags.String("file", os.Getenv("CATALOG_PATH"), "source XML/gzip file (or CATALOG_PATH)")
	batch := flags.Int("batch-size", 500, "records per transaction (1..5000)")
	migrateOnly := flags.Bool("migrate-only", false, "apply DB migrations and exit")
	if err := flags.Parse(args); err != nil {
		return 1
	}
	if flags.NArg() != 0 || *batch < 1 || *batch > 5000 || (!*migrateOnly && *path == "") {
		fmt.Fprintln(stderr, "provide -file and batch-size 1..5000")
		return 1
	}
	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		fmt.Fprintln(stderr, "POSTGRES_DSN is required")
		return 1
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		fmt.Fprintln(stderr, "invalid POSTGRES_DSN")
		return 1
	}
	cfg.MaxConns = 2
	cfg.MinConns = 0
	cfg.ConnConfig.ConnectTimeout = 5 * time.Second
	cfg.ConnConfig.RuntimeParams["statement_timeout"] = "30000"
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		fmt.Fprintln(stderr, "cannot create PostgreSQL pool")
		return 1
	}
	defer pool.Close()
	if err := postgres.Migrate(ctx, pool); err != nil {
		fmt.Fprintln(stderr, "PostgreSQL migration failed (check service and credentials)")
		return 1
	}
	if *migrateOnly {
		fmt.Fprintln(stdout, `{"migrations":"applied"}`)
		return 0
	}
	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = "data"
	}
	start := time.Now()
	source, err := feed.Open(ctx, *path, filepath.Join(dataDir, "import-cache"), catalog.Options{})
	if err != nil {
		fmt.Fprintf(stderr, "prepare source: %v\n", err)
		return 1
	}
	result, importErr := catalogimport.Run(ctx, source, postgres.New(pool), *batch)
	closeErr := source.Close()
	if err := json.NewEncoder(stdout).Encode(struct {
		Result    catalogimport.Result `json:"result"`
		ElapsedMS int64                `json:"elapsed_ms"`
	}{result, time.Since(start).Milliseconds()}); err != nil {
		fmt.Fprintln(stderr, "write report failed")
		return 1
	}
	if closeErr != nil {
		fmt.Fprintln(stderr, "cannot remove temporary feed copy")
		return 1
	}
	if errors.Is(importErr, domain.ErrInvalidCatalog) {
		fmt.Fprintln(stderr, "catalog validation failed; active snapshot unchanged")
		return 2
	}
	if importErr != nil {
		fmt.Fprintf(stderr, "import failed: %v\n", importErr)
		return 1
	}
	return 0
}
