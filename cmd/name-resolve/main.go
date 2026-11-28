// name-resolve diagnoses catalog name recognition without Elasticsearch or OpenAI.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	app "github.com/scmbr/smart-search/internal/application/names"
	d "github.com/scmbr/smart-search/internal/domain/search"
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
	q := flag.String("query", "", "brand or complete product name")
	flag.Parse()
	if *q == "" || flag.NArg() != 0 {
		return fmt.Errorf("query is required")
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	settings, err := localconfig.Load()
	if err != nil {
		return err
	}
	pool, err := settings.Pool(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()
	dictionary, err := app.Load(ctx, postgres.NewCatalogSource(pool), []d.NameAlias{
		{Alias: "Adobe", Name: "Adobe Systems", Kind: "brand"},
		{Alias: "адоб", Name: "Adobe Systems", Kind: "brand"}, {Alias: "адоб", Name: "Adobe", Kind: "brand"},
		{Alias: "фотошоп", Name: "Adobe Photoshop", Kind: "product"}, {Alias: "Photoshop", Name: "Adobe Photoshop", Kind: "product"},
	})
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(dictionary.ResolveName(*q))
}
