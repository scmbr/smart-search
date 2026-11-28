package httpapi

import (
	"encoding/json"
	s "github.com/scmbr/smart-search/internal/domain/search"
	d "github.com/scmbr/smart-search/internal/domain/searchsession"
	"reflect"
	"testing"
)

func TestHybridStatusIsVisibleAndReplayDeterministic(t *testing.T) {
	for _, tc := range []struct {
		vector, parser, reason string
		clarify                bool
	}{
		{"used", "not_selected", "", false},
		{"fallback", "disabled", "embeddings_not_ready", false},
		{"catalog_mismatch", "not_selected", "embeddings_not_ready", false},
		{"no_candidates", "fallback", "openai_unavailable", false},
		{"used", "invalid", "openai_unavailable", false},
		{"skipped_exact", "not_selected", "", false},
		{"skipped_interpretation", "clarification_required", "", true},
		{"skipped_interpretation", "unsupported", "", true},
	} {
		mode := "lexical"
		if tc.vector == "used" {
			mode = "hybrid"
		}
		record := d.Record{ID: "search", Result: s.Result{Query: "task", CatalogID: "catalog", IndexVersion: "index", SearchMode: mode, Hybrid: &s.HybridTrace{VectorStatus: tc.vector, InterpretationStatus: tc.parser}}}
		got, err := mapSearch(record)
		if err != nil {
			t.Fatal(err)
		}
		if got.Mode != mode {
			t.Fatal(got.Mode)
		}
		raw, _ := json.Marshal(record)
		var restored d.Record
		if err := json.Unmarshal(raw, &restored); err != nil {
			t.Fatal(err)
		}
		replay, err := mapSearch(restored)
		if err != nil || !reflect.DeepEqual(got, replay) {
			t.Fatal("replay changed status", err)
		}
		if tc.reason != "" && (len(got.Degraded) != 1 || got.Degraded[0] != tc.reason) {
			t.Fatalf("%+v: %+v", tc, got.Degraded)
		}
		if tc.reason == "" && len(got.Degraded) != 0 {
			t.Fatal(got.Degraded)
		}
		if tc.clarify && len(got.Clarification) < 2 {
			t.Fatal("missing parser clarification")
		}
	}
}
