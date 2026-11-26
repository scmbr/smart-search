package search

import "testing"

func TestFamilyIdentityCannotHaveConflictingDefinitions(t *testing.T) {
	base := FamilyRule{ID: "backup", Name: "Backup", Groups: []string{"standard"}, VendorPath: "/v/", SourceURL: "https://example.invalid/backup"}
	for _, field := range []string{"name", "vendor", "source"} {
		t.Run(field, func(t *testing.T) {
			other := base
			other.Groups = []string{"advanced"}
			switch field {
			case "name":
				other.Name = "Different product"
			case "vendor":
				other.VendorPath = "/other/"
			case "source":
				other.SourceURL = "https://example.invalid/other"
			}
			if _, err := NewFamilies([]FamilyRule{base, other}); err == nil {
				t.Fatal("conflicting definitions would silently collapse different products")
			}
		})
	}
	other := base
	other.Groups = []string{"advanced"}
	if _, err := NewFamilies([]FamilyRule{base, other}); err != nil {
		t.Fatal("consistent cross-group family rejected:", err)
	}
}
