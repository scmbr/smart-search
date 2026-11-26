package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	app "github.com/scmbr/smart-search/internal/application/lexical"
	"github.com/scmbr/smart-search/internal/infrastructure/elastic"
	"github.com/scmbr/smart-search/internal/infrastructure/families"
	"github.com/scmbr/smart-search/internal/infrastructure/localconfig"
	"github.com/scmbr/smart-search/internal/infrastructure/postgres"
	"os"
	"os/signal"
	"time"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := run(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run(ctx context.Context) error {
	batch := flag.Int("batch-size", 500, "offers per index batch (1..1000)")
	flag.Parse()
	if flag.NArg() != 0 || *batch < 1 || *batch > 1000 {
		return fmt.Errorf("invalid batch-size or positional arguments")
	}
	s, err := localconfig.Load()
	if err != nil {
		return err
	}
	pool, err := s.Pool(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()
	f, mappingVersion, err := families.Load()
	if err != nil {
		return err
	}
	target, err := elastic.New(s.ElasticsearchURL, s.ElasticsearchKey, "smart-search-active")
	if err != nil {
		return err
	}
	start := time.Now()
	result, err := app.Build(ctx, postgres.NewCatalogSource(pool), target, f, mappingVersion, *batch)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(struct {
		Result    app.BuildResult `json:"result"`
		ElapsedMS int64           `json:"elapsed_ms"`
	}{result, time.Since(start).Milliseconds()})
}
