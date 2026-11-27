package search

import (
	catalog "github.com/scmbr/smart-search/internal/domain/catalog"
	"testing"
)

func TestFamilyResolution(t *testing.T) {
	r, err := NewFamilies([]FamilyRule{{ID: "csp", Name: "CSP", Groups: []string{"g1", "g2"}, VendorPath: "/vendor/", SourceURL: "https://example.invalid/csp"}})
	if err != nil {
		t.Fatal(err)
	}
	a := r.Resolve(catalog.Offer{ID: "a", GroupID: "g1", URL: "https://store.softline.ru/vendor/sku-a/"})
	b := r.Resolve(catalog.Offer{ID: "b", GroupID: "g2", URL: "https://store.softline.ru/vendor/sku-b/"})
	if a.ID != b.ID || !a.Confirmed || !b.Confirmed {
		t.Fatalf("families=%+v %+v", a, b)
	}
	wrong := r.Resolve(catalog.Offer{ID: "a", GroupID: "g1", URL: "https://store.softline.ru/other/a/"})
	if wrong.Confirmed || wrong.ID == a.ID {
		t.Fatal("vendor scope ignored")
	}
	x := r.Resolve(catalog.Offer{ID: "a", URL: "https://store.softline.ru/vendor/a/"})
	y := r.Resolve(catalog.Offer{ID: "b", URL: "https://store.softline.ru/vendor/b/"})
	if x.Confirmed || x.ID == y.ID {
		t.Fatal("ungrouped offers merged")
	}
	if _, err := NewFamilies([]FamilyRule{{ID: "a", Name: "A", Groups: []string{"g"}, VendorPath: "/v/", SourceURL: "https://example.invalid/a"}, {ID: "b", Name: "B", Groups: []string{"g"}, VendorPath: "/v/", SourceURL: "https://example.invalid/b"}}); err == nil {
		t.Fatal("ambiguous mapping accepted")
	}
}

func TestDocumentProjectsOnlyKnownTypedCapacity(t *testing.T) {
	f, err := NewFamilies(nil)
	if err != nil {
		t.Fatal(err)
	}
	c := catalog.Normalize(catalog.Offer{Name: "Product на 2 ПК на 3 ПК на 256 IP до 30 участников"})
	d, err := BuildDocument(catalog.Offer{ID: "sku", Name: "Product", Characteristics: &c}, nil, f, "snapshot")
	if err != nil || d.DevicesLimit != nil || d.IPAddressesLimit == nil || *d.IPAddressesLimit != 256 || d.ParticipantsLimit == nil || *d.ParticipantsLimit != 30 || d.UsersLimit != nil {
		t.Fatalf("projection=%+v err=%v", d, err)
	}
}

func TestRequestRules(t *testing.T) {
	for _, tc := range []struct{ q, version, kind, product string }{
		{"крипто про csp 5.0", "5.0", "", ""}, {"продление продукта", "", "renewal", ""},
		{"сервер", "", "", "hardware"}, {"ПО для сервера", "", "", "software"},
		{"Microsoft 365 12 месяцев", "", "", ""},
	} {
		r, err := Prepare(Request{Query: tc.q, Limit: 4})
		if err != nil || r.Version != tc.version || r.Kind != tc.kind || r.ProductType != tc.product || r.Policy != "priced_first" {
			t.Fatalf("request=%+v err=%v", r, err)
		}
	}
	for _, r := range []Request{{Query: " ", Limit: 4}, {Query: "x", Limit: 3}, {Query: "x", Limit: 4, Policy: "invalid"}, {Query: "КриптоПро CSP 5.0", Limit: 4, Version: "4.0"}} {
		if _, err := Prepare(r); err == nil {
			t.Fatalf("invalid accepted: %+v", r)
		}
	}
}

func TestDocumentPreservesUnknownAndFamily(t *testing.T) {
	r, _ := NewFamilies(nil)
	o := catalog.Offer{ID: "o", GroupID: "g", Name: "License", CategoryID: "c", Available: false, URL: "https://store.softline.ru/v/o/"}
	c := catalog.Normalize(o)
	o.Characteristics = &c
	d, err := BuildDocument(o, map[string]catalog.Category{"c": {ID: "c", ParentID: "p", Name: "Software"}, "p": {ID: "p", Name: "Catalog"}, "g": {ID: "g", Name: "Canonical product"}}, r, "snapshot")
	if err != nil || d.ProductName != "Canonical product" || d.FamilyConfirmed || d.PriceMinor != nil || d.Version != "" || d.UsersLimit != nil || d.Available || len(d.CategoryNames) != 2 {
		t.Fatalf("document=%+v err=%v", d, err)
	}
	if _, err := BuildDocument(o, nil, r, "snapshot"); err == nil {
		t.Fatal("missing category accepted")
	}
}
