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
	"time"

	embedding "github.com/scmbr/smart-search/internal/application/embeddingpilot"
	a "github.com/scmbr/smart-search/internal/application/queryparser"
	p "github.com/scmbr/smart-search/internal/domain/queryparser"
	"github.com/scmbr/smart-search/internal/infrastructure/localconfig"
	provider "github.com/scmbr/smart-search/internal/infrastructure/openai"
	"github.com/scmbr/smart-search/internal/infrastructure/postgres"
)

type report struct {
	Mode                      string
	Plan                      p.Estimate
	Execution                 a.Execution
	BudgetBefore, BudgetAfter embedding.Budget
	Error                     string
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run(parent context.Context, args []string, out io.Writer) error {
	flags := flag.NewFlagSet("query-parse", flag.ContinueOnError)
	query := flags.String("query", "", "original query")
	mode := flags.String("mode", "plan", "plan, replay or live")
	cap := flags.Int64("max-cost-nano", p.QueryCapNano, "per-query reservation cap, up to 1000000 nanodollars")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 || (*mode != "plan" && *mode != "replay" && *mode != "live") || *cap < 1 || *cap > p.QueryCapNano {
		return errors.New("invalid parser mode, arguments or cost cap")
	}
	plan, err := p.Plan(*query)
	if err != nil {
		return err
	}
	if plan.CostUpperNano > *cap {
		return a.ErrBudget
	}
	r := report{Mode: *mode, Plan: plan}
	if *mode == "plan" {
		return json.NewEncoder(out).Encode(r)
	}
	settings, err := localconfig.Load()
	if err != nil {
		return err
	}
	if *mode == "live" {
		if err = localconfig.CheckParserLive(); err != nil {
			return err
		}
	}
	ctx, cancel := context.WithTimeout(parent, 45*time.Second)
	defer cancel()
	pool, err := settings.Pool(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()
	ledger := postgres.NewEmbeddingStore(pool)
	if r.BudgetBefore, err = ledger.Budget(ctx); err != nil {
		return a.ErrUnavailable
	}
	service := a.Service{Store: postgres.NewQueryParserStore(pool), Live: *mode == "live", RunCapNano: *cap}
	if service.Live {
		service.Provider = provider.NewQueryParser(os.Getenv("OPENAI_API_KEY"))
	}
	r.Execution, err = service.Run(ctx, *query)
	clean, stop := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	var budgetErr error
	r.BudgetAfter, budgetErr = ledger.Budget(clean)
	stop()
	if err != nil {
		r.Error = err.Error()
	}
	writeErr := json.NewEncoder(out).Encode(r)
	if budgetErr != nil {
		return errors.Join(err, writeErr, a.ErrUnavailable)
	}
	return errors.Join(err, writeErr)
}
