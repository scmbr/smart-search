package vectorindex

import (
	"context"
	"errors"
	"fmt"
	embedding "github.com/scmbr/smart-search/internal/domain/embeddingpilot"
	d "github.com/scmbr/smart-search/internal/domain/vectorcorpus"
	"strings"
	"testing"
)

type memory struct {
	data         map[string][]float64
	calls        int
	fail         bool
	spent        int64
	reserveExtra int64
}

func (m *memory) Get(context.Context, []string) (map[string][]float64, error) { return m.data, nil }
func (m *memory) Budget(context.Context) (Budget, error) {
	return Budget{ActualNano: m.spent, CapNano: embedding.ProjectCapNano}, nil
}
func (m *memory) Ensure(_ context.Context, texts []string, capNano int64) (Execution, error) {
	m.calls++
	var upper int64
	for _, text := range texts {
		n, _ := embedding.TokenBound(text)
		upper += embedding.Cost(n)
	}
	upper += m.reserveExtra
	if m.fail {
		return Execution{ReservedNano: upper}, errors.New("uncertain")
	}
	for _, text := range texts {
		v := make([]float64, embedding.Dimensions)
		v[0] = 1
		m.data[embedding.CacheKey(text)] = v
	}
	m.spent += 20
	return Execution{Requests: 1, Tokens: 1, ActualNano: 20, ReservedNano: upper}, nil
}

func TestRunBudgetIncludesRetryReservationsAcrossBatches(t *testing.T) {
	x := corpus(33)
	for i := range x.Documents {
		x.Documents[i].Text = fmt.Sprintf("unique text %02d", i)
	}
	x.SHA = d.Fingerprint(x)
	var capNano int64
	for _, doc := range x.Documents {
		n, _ := embedding.TokenBound(doc.Text)
		capNano += embedding.Cost(n)
	}
	m := &memory{data: map[string][]float64{}, reserveExtra: 1}
	r, err := Run(context.Background(), x, "live", m, m, capNano)
	if !errors.Is(err, ErrBudget) || m.calls != 1 || r.Complete {
		t.Fatalf("%+v calls=%d err=%v", r, m.calls, err)
	}
}
func corpus(n int) d.Corpus {
	x := d.Corpus{Version: d.Version, TextVersion: embedding.TextVersion, Model: embedding.Model, Dimensions: embedding.Dimensions, SourceSHA: strings.Repeat("a", 64), Offers: n}
	for i := 0; i < n; i++ {
		g := fmt.Sprintf("%04d", i)
		x.Documents = append(x.Documents, d.Document{ID: "v/group/" + g, GroupID: g, Vendor: "v", Text: "same text", SourceURL: "https://store.softline.ru/v/" + g + "/"})
	}
	x.SHA = d.Fingerprint(x)
	return x
}
func TestPlanDedupAndNoExecution(t *testing.T) {
	m := &memory{data: map[string][]float64{}}
	r, err := Run(context.Background(), corpus(3), "plan", m, m, d.MaxRunNano)
	if err != nil || r.UniqueInputs != 1 || r.CacheMisses != 1 || r.CostUpperNano != int64(len("same text")+16)*20 || m.calls != 0 {
		t.Fatalf("%+v %v", r, err)
	}
	if _, err = Run(context.Background(), corpus(3), "replay", m, m, d.MaxRunNano); err == nil || m.calls != 0 {
		t.Fatal("replay cannot pay")
	}
}
func TestBuildReusesCacheAndStopsAtRunLimit(t *testing.T) {
	m := &memory{data: map[string][]float64{}}
	r, err := Run(context.Background(), corpus(3), "live", m, m, d.MaxRunNano)
	if err != nil || !r.Complete || r.Requests != 1 || m.calls != 1 {
		t.Fatalf("%+v %v", r, err)
	}
	r, err = Run(context.Background(), corpus(3), "replay", m, nil, d.MaxRunNano)
	if err != nil || !r.Complete || r.CacheHits != 1 || m.calls != 1 {
		t.Fatalf("%+v %v", r, err)
	}
	m = &memory{data: map[string][]float64{}}
	if _, err = Run(context.Background(), corpus(3), "live", m, m, 1); err == nil || m.calls != 0 {
		t.Fatal("over-budget call")
	}
	m.fail = true
	if _, err = Run(context.Background(), corpus(3), "live", m, m, d.MaxRunNano); err == nil || m.calls != 1 {
		t.Fatal("must stop on uncertainty")
	}
}
