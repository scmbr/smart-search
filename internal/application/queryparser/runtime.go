// Package queryparser executes bounded interpretations through persistent ports.
package queryparser

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	p "github.com/scmbr/smart-search/internal/domain/queryparser"
	d "github.com/scmbr/smart-search/internal/domain/search"
)

var ErrBudget = errors.New("parser budget exhausted or invalid")
var ErrBusy = errors.New("shared API workers busy")
var ErrUncertain = errors.New("parser request unresolved; reconciliation required")
var ErrCacheMiss = errors.New("parser cache miss; live disabled")
var ErrRejected = errors.New("parser provider rejected request")
var ErrUnavailable = errors.New("parser unavailable (details redacted)")
var ErrOutput = errors.New("parser output unusable (cached)")

type Entry struct {
	Query   string
	Value   d.Interpretation
	Failure string
}

func ValidateEntry(query string, e Entry) error {
	if e.Query != query {
		return d.ErrInvalidInterpretation
	}
	if e.Failure == "" {
		return d.ValidateInterpretation(query, e.Value)
	}
	if (e.Failure != "invalid" && e.Failure != "refused" && e.Failure != "truncated") || e.Value != (d.Interpretation{}) {
		return d.ErrInvalidInterpretation
	}
	return nil
}

type Reservation struct {
	ID, Key   string
	UpperNano int64
}
type Guard interface{ Close() error }
type Store interface {
	Get(context.Context, string) (Entry, bool, error)
	TryGuard(context.Context, string) (Guard, error)
	Begin(context.Context, Reservation) error
	Settle(context.Context, Reservation, Entry, int64, int64) error
	Rejected(context.Context, string) error
}
type Response struct {
	Model                     string
	InputTokens, OutputTokens int64
	Entry                     Entry
}
type Provider interface {
	Generate(context.Context, string) (Response, error)
}
type Execution struct {
	Value                                    d.Interpretation
	CacheHit                                 bool
	Requests                                 int
	InputTokens, OutputTokens, AccountedNano int64
}
type Service struct {
	Store      Store
	Provider   Provider
	Live       bool
	RunCapNano int64
}

func (s Service) Interpret(ctx context.Context, query string) (d.Interpretation, error) {
	r, err := s.Run(ctx, query)
	return r.Value, err
}
func (s Service) Run(ctx context.Context, query string) (r Execution, err error) {
	if err = ctx.Err(); err != nil {
		return r, err
	}
	plan, err := p.Plan(query)
	if err != nil {
		return r, err
	}
	if s.Store == nil {
		return r, ErrUnavailable
	}
	load := func() (bool, error) {
		entry, found, e := s.Store.Get(ctx, plan.Key)
		if e != nil {
			return false, safeError(e)
		}
		if !found {
			return false, nil
		}
		if ValidateEntry(query, entry) != nil {
			return true, d.ErrInvalidInterpretation
		}
		r.CacheHit = true
		if entry.Failure != "" {
			return true, ErrOutput
		}
		r.Value = entry.Value
		return true, nil
	}
	if found, e := load(); found || e != nil {
		return r, e
	}
	if !s.Live {
		return r, ErrCacheMiss
	}
	cap := s.RunCapNano
	if cap == 0 {
		cap = p.QueryCapNano
	}
	if cap < 1 || cap > p.QueryCapNano || plan.CostUpperNano > cap {
		return r, ErrBudget
	}
	if s.Provider == nil {
		return r, ErrUnavailable
	}
	waitCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	var guard Guard
	for {
		guard, err = s.Store.TryGuard(waitCtx, plan.Key)
		if !errors.Is(err, ErrBusy) {
			break
		}
		timer := time.NewTimer(25 * time.Millisecond)
		select {
		case <-waitCtx.Done():
			timer.Stop()
			if ctx.Err() != nil {
				return r, ctx.Err()
			}
			return r, ErrBusy
		case <-timer.C:
		}
	}
	if err != nil {
		if ctx.Err() != nil {
			return r, ctx.Err()
		}
		if waitCtx.Err() != nil {
			return r, ErrBusy
		}
		return r, safeError(err)
	}
	defer func() {
		if guard.Close() != nil && err == nil {
			err = ErrUnavailable
		}
	}()
	if found, e := load(); found || e != nil {
		return r, e
	}
	if err = ctx.Err(); err != nil {
		return r, err
	}
	var nonce [16]byte
	if _, err = rand.Read(nonce[:]); err != nil {
		return r, ErrUnavailable
	}
	reservation := Reservation{ID: hex.EncodeToString(nonce[:]), Key: plan.Key, UpperNano: plan.CostUpperNano}
	if err = s.Store.Begin(ctx, reservation); err != nil {
		return r, safeError(err)
	}
	r.Requests = 1
	call, stop := context.WithTimeout(ctx, 30*time.Second)
	response, callErr := s.Provider.Generate(call, query)
	stop()
	if callErr != nil {
		if errors.Is(callErr, ErrRejected) {
			clean, end := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			releaseErr := s.Store.Rejected(clean, reservation.ID)
			end()
			if releaseErr != nil {
				return r, ErrUncertain
			}
			return r, ErrRejected
		}
		if ctx.Err() != nil {
			return r, ctx.Err()
		}
		return r, ErrUncertain
	}
	if response.Model != p.Model || response.InputTokens < 1 || response.InputTokens > plan.InputTokenUpper || response.OutputTokens < 0 || response.OutputTokens > p.MaxOutputTokens {
		return r, ErrUncertain
	}
	entry := response.Entry
	if ValidateEntry(query, entry) != nil {
		entry = Entry{Query: query, Failure: "invalid"}
	}
	clean, end := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	settleErr := s.Store.Settle(clean, reservation, entry, response.InputTokens, response.OutputTokens)
	end()
	if settleErr != nil {
		return r, ErrUncertain
	}
	r.InputTokens = response.InputTokens
	r.OutputTokens = response.OutputTokens
	r.AccountedNano = p.Cost(response.InputTokens, response.OutputTokens)
	if ctx.Err() != nil {
		return r, ctx.Err()
	}
	if entry.Failure != "" {
		return r, ErrOutput
	}
	r.Value = entry.Value
	return r, nil
}
func safeError(err error) error {
	for _, known := range []error{ErrBudget, ErrBusy, ErrUncertain, context.Canceled, context.DeadlineExceeded} {
		if errors.Is(err, known) {
			return known
		}
	}
	return ErrUnavailable
}
