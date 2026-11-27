package main

import "testing"

func TestParserModeRequiresCompleteCorpusAndExplicitLiveQuery(t *testing.T) {
	for _, tc := range []struct {
		mode, corpus string
		live, batch  bool
		valid        bool
	}{
		{mode: "off", valid: true},
		{mode: "replay", corpus: "manifest", batch: true, valid: true},
		{mode: "live", corpus: "manifest", live: true, valid: true},
		{mode: "replay"}, {mode: "unknown", corpus: "manifest"}, {mode: "live", corpus: "manifest"},
		{mode: "live", corpus: "manifest", live: true, batch: true},
	} {
		if (validateParserMode(tc.mode, tc.corpus, tc.live, tc.batch) == nil) != tc.valid {
			t.Fatalf("%+v", tc)
		}
	}
}
