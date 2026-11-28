package search

import "testing"

func TestWindowsAliasesAndIntentPreservePlatformContext(t *testing.T) {
	for _, query := range []string{"виндовс", "виндоус", "винда", "Microsoft Windows", "продление виндовс"} {
		r, err := Prepare(Request{Query: query, Limit: 4})
		if err != nil || r.Plan.Purpose != "windows_operating_system" {
			t.Fatalf("%s: %+v %v", query, r.Plan, err)
		}
		found := false
		for _, variant := range r.Plan.Variants {
			for _, term := range variant.Terms {
				if term.Text == "windows" {
					found = true
				}
			}
		}
		if !found {
			t.Fatal("missing Windows canonical form", query)
		}
	}
	for _, query := range []string{"плеер для виндовс", "ПО для Windows", "совместимый с Windows", "не windows"} {
		r, err := Prepare(Request{Query: query, Limit: 4})
		if err != nil {
			t.Fatal(err)
		}
		if r.Plan.Purpose == "windows_operating_system" {
			t.Fatal("platform mention became product intent", query)
		}
	}
}
func TestWindowsRequiresOwnIdentityAndOperatingSystemTaxonomy(t *testing.T) {
	r, _ := Prepare(Request{Query: "виндовс", Limit: 4})
	base := Document{ProductName: "Microsoft Windows 11 Pro", CategoryNames: []string{"Windows 11", "Операционные системы"}, ProductType: "software", URL: "https://store.softline.ru/microsoft/windows/"}
	if !PurposeAllowed(r, base) {
		t.Fatal("Windows OS rejected")
	}
	for _, field := range []string{"category", "identity", "hardware"} {
		doc := base
		switch field {
		case "category":
			doc.CategoryNames = []string{"Мультимедиа"}
			doc.ProductName = "Плеер для Windows"
		case "identity":
			doc.ProductName = "Other OS"
			doc.Description = "Совместимость с Microsoft Windows"
		case "hardware":
			doc.ProductType = "hardware"
		}
		if PurposeAllowed(r, doc) {
			t.Fatal("mention became Windows product", field)
		}
	}
}
