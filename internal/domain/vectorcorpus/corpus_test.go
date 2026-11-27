package vectorcorpus

import (
	embedding "github.com/scmbr/smart-search/internal/domain/embeddingpilot"
	"strings"
	"testing"
)

func example() Corpus {
	return Corpus{Version: Version, TextVersion: embedding.TextVersion, Model: embedding.Model, Dimensions: embedding.Dimensions, SourceSHA: strings.Repeat("a", 64), Offers: 2, Documents: []Document{{ID: "v/group/g", GroupID: "g", Vendor: "v", Text: "product", SourceURL: "https://store.softline.ru/v/product/"}, {ID: "v/offer/s", OfferID: "s", Vendor: "v", Text: "sku", SourceURL: "https://store.softline.ru/v/sku/"}}}
}
func TestCorpusIdentityAndFingerprint(t *testing.T) {
	x := example()
	x.SHA = Fingerprint(x)
	if err := Validate(x); err != nil {
		t.Fatal(err)
	}
	for _, edit := range []func(*Corpus){
		func(x *Corpus) { x.Documents[0].Text = "changed" },
		func(x *Corpus) { x.Model = "other" },
		func(x *Corpus) { x.SourceSHA = "bad" },
		func(x *Corpus) { x.Documents[0].OfferID = "s" },
		func(x *Corpus) { x.Documents[0].Vendor = "other" },
		func(x *Corpus) { x.Documents[0].Text = " " },
		func(x *Corpus) { x.Documents[1] = x.Documents[0] },
	} {
		y := example()
		y.SHA = Fingerprint(y)
		edit(&y)
		if Validate(y) == nil {
			t.Fatalf("accepted %+v", y)
		}
	}
}

func TestRealVendorPathWithUnderscore(t *testing.T) {
	x := example()
	x.Documents = x.Documents[:1]
	x.Documents[0].Vendor = "softlab_nsk"
	x.Documents[0].ID = "softlab_nsk/group/g"
	x.Documents[0].SourceURL = "https://store.softline.ru/softlab_nsk/product/"
	x.SHA = Fingerprint(x)
	if err := Validate(x); err != nil {
		t.Fatal(err)
	}
}
