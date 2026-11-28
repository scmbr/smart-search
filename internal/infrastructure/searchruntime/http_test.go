package searchruntime

import (
	"context"
	"errors"
	h "github.com/scmbr/smart-search/internal/application/hybrid"
	d "github.com/scmbr/smart-search/internal/domain/search"
	"testing"
	"time"
)

type waitingVector struct{}

func (waitingVector) Candidates(ctx context.Context, _ d.Request) ([]d.GroupCandidate, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

type deadlineVector struct{ t *testing.T }

func (p deadlineVector) Candidates(ctx context.Context, _ d.Request) ([]d.GroupCandidate, error) {
	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) < 9*time.Second || time.Until(deadline) > 15*time.Second {
		p.t.Fatal("live embedding has insufficient or unbounded wait")
	}
	return nil, nil
}

type deadlineParser struct{ t *testing.T }

func (p deadlineParser) Interpret(ctx context.Context, _ string) (d.Interpretation, error) {
	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) < 5*time.Second || time.Until(deadline) > 10*time.Second {
		p.t.Fatal("live parser has insufficient or unbounded wait")
	}
	return d.Interpretation{}, nil
}
func TestHTTPAllowsMeasuredLiveLatencyWithoutWaiting(t *testing.T) {
	b, err := ForHTTP(context.Background(), h.Backend{Vectors: deadlineVector{t}, Parser: deadlineParser{t}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = b.Vectors.Candidates(context.Background(), d.Request{}); err != nil {
		t.Fatal(err)
	}
	if _, err = b.Parser.Interpret(context.Background(), "task"); err != nil {
		t.Fatal(err)
	}
}

type waitingParser struct{}

func (waitingParser) Interpret(ctx context.Context, _ string) (d.Interpretation, error) {
	<-ctx.Done()
	return d.Interpretation{}, ctx.Err()
}
func TestOptionalTimeoutDoesNotCancelParent(t *testing.T) {
	ctx := context.Background()
	_, err := (boundedVectors{waitingVector{}, time.Millisecond}).Candidates(ctx, d.Request{})
	if !errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil {
		t.Fatal(err)
	}
	_, err = (boundedParser{waitingParser{}, time.Millisecond}).Interpret(ctx, "task")
	if !errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	_, err = (boundedVectors{waitingVector{}, time.Second}).Candidates(canceled, d.Request{})
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
