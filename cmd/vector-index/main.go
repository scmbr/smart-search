// Command vector-index plans or resumes a complete, explicitly approved corpus.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	runtime "github.com/scmbr/smart-search/internal/application/embeddingpilot"
	app "github.com/scmbr/smart-search/internal/application/vectorindex"
	catalog "github.com/scmbr/smart-search/internal/domain/catalog"
	d "github.com/scmbr/smart-search/internal/domain/vectorcorpus"
	"github.com/scmbr/smart-search/internal/infrastructure/localconfig"
	provider "github.com/scmbr/smart-search/internal/infrastructure/openai"
	"github.com/scmbr/smart-search/internal/infrastructure/postgres"
	"github.com/scmbr/smart-search/internal/infrastructure/vectorcache"
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
	mode := flag.String("mode", "plan", "plan, live or replay")
	file := flag.String("corpus", "artifacts/vector-corpus.json", "label-free manifest")
	approved := flag.String("approved-sha", "", "explicitly approved corpus fingerprint for live only")
	capNano := flag.Int64("max-cost-nano", d.MaxRunNano, "total reservation cap for this run, at most 150000000 ($0.15)")
	out := flag.String("output", "artifacts/vector-index-report.json", "bounded indexing report")
	flag.Parse()
	if flag.NArg() != 0 || (*mode != "plan" && *mode != "live" && *mode != "replay") {
		return errors.New("invalid vector index mode")
	}
	x, err := vectorcache.LoadManifest(*file)
	if err != nil {
		return err
	}
	s, err := localconfig.Load()
	if err != nil {
		return err
	}
	var p runtime.Provider
	if *mode == "live" {
		if *approved != x.SHA || os.Getenv("OPENAI_LIVE_ENABLED") != "true" {
			return errors.New("live requires approved corpus SHA and OPENAI_LIVE_ENABLED=true")
		}
		if os.Getenv("OPENAI_API_KEY") == "" {
			return errors.New("OPENAI_API_KEY missing")
		}
		p = provider.New(os.Getenv("OPENAI_API_KEY"))
	}
	if model := os.Getenv("OPENAI_EMBEDDING_MODEL"); model != "" && model != x.Model {
		return errors.New("embedding model differs from manifest")
	}
	if budget := os.Getenv("OPENAI_BUDGET_USD"); budget != "" && budget != "5" && budget != "5.00" {
		return errors.New("project cap differs from 5 USD")
	}
	pool, err := s.Pool(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()
	var schema string
	err = pool.QueryRow(ctx, `SELECT s.schema_version FROM catalog_snapshots s JOIN catalog_active a ON a.snapshot_id=s.id WHERE s.status='ready' AND s.source_sha256=$1`, x.SourceSHA).Scan(&schema)
	if err != nil || !strings.HasSuffix(schema, "/"+catalog.NormalizationVersion) {
		return errors.New("manifest does not match active normalized feed")
	}
	executor := vectorcache.IndexExecutor{Runtime: runtime.Runtime{Store: postgres.NewEmbeddingStore(pool), Provider: p}, Live: *mode == "live"}
	call, cancel := context.WithTimeout(ctx, 20*time.Minute)
	defer cancel()
	report, runErr := app.Run(call, x, *mode, executor, executor, *capNano)
	if err = os.MkdirAll(filepath.Dir(*out), 0755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	if err = os.WriteFile(*out, append(raw, '\n'), 0600); err != nil {
		return err
	}
	fmt.Printf("%s: %d documents, %d cached/%d missing unique texts, upper %.8f USD, %d requests, complete=%t\n", report.Mode, report.Documents, report.CacheHits, report.CacheMisses, float64(report.CostUpperNano)/1e9, report.Requests, report.Complete)
	return runErr
}
