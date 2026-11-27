package hybrid

import (
	"context"
	"errors"
	"reflect"
	"testing"

	lexical "github.com/scmbr/smart-search/internal/application/lexical"
	d "github.com/scmbr/smart-search/internal/domain/search"
)

type fakeParser struct {
	calls    int
	response d.Interpretation
	err      error
	cancel   context.CancelFunc
}

func (f *fakeParser) Interpret(_ context.Context, _ string) (d.Interpretation, error) {
	f.calls++
	if f.cancel != nil {
		f.cancel()
	}
	return f.response, f.err
}

type recordingVectors struct {
	request d.Request
	calls   int
}

func (v *recordingVectors) Candidates(_ context.Context, r d.Request) ([]d.GroupCandidate, error) {
	v.request = r
	v.calls++
	return nil, nil
}

func TestSelectiveInterpretationPreservesFilters(t *testing.T) {
	r, err := d.Prepare(d.Request{Query: "нужно удаленно помогать сотрудникам с компьютерами", Limit: 4, MinDevices: 25, Policy: "available_only"})
	if err != nil {
		t.Fatal(err)
	}
	original := r
	p := &fakeParser{response: d.Interpretation{Version: d.InterpretationVersion, SourceQuery: r.OriginalQuery, Status: "resolved", SemanticQuery: "программа удаленного доступа для поддержки сотрудников"}}
	v := &recordingVectors{}
	b := Backend{Lexical: &fakeBackend{base: d.Result{CatalogID: "c", IndexVersion: "i"}}, Vectors: v, Parser: p}
	result, err := b.Search(context.Background(), r)
	if err != nil || p.calls != 1 || v.calls != 1 || result.Hybrid.InterpretationStatus != "used" {
		t.Fatalf("%+v %v", result, err)
	}
	if v.request.VectorQuery() != p.response.SemanticQuery {
		t.Fatal("rewrite not used")
	}
	expectedVectorRequest := original
	expectedVectorRequest.Limit = d.CandidateLimit
	v.request.SemanticQuery = ""
	if !reflect.DeepEqual(v.request, expectedVectorRequest) || !reflect.DeepEqual(r, original) {
		t.Fatal("request mutated")
	}
}

type recordingCatalog struct {
	fakeBackend
	request d.Request
}

func (c *recordingCatalog) Resolve(ctx context.Context, r d.Request, groups []d.GroupCandidate, priced bool, catalog, index string) ([]d.Hit, error) {
	c.request = r
	return c.fakeBackend.Resolve(ctx, r, groups, priced, catalog, index)
}

func TestInterpretationNeverBypassesFinalSKUValidation(t *testing.T) {
	query := "нужно удаленно помогать сотрудникам с компьютерами"
	p := &fakeParser{response: d.Interpretation{Version: d.InterpretationVersion, SourceQuery: query, Status: "resolved", SemanticQuery: "программа удаленной поддержки"}}
	hit := h("remote", true)
	hit.Document.ProductType = "hardware"
	c := &recordingCatalog{fakeBackend: fakeBackend{base: d.Result{CatalogID: "c", IndexVersion: "i"}, resolved: []d.Hit{hit}}}
	b := Backend{Lexical: c, Vectors: &fakeVectors{}, Parser: p}
	input := d.Request{Query: query, Limit: 4, ProductType: "software"}
	if _, err := lexical.Search(context.Background(), b, input); !errors.Is(err, d.ErrUnavailable) {
		t.Fatalf("invalid SKU accepted: %v", err)
	}
	if c.request.OriginalQuery != query || c.request.SemanticQuery != "" || c.request.ProductType != "software" {
		t.Fatalf("resolve lost original request: %+v", c.request)
	}
	hit.Document.ProductType = "software"
	c.resolved = []d.Hit{hit}
	result, err := lexical.Search(context.Background(), b, input)
	if err != nil || result.Query != query || len(result.Hits) != 1 {
		t.Fatalf("%+v %v", result, err)
	}
}

func TestInterpretationSelectionAndFallback(t *testing.T) {
	query := "нужно удаленно помогать сотрудникам с компьютерами"
	for _, tc := range []struct {
		name, status           string
		hits                   bool
		query                  string
		exact                  bool
		family                 string
		invalid, failure       bool
		wantCalls, wantVectors int
		wantStatus             string
	}{
		{name: "resolved", status: "resolved", wantCalls: 1, wantVectors: 1, wantStatus: "used"},
		{name: "clarify", status: "clarify", wantCalls: 1, wantStatus: "clarification_required"},
		{name: "unsupported", status: "unsupported", wantCalls: 1, wantStatus: "unsupported"},
		{name: "invalid", status: "resolved", invalid: true, wantCalls: 1, wantVectors: 1, wantStatus: "invalid"},
		{name: "failure", failure: true, wantCalls: 1, wantVectors: 1, wantStatus: "fallback"},
		{name: "lexical", hits: true, wantVectors: 1, wantStatus: "not_selected"},
		{name: "short", query: "удаленный доступ", wantVectors: 1, wantStatus: "not_selected"},
		{name: "family", family: "anydesk", wantVectors: 1, wantStatus: "not_selected"},
		{name: "exact", exact: true, wantStatus: "not_selected"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q := query
			if tc.query != "" {
				q = tc.query
			}
			r := d.Request{OriginalQuery: q, Query: q, Limit: 4, FamilyID: tc.family, Plan: d.QueryPlan{ExactOnly: tc.exact}}
			base := d.Result{CatalogID: "c", IndexVersion: "i"}
			if tc.hits {
				base.Hits = []d.Hit{h("family", true)}
			}
			response := d.Interpretation{Version: d.InterpretationVersion, SourceQuery: q, Status: tc.status}
			if tc.status == "resolved" {
				response.SemanticQuery = "программа удаленного доступа"
			}
			if tc.invalid {
				response.SourceQuery = "другой запрос"
			}
			p := &fakeParser{response: response}
			if tc.failure {
				p.err = errors.New("secret")
			}
			v := &recordingVectors{}
			result, err := (Backend{Lexical: &fakeBackend{base: base}, Vectors: v, Parser: p}).Search(context.Background(), r)
			if err != nil || p.calls != tc.wantCalls || v.calls != tc.wantVectors || result.Hybrid.InterpretationStatus != tc.wantStatus {
				t.Fatalf("%+v calls=%d/%d %v", result, p.calls, v.calls, err)
			}
			if tc.wantStatus != "used" && v.request.SemanticQuery != "" {
				t.Fatal("invalid rewrite used")
			}
		})
	}
}

func TestInterpretationCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p := &fakeParser{cancel: cancel}
	v := &recordingVectors{}
	_, err := (Backend{Lexical: &fakeBackend{base: d.Result{CatalogID: "c", IndexVersion: "i"}}, Vectors: v, Parser: p}).Search(ctx, d.Request{Query: "нужно удаленно помогать сотрудникам с компьютерами", OriginalQuery: "нужно удаленно помогать сотрудникам с компьютерами", Limit: 4})
	if !errors.Is(err, context.Canceled) || v.calls != 0 {
		t.Fatalf("%v calls=%d", err, v.calls)
	}
}
