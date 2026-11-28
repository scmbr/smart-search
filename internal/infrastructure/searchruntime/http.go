package searchruntime

import (
	"context"
	hybrid "github.com/scmbr/smart-search/internal/application/hybrid"
	d "github.com/scmbr/smart-search/internal/domain/search"
	"time"
)

// ForHTTP warms only cached corpus vectors, then bounds optional request ports.
// A warmup failure is reported to the caller; lexical remains available.
func ForHTTP(ctx context.Context, b hybrid.Backend) (hybrid.Backend, error) {
	var err error
	if warm, ok := b.Vectors.(interface{ Warm(context.Context) error }); ok {
		warmCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		err = warm.Warm(warmCtx)
		cancel()
	}
	if b.Vectors != nil {
		b.Vectors = boundedVectors{b.Vectors, 15 * time.Second}
	}
	if b.Parser != nil {
		b.Parser = boundedParser{b.Parser, 10 * time.Second}
	}
	return b, err
}

type boundedVectors struct {
	next    hybrid.Vectors
	timeout time.Duration
}

func (b boundedVectors) Candidates(ctx context.Context, r d.Request) ([]d.GroupCandidate, error) {
	child, cancel := context.WithTimeout(ctx, b.timeout)
	defer cancel()
	return b.next.Candidates(child, r)
}

type boundedParser struct {
	next    hybrid.Parser
	timeout time.Duration
}

func (b boundedParser) Interpret(ctx context.Context, q string) (d.Interpretation, error) {
	child, cancel := context.WithTimeout(ctx, b.timeout)
	defer cancel()
	return b.next.Interpret(child, q)
}
