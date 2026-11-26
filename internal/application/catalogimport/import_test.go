package catalogimport

import (
	"context"
	"errors"
	"testing"

	domain "github.com/scmbr/smart-search/internal/domain/catalog"
)

type fakeStore struct {
	state     domain.Snapshot
	rows      []domain.Record
	finished  bool
	closed    bool
	failSave  bool
	finishErr error
	closeErr  error
}

func (f *fakeStore) Begin(context.Context, domain.Identity) (domain.ImportSession, error) {
	return f, nil
}
func (f *fakeStore) Snapshot() domain.Snapshot { return f.state }
func (f *fakeStore) Save(_ context.Context, first int64, records []domain.Record) error {
	if f.failSave {
		return errors.New("write failure")
	}
	if first != f.state.Checkpoint+1 {
		return errors.New("wrong checkpoint")
	}
	f.rows = append(f.rows, records...)
	f.state.Checkpoint += int64(len(records))
	return nil
}
func (f *fakeStore) Finish(_ context.Context, v domain.Validation) error {
	if f.finishErr != nil {
		return f.finishErr
	}
	f.finished = true
	f.state.Validation = v
	if v.Eligible() {
		f.state.Status = domain.Ready
	} else {
		f.state.Status = domain.Invalid
	}
	return nil
}
func (f *fakeStore) Close() error { f.closed = true; return f.closeErr }

type fakeSource struct {
	count   int
	fail    bool
	invalid bool
	reads   int
}

func (*fakeSource) Identity() domain.Identity {
	return domain.Identity{SourceSHA256: "source", SchemaVersion: "v1"}
}
func (s *fakeSource) Read(ctx context.Context, emit func(context.Context, domain.Record) error) (domain.Validation, error) {
	s.reads++
	for i := 0; i < s.count; i++ {
		if err := ctx.Err(); err != nil {
			return domain.Validation{}, err
		}
		r := domain.Record{Offer: &domain.Offer{ID: string(rune('a' + i))}}
		if i == 0 {
			r = domain.Record{Category: &domain.Category{ID: "cat"}}
		}
		if err := emit(ctx, r); err != nil {
			return domain.Validation{}, err
		}
	}
	if s.fail {
		return domain.Validation{}, errors.New("late checksum failure")
	}
	v := domain.Validation{Complete: true, Categories: 1, Offers: s.count - 1, Accepted: s.count - 1}
	if s.invalid {
		v.Issues = 1
	}
	return v, nil
}

func TestImportResumesCommittedPrefix(t *testing.T) {
	f := &fakeStore{state: domain.Snapshot{Status: domain.Staging}}
	s := &fakeSource{count: 5, fail: true}
	_, err := Run(context.Background(), s, f, 2)
	if err == nil || f.finished || f.state.Checkpoint != 4 || !f.closed {
		t.Fatalf("failure leaked activation: %+v %v", f, err)
	}
	s.fail = false
	f.closed = false
	r, err := Run(context.Background(), s, f, 2)
	if err != nil || !f.finished || len(f.rows) != 5 || r.Snapshot.Status != domain.Ready || !f.closed {
		t.Fatalf("retry: %+v %v", r, err)
	}
}

func TestImportInvalidDoesNotActivate(t *testing.T) {
	f := &fakeStore{state: domain.Snapshot{Status: domain.Staging}}
	_, err := Run(context.Background(), &fakeSource{count: 4, invalid: true}, f, 2)
	if !errors.Is(err, domain.ErrInvalidCatalog) || f.state.Status != domain.Invalid {
		t.Fatalf("invalid: %+v %v", f, err)
	}
}

func TestReadyImportDoesNotReadOrReactivate(t *testing.T) {
	f := &fakeStore{state: domain.Snapshot{Status: domain.Ready}}
	s := &fakeSource{}
	r, err := Run(context.Background(), s, f, 2)
	if err != nil || !r.Reused || s.reads != 0 || f.finished {
		t.Fatalf("reuse: %+v %v", r, err)
	}
}

func TestImportFailures(t *testing.T) {
	for _, name := range []string{"save", "cancel", "short source", "invalid snapshot", "batch size"} {
		t.Run(name, func(t *testing.T) {
			f := &fakeStore{state: domain.Snapshot{Status: domain.Staging}}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			batch := 2
			s := &fakeSource{count: 3}
			switch name {
			case "save":
				f.failSave = true
			case "cancel":
				cancel()
			case "short source":
				f.state.Checkpoint = 10
			case "invalid snapshot":
				f.state.Status = domain.Invalid
			case "batch size":
				batch = 0
			}
			if _, err := Run(ctx, s, f, batch); err == nil || f.finished {
				t.Fatalf("expected failure without finish: %+v %v", f, err)
			}
		})
	}
}

func TestFinishAndCloseErrorsArePreserved(t *testing.T) {
	finishErr, closeErr := errors.New("finish failure"), errors.New("unlock failure")
	f := &fakeStore{state: domain.Snapshot{Status: domain.Staging}, finishErr: finishErr, closeErr: closeErr}
	r, err := Run(context.Background(), &fakeSource{count: 3}, f, 2)
	if !errors.Is(err, finishErr) || !errors.Is(err, closeErr) || !f.closed || r.Snapshot.Status != domain.Staging {
		t.Fatalf("result=%+v err=%v", r, err)
	}
}
