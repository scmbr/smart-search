// Command api starts the local HTTP application; currently health routes only.
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

	app "github.com/scmbr/smart-search/internal/application/readiness"
	"github.com/scmbr/smart-search/internal/infrastructure/elastic"
	"github.com/scmbr/smart-search/internal/infrastructure/localconfig"
	"github.com/scmbr/smart-search/internal/infrastructure/postgres"
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
	if err := flags.Parse(args); err != nil {
		return err
	}
	host, _, err := net.SplitHostPort(*address)
	if flags.NArg() != 0 || err != nil || (host != "localhost" && (net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback())) {
		return errors.New("listen must be a loopback host:port")
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
	index, err := elastic.New(s.ElasticsearchURL, s.ElasticsearchKey, "smart-search-active")
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", *address)
	if err != nil {
		return errors.New("cannot listen on local HTTP address")
	}
	defer listener.Close()
	service := app.Service{Catalog: postgres.New(pool), Index: index}
	return serve(ctx, listener, httpapi.New(service))
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
