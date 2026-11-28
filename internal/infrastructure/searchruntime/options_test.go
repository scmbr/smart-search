package searchruntime

import "testing"

func TestOptionsRejectImplicitPaidModes(t *testing.T) {
	for _, o := range []Options{{ParserMode: "bad"}, {LiveQuery: true, ParserMode: "off"}, {ParserMode: "replay"}, {Corpus: "file", ParserMode: "live"}} {
		if o.Validate() == nil {
			t.Fatal(o)
		}
	}
	for _, o := range []Options{{ParserMode: "off"}, {Corpus: "file", ParserMode: "replay"}, {Corpus: "file", LiveQuery: true, ParserMode: "live"}} {
		if err := o.Validate(); err != nil {
			t.Fatal(o, err)
		}
	}
}
