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

func TestReviewedEditionsShareFamilyAndDistinctProductsStaySeparate(t *testing.T) {
	r, _, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, tc := range []struct {
		id, vendor string
		groups     []string
	}{
		{"microsoft-office", "microsoft", []string{"391338", "391335", "161788", "161807", "162056", "24300", "284105", "3170"}},
		{"microsoft-365", "microsoft", []string{"165034", "283494"}},
		{"microsoft-visio", "microsoft", []string{"391347", "391350", "102094", "24317", "24321"}},
		{"microsoft-project", "microsoft", []string{"391341", "391344"}},
		{"microsoft-word", "microsoft", []string{"162037", "162743"}},
		{"microsoft-windows-desktop", "microsoft", []string{"3564", "65851", "286676", "310176", "295882", "24361"}},
		{"microsoft-windows-server", "microsoft", []string{"161826", "161664", "281974", "294539", "161851", "24309", "228208", "281954"}},
		{"microsoft-sql-server", "microsoft", []string{"1820", "1870", "1940", "24289", "24294", "103140"}},
		{"kaspersky-endpoint-security-business", "kaspersky", []string{"3935", "3953", "419620"}},
		{"kaspersky-small-office-security", "kaspersky", []string{"3761", "379232"}},
		{"mts-link", "webinar", []string{"414326", "447242", "447243"}},
		{"xspider", "positive-technologies", []string{"28574", "428013"}},
		{"cryptopro-ca", "kripto-pro", []string{"385311", "385387", "385356"}},
	} {
		if seen[tc.id] {
			t.Fatal("duplicate family fixture")
		}
		seen[tc.id] = true
		for _, group := range tc.groups {
			f := r.Resolve(catalog.Offer{ID: "fixture", GroupID: group, URL: "https://store.softline.ru/" + tc.vendor + "/fixture/"})
			if !f.Confirmed || f.ID != tc.id {
				t.Fatalf("group %s should share %s: %+v", group, tc.id, f)
			}
			for _, url := range []string{"https://example.invalid/" + tc.vendor + "/fixture/", "https://store.softline.ru/wrong/fixture/"} {
				if f := r.Resolve(catalog.Offer{ID: "fixture", GroupID: group, URL: url}); f.Confirmed {
					t.Fatal("vendor scope bypassed")
				}
			}
		}
	}
	for _, tc := range []struct{ group, vendor string }{{"442025", "cyberprotect"}, {"444870", "cyberprotect"}, {"393657", "astralinux"}, {"446661", "astralinux"}, {"401458", "astralinux"}, {"385280", "kripto-pro"}, {"65743", "microsoft"}, {"unseen", "microsoft"}} {
		f := r.Resolve(catalog.Offer{ID: "fixture", GroupID: tc.group, Name: "Microsoft Office Кибер Бэкап", URL: "https://store.softline.ru/" + tc.vendor + "/fixture/"})
		if f.Confirmed {
			t.Fatalf("unreviewed neighbor guessed: %s", tc.group)
		}
	}
}
