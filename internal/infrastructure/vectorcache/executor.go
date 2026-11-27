package vectorcache

import (
	"context"
	runtime "github.com/scmbr/smart-search/internal/application/embeddingpilot"
	app "github.com/scmbr/smart-search/internal/application/vectorindex"
	d "github.com/scmbr/smart-search/internal/domain/embeddingpilot"
)

// IndexExecutor bridges application ports without importing application into application.
type IndexExecutor struct {
	Runtime runtime.Runtime
	Live    bool
}

func (e IndexExecutor) Get(ctx context.Context, keys []string) (map[string][]float64, error) {
	return e.Runtime.Store.Get(ctx, keys)
}
func (e IndexExecutor) Budget(ctx context.Context) (app.Budget, error) {
	b, err := e.Runtime.Store.Budget(ctx)
	return app.Budget{ActualNano: b.ActualNano, ReservedNano: b.UncertainNano, CapNano: b.CapNano}, err
}
func (e IndexExecutor) Ensure(ctx context.Context, texts []string, capNano int64) (app.Execution, error) {
	r, err := e.Runtime.Ensure(ctx, texts, runtime.Options{Live: e.Live, RunCapNano: capNano, ProjectCapNano: d.ProjectCapNano})
	return app.Execution{Requests: r.Requests, Tokens: r.Tokens, ActualNano: r.ActualNano, ReservedNano: r.ReservedNano}, err
}
