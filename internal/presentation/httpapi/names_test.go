package httpapi

import (
	"encoding/json"
	s "github.com/scmbr/smart-search/internal/domain/search"
	d "github.com/scmbr/smart-search/internal/domain/searchsession"
	"reflect"
	"testing"
)

func TestNameFeedbackSurvivesReplay(t *testing.T) {
	for _, tc := range []struct {
		input, status string
		distance      int
		correction    bool
	}{{"фотошор", "resolved", 10, true}, {"фотошоп", "resolved", 0, true}, {"Adobe Photoshop", "resolved", 0, false}, {"peint", "ambiguous", 10, false}} {
		record := d.Record{ID: "search", Result: s.Result{Query: tc.input, NameInput: tc.input, CatalogID: "catalog", IndexVersion: "index", NameResolution: &s.NameResolution{Status: tc.status, DictionarySHA: "dictionary", Candidates: []s.NameCandidate{{Entity: s.NameEntity{Name: "Adobe Photoshop"}, Distance: tc.distance}}}}}
		got, err := mapSearch(record)
		if err != nil {
			t.Fatal(err)
		}
		if (len(got.Corrections) > 0) != tc.correction {
			t.Fatalf("%+v %+v", tc, got.Corrections)
		}
		if tc.status == "ambiguous" && (got.Total != nil || len(got.Clarification) == 0 || got.Shortfall != nil) {
			t.Fatal("ambiguity misreported as zero results", got)
		}
		raw, _ := json.Marshal(record)
		var restored d.Record
		if err = json.Unmarshal(raw, &restored); err != nil {
			t.Fatal(err)
		}
		replay, err := mapSearch(restored)
		if err != nil || !reflect.DeepEqual(got, replay) {
			t.Fatal("name feedback lost on replay", err)
		}
	}
}
