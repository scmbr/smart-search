package search

import "testing"

func TestIdentityRankingProtectsNamedProductFromSemanticConsensus(t *testing.T) {
	r, err := Prepare(Request{Query: "Vector Studio", Limit: 4})
	if err != nil {
		t.Fatal(err)
	}
	a := fusionHit("software", "a")
	a.Document.ProductName = "Vector Studio Professional"
	b := fusionHit("hardware", "b")
	b.Document.ProductName = "Document scanner"
	got, count, err := FuseRequest(r, []Hit{a, b}, []Hit{b}, 4)
	if err != nil || count != 1 || len(got) != 2 || got[0].Document.OfferID != "a" {
		t.Fatalf("hits=%+v anchors=%d err=%v", got, count, err)
	}
}

func TestIdentityRankingDoesNotTrustDescriptionOrPartialWords(t *testing.T) {
	r, _ := Prepare(Request{Query: "Vector Studio", Limit: 4})
	a := fusionHit("software", "a")
	a.Document.ProductName = "Vector Studios"
	a.Document.Description = "Vector Studio"
	b := fusionHit("other", "b")
	got, count, err := FuseRequest(r, []Hit{a, b}, []Hit{b}, 4)
	if err != nil || count != 0 || got[0].Document.OfferID != "b" {
		t.Fatalf("%+v %d %v", got, count, err)
	}
}

func TestIdentityRankingTransliterationAndSingleTokenBoundary(t *testing.T) {
	for _, tc := range []struct {
		query, name string
		want        int
	}{
		{"skaner vs", "Комплекс Сканер-ВС", 1},
		{"Сканер-ВС", "Комплекс Сканер-ВС", 1},
		{"AnyDesk", "AnyDesk", 1},
		{"backup", "Backup suite", 0},
		{"для резервного копирования", "Backup suite", 0},
	} {
		t.Run(tc.query, func(t *testing.T) {
			r, _ := Prepare(Request{Query: tc.query, Limit: 4})
			a := fusionHit("a", "a")
			a.Document.ProductName = tc.name
			_, count, err := FuseRequest(r, []Hit{a}, nil, 4)
			if err != nil || count != tc.want {
				t.Fatalf("count=%d err=%v", count, err)
			}
		})
	}
}
