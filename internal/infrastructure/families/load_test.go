package families

import (
	"encoding/json"
	catalog "github.com/scmbr/smart-search/internal/domain/catalog"
	"os"
	"strings"
	"testing"
)

func TestMappingRetainsReferenceSourcesAndScope(t *testing.T) {
	r, version, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(version, "families-v2-") {
		t.Fatal("missing mapping version")
	}
	var config struct {
		Rules []struct {
			Groups                []string
			SourceURL, VendorPath string
		}
	}
	if err := json.Unmarshal(rules, &config); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile("../../../testdata/evaluation/source-queries.json")
	if err != nil {
		t.Fatal(err)
	}
	var reference struct {
		Cases []struct {
			ExpectedURL string `json:"expected_url"`
		}
	}
	if err := json.Unmarshal(data, &reference); err != nil {
		t.Fatal(err)
	}
	for _, rule := range config.Rules {
		found := false
		for _, row := range reference.Cases {
			if row.ExpectedURL == rule.SourceURL {
				found = true
			}
		}
		if !found {
			t.Fatalf("source URL is not from provided reference: %s", rule.SourceURL)
		}
		f := r.Resolve(catalog.Offer{ID: "a", GroupID: rule.Groups[0], URL: "https://store.softline.ru" + rule.VendorPath + "example/"})
		if !f.Confirmed || f.Source != rule.SourceURL {
			t.Fatalf("rule lost: %+v", f)
		}
	}
}
