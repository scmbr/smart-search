// Command api starts the local HTTP application with persistent search replay.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"time"

	lexical "github.com/scmbr/smart-search/internal/application/lexical"
	app "github.com/scmbr/smart-search/internal/application/readiness"
	searchsession "github.com/scmbr/smart-search/internal/application/searchsession"
	search "github.com/scmbr/smart-search/internal/domain/search"
	"github.com/scmbr/smart-search/internal/infrastructure/elastic"
	"github.com/scmbr/smart-search/internal/infrastructure/localconfig"
	"github.com/scmbr/smart-search/internal/infrastructure/postgres"
	"github.com/scmbr/smart-search/internal/infrastructure/searchruntime"
	"github.com/scmbr/smart-search/internal/presentation/httpapi"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := run(ctx, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("api", flag.ContinueOnError)
	address := flags.String("listen", "127.0.0.1:8080", "loopback HTTP address")
	options := searchruntime.Options{}
	nameCatalog := flags.String("name-catalog", "", "compact snapshot-bound name catalog")
	flags.StringVar(&options.Corpus, "vector-corpus", "", "enable hybrid using a versioned corpus; cached embeddings by default")
	flags.BoolVar(&options.LiveQuery, "live-query", false, "explicitly allow new query embeddings within the shared budget")
	flags.StringVar(&options.ParserMode, "parser-mode", "off", "selective parser: off, replay or live")
	if err := flags.Parse(args); err != nil {
		return err
	}
	host, _, err := net.SplitHostPort(*address)
	if flags.NArg() != 0 || err != nil || (host != "localhost" && (net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback())) {
		return errors.New("listen must be a loopback host:port")
	}
	if err := options.Validate(); err != nil {
		return err
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
	// Long-lived idempotency locks must not exhaust the catalog/runtime pool.
	searchPool, err := s.Pool(ctx)
	if err != nil {
		return err
	}
	defer searchPool.Close()
	index, err := elastic.New(s.ElasticsearchURL, s.ElasticsearchKey, "smart-search-active")
	if err != nil {
		return err
	}
	var backend lexical.Backend = index
	if options.Corpus != "" {
		combined, closeRuntime, err := searchruntime.NewFull(ctx, s, index, options)
		if err != nil {
			return err
		}
		defer closeRuntime()
		combined, warmErr := searchruntime.ForHTTP(ctx, combined)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if warmErr != nil {
			fmt.Fprintln(os.Stderr, "Vector cache warmup unavailable; lexical fallback remains enabled")
		}
		backend = combined
	}
	if *nameCatalog != "" {
		named, loadErr := searchruntime.WithNames(ctx, pool, *nameCatalog, index, backend)
		if loadErr != nil {
			return loadErr
		}
		backend = named
	}
	listener, err := net.Listen("tcp", *address)
	if err != nil {
		return errors.New("cannot listen on local HTTP address")
	}
	defer listener.Close()
	store := postgres.NewSearchStore(searchPool)
	service := app.Service{Catalog: postgres.New(pool), Index: index, SearchStore: store}
	searches := searchsession.Service{Store: store, Engine: func(ctx context.Context, r search.Request) (search.Result, error) {
		return lexical.Search(ctx, backend, r)
	}}
	return serve(ctx, listener, httpapi.New(service, searches))
}

func serve(ctx context.Context, listener net.Listener, handler http.Handler) error {
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
	done := make(chan struct{})
	defer close(done)
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		select {
		case <-ctx.Done():
			shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if server.Shutdown(shutdown) != nil {
				_ = server.Close()
			}
		case <-done:
		}
	}()
	err := server.Serve(listener)
	if errors.Is(err, http.ErrServerClosed) {
		<-stopped
		return nil
	}
	return err
}
