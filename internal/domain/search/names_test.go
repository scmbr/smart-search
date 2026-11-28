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

func TestSameVendorBrandAndProductAreNotAmbiguous(t *testing.T) {
	d, err := NewNameDictionary("s", []NameEntity{{Kind: "brand", Name: "AnyDesk", Scope: "/anydesk/"}, {Kind: "product", Name: "AnyDesk", Scope: "/anydesk/"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if r := d.ResolveName("AnyDesk"); r.Status != "resolved" || r.Candidates[0].Entity.Kind != "product" {
		t.Fatalf("%+v", r)
	}
}
func TestRussianSpellingOfLatinBrandAlias(t *testing.T) {
	d, err := NewNameDictionary("s", []NameEntity{{Kind: "brand", Name: "Corel Corporation", Scope: "/corel/"}, {Kind: "product", Name: "Qlik Core", Scope: "/qlik/"}}, []NameAlias{{Alias: "Corel", Name: "Corel Corporation", Kind: "brand"}, {Alias: "Core", Name: "Qlik Core", Kind: "product"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{"Corel", "корел"} {
		r := d.ResolveName(q)
		if r.Status != "resolved" || r.Candidates[0].Entity.Scope != "/corel/" {
			t.Fatalf("%s %+v", q, r)
		}
	}
}

func TestShortOrdinaryWordsAreNotConfidentlyReplacedByOtherVendors(t *testing.T) {
	d, err := NewNameDictionary("s", []NameEntity{{Kind: "brand", Name: "SAINT Corporation", Scope: "/saint/"}, {Kind: "brand", Name: "Joint", Scope: "/joint/"}}, []NameAlias{{Alias: "saint", Name: "SAINT Corporation", Kind: "brand"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{"paint", "point"} {
		if r := d.ResolveName(q); r.Status != "unresolved" {
			t.Fatalf("%s: %+v", q, r)
		}
	}
}
func TestVendorPathNeedsNameCorroboration(t *testing.T) {
	if VendorPathAlias(NameEntity{Kind: "brand", Name: "Corel Corporation", Scope: "/corel/"}) != "corel" {
		t.Fatal("missing catalog brand alias")
	}
	if VendorPathAlias(NameEntity{Kind: "brand", Name: "Группа Астра", Scope: "/astralinux/"}) != "" {
		t.Fatal("product path became owner alias")
	}
	if nameDistance("astra linux", "astralinux") < 1000 {
		t.Fatal("word boundaries collapsed")
	}
}

func TestLongNameCanCorrectFirstLetter(t *testing.T) {
	dictionary, err := NewNameDictionary("snapshot", []NameEntity{{Kind: "product", Name: "AnyDesk", Scope: "/anydesk/"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if r := dictionary.ResolveName("bnydesk"); r.Status != "resolved" || r.Candidates[0].Entity.Name != "AnyDesk" {
		t.Fatalf("%+v", r)
	}
}

func TestEquivalentNamesHaveDeterministicTieBreak(t *testing.T) {
	dictionary, err := NewNameDictionary("s", []NameEntity{{Kind: "product", Name: "AnyDesk", Scope: "/anydesk/"}, {Kind: "product", Name: "ANYDESK", Scope: "/anydesk/"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 100; i++ {
		r := dictionary.ResolveName("anydesk")
		if r.Status != "resolved" || r.Candidates[0].Entity.Name != "ANYDESK" {
			t.Fatalf("unstable equivalent identity: %+v", r)
		}
	}
}
