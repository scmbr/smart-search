package embeddingpilot

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	randv2 "math/rand/v2"
	"time"

	d "github.com/scmbr/smart-search/internal/domain/embeddingpilot"
)

var ErrBusy = errors.New("embedding workers busy")
var ErrUncertain = errors.New("embedding input has unresolved request; reconciliation required")
var ErrCacheMiss = errors.New("embedding cache miss; live disabled")

// Failure carries only safe classifications, never a raw provider response.
type Failure struct {
	Kind       string
	RetryAfter time.Duration
}

func (e *Failure) Error() string { return "embedding provider: " + e.Kind }

type Guard interface{ Close() error }
type RuntimeStore interface {
	Store
	TryGuard(context.Context, []string) (Guard, error)
	BeginAttempt(context.Context, Reservation, []string) error
	Rejected(context.Context, string) error
}
type Execution struct {
	Vectors                          map[string][]float64
	Requests                         int
	Tokens, ActualNano, ReservedNano int64
	LatencyMS                        []float64
}

// Runtime is stateless; concurrency and deduplication are shared through Store.
type Runtime struct {
	Store    RuntimeStore
	Provider Provider
}

func (s Runtime) Ensure(ctx context.Context, texts []string, o Options) (r Execution, err error) {
	r.Vectors = map[string][]float64{}
	if len(texts) == 0 || len(texts) > 612 {
		return r, errors.New("invalid embedding input count")
	}
	unique := []string{}
	seen := map[string]bool{}
	for _, text := range texts {
		if _, err = d.TokenBound(text); err != nil {
			return r, err
		}
		key := d.CacheKey(text)
		if !seen[key] {
			seen[key] = true
			unique = append(unique, text)
		}
	}
	if o.Live && (s.Provider == nil || o.RunCapNano < 1 || o.RunCapNano > d.RunCapNano || o.ProjectCapNano < 1 || o.ProjectCapNano > d.ProjectCapNano) {
		return r, ErrBudget
	}
	for start := 0; start < len(unique); start += 32 {
		if err = s.batch(ctx, unique[start:min(start+32, len(unique))], o, &r); err != nil {
			return r, err
		}
	}
	return r, nil
}
func (s Runtime) batch(ctx context.Context, texts []string, o Options, r *Execution) (err error) {
	keys := make([]string, len(texts))
	for i, t := range texts {
		keys[i] = d.CacheKey(t)
	}
	load := func() ([]string, error) {
		cache, e := s.Store.Get(ctx, keys)
		if e != nil {
			return nil, e
		}
		missing := []string{}
		for i, key := range keys {
			if v, ok := cache[key]; ok {
				if e = d.ValidateVector(v); e != nil {
					return nil, e
				}
				r.Vectors[key] = v
			} else {
				missing = append(missing, texts[i])
			}
		}
		return missing, nil
	}
	missing, err := load()
	if err != nil || len(missing) == 0 {
		return err
	}
	if !o.Live {
		return ErrCacheMiss
	}
	deadline := time.Now().Add(2 * time.Second)
	guardContext, stopGuardWait := context.WithDeadline(ctx, deadline)
	defer stopGuardWait()
	var guard Guard
	for {
		if err = ctx.Err(); err != nil {
			return err
		}
		guard, err = s.Store.TryGuard(guardContext, keys)
		if errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil {
			return ErrBusy
		}
		if !errors.Is(err, ErrBusy) {
			break
		}
		if time.Now().After(deadline) {
			return ErrBusy
		}
		if err = wait(ctx, 25*time.Millisecond); err != nil {
			return err
		}
	}
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, guard.Close()) }()
	missing, err = load()
	if err != nil || len(missing) == 0 {
		return err
	}
	var bound int64
	missingKeys := make([]string, len(missing))
	for i, t := range missing {
		n, _ := d.TokenBound(t)
		bound += n
		missingKeys[i] = d.CacheKey(t)
	}
	for attempt := 0; attempt < 3; attempt++ {
		if err = ctx.Err(); err != nil {
			return err
		}
		upper := d.Cost(bound)
		if upper > o.RunCapNano-r.ReservedNano {
			return ErrBudget
		}
		var nonce [16]byte
		if _, err = rand.Read(nonce[:]); err != nil {
			return err
		}
		reservation := Reservation{ID: hex.EncodeToString(nonce[:]), UpperNano: upper, CapNano: o.ProjectCapNano}
		if err = s.Store.BeginAttempt(ctx, reservation, missingKeys); err != nil {
			return err
		}
		r.ReservedNano += upper
		r.Requests++
		call, cancel := context.WithTimeout(ctx, 30*time.Second)
		start := time.Now()
		response, e := s.Provider.Embed(call, missing)
		cancel()
		r.LatencyMS = append(r.LatencyMS, float64(time.Since(start).Microseconds())/1000)
		if e != nil {
			var failure *Failure
			if !errors.As(e, &failure) || (failure.Kind != "rate_limit" && failure.Kind != "quota" && failure.Kind != "rejected") {
				return e
			}
			// Only a definitive rejection releases the input claim, never its cost reserve.
			clean, stop := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			releaseErr := s.Store.Rejected(clean, reservation.ID)
			stop()
			if releaseErr != nil {
				return errors.Join(e, releaseErr)
			}
			if failure.Kind != "rate_limit" || attempt == 2 {
				return e
			}
			delay := time.Duration(250*(1<<attempt))*time.Millisecond + time.Duration(randv2.IntN(101))*time.Millisecond
			if failure.RetryAfter > delay {
				delay = failure.RetryAfter
			}
			if delay > 5*time.Second {
				return e
			}
			if err = wait(ctx, delay); err != nil {
				return err
			}
			continue
		}
		if response.Model != d.Model || response.Tokens < 1 || response.Tokens > bound || len(response.Vectors) != len(missing) {
			return errors.New("invalid embedding response; reservation retained")
		}
		entries := make([]Entry, len(missing))
		for i, v := range response.Vectors {
			if err = d.ValidateVector(v); err != nil {
				return err
			}
			entries[i] = Entry{Key: missingKeys[i], Vector: v}
		}
		actual := d.Cost(response.Tokens)
		if err = s.Store.Settle(ctx, reservation, actual, response.Tokens, entries); err != nil {
			return err
		}
		r.Tokens += response.Tokens
		r.ActualNano += actual
		for _, entry := range entries {
			r.Vectors[entry.Key] = entry.Vector
		}
		return nil
	}
	return errors.New("embedding attempts exhausted")
}
func wait(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
