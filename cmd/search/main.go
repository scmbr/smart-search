package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	app "github.com/scmbr/smart-search/internal/application/lexical"
	domain "github.com/scmbr/smart-search/internal/domain/search"
	"github.com/scmbr/smart-search/internal/infrastructure/elastic"
	"github.com/scmbr/smart-search/internal/infrastructure/localconfig"
	"os"
	"os/signal"
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
	r := domain.Request{}
	corpus := flag.String("hybrid-corpus", "", "experimental cached development corpus; no paid API calls")
	flag.StringVar(&r.Query, "query", "", "search text")
	flag.StringVar(&r.FamilyID, "family", "", "explicit confirmed family ID")
	flag.IntVar(&r.Limit, "limit", 4, "maximum families (4..20)")
	flag.StringVar(&r.Policy, "policy", "priced_first", "priced_first or available_only")
	flag.StringVar(&r.Version, "version", "", "explicit numeric product version")
	flag.StringVar(&r.Kind, "kind", "", "explicit license role")
	flag.StringVar(&r.Platform, "platform", "", "explicit supported OS")
	flag.StringVar(&r.ProductType, "product-type", "", "hardware or software")
	flag.IntVar(&r.MinUsers, "min-users", 0, "minimum known user upper bound")
	flag.IntVar(&r.MinDevices, "min-devices", 0, "minimum known device upper bound")
	flag.IntVar(&r.MinIPAddresses, "min-ip-addresses", 0, "minimum known IP address upper bound")
	flag.IntVar(&r.MinParticipants, "min-participants", 0, "minimum known participant upper bound")
	flag.StringVar(&r.Edition, "edition", "", "known edition")
	flag.StringVar(&r.TermKind, "term-kind", "", "fixed or perpetual")
	flag.IntVar(&r.TermMonths, "term-months", 0, "exact license term in months")
	flag.StringVar(&r.Deployment, "deployment", "", "server or workstation")
	flag.Parse()
	if flag.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments")
	}
	if _, err := domain.Prepare(r); err != nil {
		return err
	}
	s, err := localconfig.Load()
	if err != nil {
		return err
	}
	backend, err := elastic.New(s.ElasticsearchURL, s.ElasticsearchKey, "smart-search-active")
	if err != nil {
		return err
	}
	var searchBackend app.Backend = backend
	if *corpus != "" {
		combined, closePool, err := cachedHybrid(ctx, s, backend, *corpus)
		if err != nil {
			return err
		}
		defer closePool()
		searchBackend = combined
	}
	result, err := app.Search(ctx, searchBackend, r)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(result)
}
