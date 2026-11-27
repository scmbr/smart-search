// Command embedding-pilot runs an isolated, budgeted development experiment.
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
	"time"

	a "github.com/scmbr/smart-search/internal/application/embeddingpilot"
	d "github.com/scmbr/smart-search/internal/domain/embeddingpilot"
	"github.com/scmbr/smart-search/internal/infrastructure/elastic"
	"github.com/scmbr/smart-search/internal/infrastructure/localconfig"
	provider "github.com/scmbr/smart-search/internal/infrastructure/openai"
	"github.com/scmbr/smart-search/internal/infrastructure/postgres"
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
	mode := flag.String("mode", "plan", "plan, live, or replay")
	file := flag.String("dataset", "artifacts/embedding-pilot-dataset.json", "prepared development-only dataset")
	out := flag.String("output", "artifacts/embedding-pilot-report.json", "report path (no keys)")
	flag.Parse()
	if flag.NArg() != 0 || (*mode != "plan" && *mode != "live" && *mode != "replay") {
		return errors.New("invalid pilot mode")
	}
	raw, err := os.ReadFile(*file)
	if err != nil {
		return errors.New("cannot read pilot dataset")
	}
	if len(raw) > 4*1024*1024 {
		return errors.New("pilot dataset too large")
	}
	var dataset d.Dataset
	if err = json.Unmarshal(raw, &dataset); err != nil {
		return errors.New("invalid pilot dataset JSON")
	}
	if err = d.Validate(dataset); err != nil {
		return err
	}
	raw, err = os.ReadFile("testdata/evaluation/scenarios.json")
	if err != nil {
		return err
	}
	var source struct {
		SHA string `json:"cases_sha256"`
	}
	if err = json.Unmarshal(raw, &source); err != nil {
		return err
	}
	if dataset.TuningSHA != source.SHA {
		return errors.New("pilot tuning SHA differs")
	}
	s, err := localconfig.Load()
	if err != nil {
		return err
	}
	if model := os.Getenv("OPENAI_EMBEDDING_MODEL"); model != "" && model != d.Model {
		return errors.New("pilot price is only verified for text-embedding-3-small")
	}
	if budget := os.Getenv("OPENAI_BUDGET_USD"); budget != "" && budget != "5.00" && budget != "5" {
		return errors.New("project budget must match approved 5 USD")
	}
	var p a.Provider
	if *mode == "live" {
		if os.Getenv("OPENAI_LIVE_ENABLED") != "true" {
			return errors.New("live API disabled; use plan/replay or explicitly enable the approved pilot")
		}
		key := os.Getenv("OPENAI_API_KEY")
		if key == "" {
			return errors.New("OPENAI_API_KEY missing")
		}
		p = provider.New(key)
	}
	pool, err := s.Pool(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err = postgres.Migrate(ctx, pool); err != nil {
		return err
	}
	store := postgres.NewEmbeddingStore(pool)
	var lexical a.Lexical
	if *mode != "plan" {
		c, err := elastic.New(s.ElasticsearchURL, s.ElasticsearchKey, "smart-search-active")
		if err != nil {
			return err
		}
		lexical = elastic.PilotRank{Client: c}
	}
	call, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	report, runErr := a.Run(call, dataset, a.Options{Live: *mode == "live", Evaluate: *mode != "plan", RunCapNano: d.RunCapNano, ProjectCapNano: d.ProjectCapNano}, store, p, lexical)
	if err = os.MkdirAll(filepath.Dir(*out), 0755); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	if err = os.WriteFile(*out, append(payload, '\n'), 0600); err != nil {
		return err
	}
	fmt.Println(a.Summary(report))
	return runErr
}
