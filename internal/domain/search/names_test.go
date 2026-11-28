package search

import "testing"

func TestCatalogNamesResolveAndPreserveAmbiguity(t *testing.T) {
	d, err := NewNameDictionary("snapshot", []NameEntity{{Kind: "brand", Name: "Adobe", Scope: "/adobe/"}, {Kind: "product", Name: "Adobe Photoshop", Scope: "/adobe/"}, {Kind: "product", Name: "ФотоШОУ PRO", Scope: "/ams/"}}, []NameAlias{{Alias: "адоб", Name: "Adobe", Kind: "brand"}, {Alias: "фотошоп", Name: "Adobe Photoshop", Kind: "product"}, {Alias: "Photoshop", Name: "Adobe Photoshop", Kind: "product"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{"Adobe", "адоб", "адоь", "adboe"} {
		r := d.ResolveName(q)
		if r.Status != "resolved" || r.Candidates[0].Entity.Name != "Adobe" {
			t.Fatalf("%s: %+v", q, r)
		}
	}
	for _, q := range []string{"Photoshop", "фотошоп", "фотошор", "phtoshop"} {
		r := d.ResolveName(q)
		if r.Status != "resolved" || r.Candidates[0].Entity.Name != "Adobe Photoshop" {
			t.Fatalf("%s: %+v", q, r)
		}
	}
	for _, q := range []string{"FQC-10572", "Photoshop 2025", "нужно обрабатывать фотографии", "аб"} {
		if r := d.ResolveName(q); r.Status != "unresolved" {
			t.Fatalf("unsafe correction %s: %+v", q, r)
		}
	}
	if d.Fingerprint() == "" {
		t.Fatal("missing identity")
	}
	other, _ := NewNameDictionary("other", []NameEntity{{Kind: "brand", Name: "Adobe", Scope: "/adobe/"}}, nil)
	if d.Fingerprint() == other.Fingerprint() {
		t.Fatal("snapshot not pinned")
	}
	ambiguous, _ := NewNameDictionary("s", []NameEntity{{Kind: "product", Name: "paint", Scope: "/a/"}, {Kind: "product", Name: "point", Scope: "/b/"}}, nil)
	if r := ambiguous.ResolveName("peint"); r.Status != "ambiguous" || len(r.Candidates) != 2 {
		t.Fatalf("%+v", r)
	}
}

func TestNameAliasRequiresCatalogTarget(t *testing.T) {
	d, err := NewNameDictionary("s", []NameEntity{{Kind: "brand", Name: "Other", Scope: "/other/"}}, []NameAlias{{Alias: "адоб", Name: "Adobe", Kind: "brand"}})
	if err != nil {
		t.Fatal(err)
	}
	if r := d.ResolveName("адоб"); r.Status != "unresolved" {
		t.Fatal(r)
	}
}
