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
	if !strings.HasPrefix(version, "families-v5-") {
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
	data, err = os.ReadFile("../../../testdata/evaluation/family-evidence.json")
	if err != nil {
		t.Fatal(err)
	}
	var evidence struct {
		Groups map[string]struct {
			Examples []struct {
				URL string `json:"url"`
			} `json:"examples"`
		} `json:"groups"`
	}
	if err := json.Unmarshal(data, &evidence); err != nil {
		t.Fatal(err)
	}
	sources := map[string]bool{}
	for _, rule := range config.Rules {
		found := false
		for _, row := range reference.Cases {
			if row.ExpectedURL == rule.SourceURL {
				found = true
			}
		}
		if !found {
			for _, group := range rule.Groups {
				for _, sample := range evidence.Groups[group].Examples {
					found = found || sample.URL == rule.SourceURL
				}
			}
		}
		if !found {
			t.Fatalf("source URL is not from reference or frozen SKU evidence: %s", rule.SourceURL)
		}
		sources[rule.SourceURL] = true
		f := r.Resolve(catalog.Offer{ID: "a", GroupID: rule.Groups[0], URL: "https://store.softline.ru" + rule.VendorPath + "example/"})
		if !f.Confirmed || f.Source != rule.SourceURL {
			t.Fatalf("rule lost: %+v", f)
		}
	}
	for _, row := range reference.Cases {
		if !sources[row.ExpectedURL] {
			t.Fatalf("reference source lost: %s", row.ExpectedURL)
		}
	}
}
