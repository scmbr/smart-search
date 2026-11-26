package families

import (
	"encoding/json"
	"os"
	"testing"

	catalog "github.com/scmbr/smart-search/internal/domain/catalog"
	domain "github.com/scmbr/smart-search/internal/domain/search"
)

func TestEveryMappedGroupHasFrozenCatalogEvidence(t *testing.T) {
	data, err := os.ReadFile("../../../testdata/evaluation/family-evidence.json")
	if err != nil {
		t.Fatal(err)
	}
	var evidence struct {
		Role    string `json:"evidence_role"`
		SHA     string `json:"source_sha256"`
		Version string `json:"mapping_version"`
		Groups  map[string]struct {
			Family     string `json:"family_id"`
			Name       string `json:"group_name"`
			Source     string `json:"source_url"`
			Vendor     string `json:"vendor_path"`
			Offers     int    `json:"offers"`
			Mismatches int    `json:"scope_mismatches"`
			Examples   []struct {
				ID   string `json:"offer_id"`
				Name string `json:"name"`
				URL  string `json:"url"`
			} `json:"examples"`
		} `json:"groups"`
	}
	if err := json.Unmarshal(data, &evidence); err != nil {
		t.Fatal(err)
	}
	var config struct {
		Version string
		Rules   []domain.FamilyRule
	}
	if err := json.Unmarshal(rules, &config); err != nil {
		t.Fatal(err)
	}
	if evidence.Role != "catalog_structure_not_independent_relevance_review" || len(evidence.SHA) != 64 || evidence.Version != config.Version {
		t.Fatal("evidence provenance/version missing or wrong review claim")
	}
	resolver, _, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, rule := range config.Rules {
		for _, group := range rule.Groups {
			count++
			row, ok := evidence.Groups[group]
			if !ok || row.Family != rule.ID || row.Name == "" || row.Source != rule.SourceURL || row.Vendor != rule.VendorPath || row.Offers < 1 || row.Mismatches != 0 || len(row.Examples) == 0 {
				t.Fatalf("missing/stale catalog evidence for group %s", group)
			}
			for _, sample := range row.Examples {
				family := resolver.Resolve(catalog.Offer{ID: sample.ID, Name: sample.Name, GroupID: group, URL: sample.URL})
				if sample.ID == "" || sample.Name == "" || !family.Confirmed || family.ID != row.Family {
					t.Fatalf("catalog example not resolved: %s/%s", group, sample.ID)
				}
			}
		}
	}
	if count != len(evidence.Groups) {
		t.Fatal("orphan evidence groups")
	}
}

func TestBackupEditionsShareFamilyWithoutMergingCloud(t *testing.T) {
	r, _, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, group := range []string{"173614", "173615"} {
		f := r.Resolve(catalog.Offer{ID: "fixture", GroupID: group, URL: "https://store.softline.ru/cyberprotect/fixture/"})
		if !f.Confirmed || f.ID != "cyber-backup" {
			t.Fatalf("edition counted separately: %+v", f)
		}
	}
	f := r.Resolve(catalog.Offer{ID: "fixture", GroupID: "444870", URL: "https://store.softline.ru/cyberprotect/fixture/"})
	if f.Confirmed {
		t.Fatal("unreviewed cloud group guessed from vendor")
	}
}
