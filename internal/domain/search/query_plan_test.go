package search

import (
	"strings"
	"testing"
)

func TestQueryPlanPreservesInputAndUsesBoundedHypotheses(t *testing.T) {
	for _, tc := range []struct{ query, want string }{
		{"астра линукс", "astra linux"},
		{"каспер для маленького офиса", "kaspersky для small office"},
		{"крипто про цсп 5.0", "криптопро csp 5.0"},
		{"анидеск", "anydesk"},
		{"skaner vs", "сканер вс"},
		{"mts link", "мтс линк"},
		{"фыекф", "astra"},
	} {
		r, err := Prepare(Request{Query: tc.query, Limit: 4})
		if err != nil {
			t.Fatal(err)
		}
		if r.OriginalQuery != tc.query || r.Plan.Version != QueryPolicyVersion || len(r.Plan.Variants) > 5 {
			t.Fatalf("input lost or unbounded plan: %+v", r)
		}
		found := false
		for _, v := range r.Plan.Variants {
			if v.Text == tc.want {
				found = true
			}
		}
		if !found {
			t.Fatalf("missing %q for %q: %+v", tc.want, tc.query, r.Plan)
		}
	}
}

func TestQueryExpansionProtectsNumbersAndDoesNotInventBrand(t *testing.T) {
	for _, query := range []string{"365", "402265", "EP2-06605", "CODE-5003"} {
		r, err := Prepare(Request{Query: query, Limit: 4})
		if err != nil || !r.Plan.ExactOnly || len(r.Plan.Variants) != 0 || r.Query != query {
			t.Fatalf("identifier expanded: %+v err=%v", r, err)
		}
	}
	r, err := Prepare(Request{Query: "криптопор csp 5.0", Limit: 4})
	if err != nil || r.Version != "5.0" {
		t.Fatalf("explicit CSP version lost: %+v %v", r, err)
	}
	for _, variant := range r.Plan.Variants {
		if !strings.Contains(variant.Text, "5.0") {
			t.Fatalf("numeric token altered: %+v", variant)
		}
		for _, term := range variant.Terms {
			if strings.ContainsAny(term.Text, "0123456789") && term.Fuzziness != 0 {
				t.Fatal("numbers made fuzzy")
			}
		}
	}
	if _, err := Prepare(Request{Query: "крипто про цсп 5.0", Version: "4.0", Limit: 4}); err == nil {
		t.Fatal("conflicting version accepted")
	}
	if _, err := Prepare(Request{Query: "CSP5.0", Version: "4.0", Limit: 4}); err == nil {
		t.Fatal("joined CSP version conflict accepted")
	}
	if _, err := Prepare(Request{Query: "CSP 5.0 CSP 4.0", Limit: 4}); err == nil {
		t.Fatal("two conflicting CSP versions accepted")
	}
	if _, err := Prepare(Request{Query: "продление продукта", Kind: "new_license", Limit: 4}); err == nil {
		t.Fatal("conflicting role accepted")
	}
	if _, err := Prepare(Request{Query: "новая лицензия и продление продукта", Limit: 4}); err == nil {
		t.Fatal("conflicting roles in query accepted")
	}
	mixed, err := Prepare(Request{Query: "товар EP2-06605", Limit: 4})
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range mixed.Plan.Variants {
		if !strings.Contains(v.Text, "ep2-06605") {
			t.Fatalf("article transliterated: %+v", v)
		}
	}
}

func TestLongQueryDisablesExpansionWithoutDroppingTerms(t *testing.T) {
	q := strings.Repeat("alpha ", 20) + "5.0"
	r, err := Prepare(Request{Query: q, Limit: 4})
	if err != nil || !r.Plan.ExactOnly || r.Plan.LimitReason != "query_expansion_term_limit" || r.Query != strings.TrimSpace(q) {
		t.Fatalf("long query truncated: %+v %v", r, err)
	}
}

func TestUnknownDecimalConstraintCannotBeRelaxedByFuzzySearch(t *testing.T) {
	r, err := Prepare(Request{Query: "unknown product 5.0", Limit: 4})
	if err != nil || !r.Plan.ExactOnly || r.Plan.LimitReason != "unparsed_numeric_constraint" || len(r.Plan.Variants) != 0 {
		t.Fatalf("unsupported version was expanded without a strict constraint: %+v err=%v", r, err)
	}
}
