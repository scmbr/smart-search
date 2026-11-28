package searchruntime

import (
	"context"
	"errors"
	d "github.com/scmbr/smart-search/internal/domain/search"
	"testing"
	"time"
)

type waitingVector struct{}

func (waitingVector) Candidates(ctx context.Context, _ d.Request) ([]d.GroupCandidate, error) {
	<-ctx.Done()
	return nil, ctx.Err()
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
