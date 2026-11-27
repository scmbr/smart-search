package contracts_test

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Test-only representation of the small fixture, not the production importer.
type catalogFixture struct {
	Shop struct {
		Categories []struct {
			ID     string `xml:"id,attr"`
			Parent string `xml:"parentId,attr"`
		} `xml:"categories>category"`
		Offers []struct {
			ID        string `xml:"id,attr"`
			Group     string `xml:"group_id,attr"`
			Available bool   `xml:"available,attr"`
			Name      string `xml:"name"`
			Category  string `xml:"categoryId"`
			URL       string `xml:"url"`
			Price     string `xml:"price"`
			Params    []struct {
				Name  string `xml:"name,attr"`
				Value string `xml:",chardata"`
			} `xml:"param"`
		} `xml:"offers>offer"`
	} `xml:"shop"`
}

func TestCatalogFixtureCoverage(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "testdata", "catalog", "minimal.xml"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture catalogFixture
	if err := xml.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	categories := make(map[string]bool)
	for _, c := range fixture.Shop.Categories {
		categories[c.ID] = true
	}
	for _, c := range fixture.Shop.Categories {
		if c.Parent != "" && !categories[c.Parent] {
			t.Errorf("unknown parent %s", c.Parent)
		}
	}
	ids := make(map[string]bool)
	roles := make(map[string]bool)
	var unavailableZero, repeatedParameter, hardware, serverSoftware bool
	for _, offer := range fixture.Shop.Offers {
		if ids[offer.ID] {
			t.Errorf("duplicate ID in valid fixture: %s", offer.ID)
		}
		ids[offer.ID] = true
		if !categories[offer.Category] {
			t.Errorf("unknown category: %s", offer.Category)
		}
		if !strings.HasPrefix(offer.URL, "https://example.invalid/") {
			t.Errorf("fixture contains real URL: %s", offer.URL)
		}
		if offer.Available && offer.Price == "0.00" {
			t.Errorf("available offer with zero price: %s", offer.ID)
		}
		unavailableZero = unavailableZero || !offer.Available && offer.Price == "0.00"
		seen := make(map[string]bool)
		for _, p := range offer.Params {
			repeatedParameter = repeatedParameter || seen[p.Name]
			seen[p.Name] = true
			if offer.Group == "os-product" && p.Name == "Тип лицензии" {
				roles[p.Value] = true
			}
		}
		hardware = hardware || offer.ID == "server-hardware" && offer.Category == "servers"
		serverSoftware = serverSoftware || offer.ID == "server-software" && offer.Category == "os"
	}
	if !unavailableZero || !repeatedParameter || !hardware || !serverSoftware {
		t.Fatal("fixture lost a required availability/parameter/intent edge case")
	}
	for _, role := range []string{"Полная версия", "Продление", "Техническая поддержка"} {
		if !roles[role] {
			t.Errorf("same-product role edge case missing: %s", role)
		}
	}
}

func TestArchivedEvaluationDoesNotClaimExposedCasesAsHoldout(t *testing.T) {
	var fixture struct {
		Role   string `json:"dataset_role"`
		Review struct {
			Eligible    bool   `json:"eligible"`
			Status      string `json:"review_status"`
			Replacement string `json:"replacement_holdout_status"`
		} `json:"independent_evaluation"`
		Cases []struct{ ID, Split, Family, Query string } `json:"cases"`
	}
	readJSON(t, filepath.Join("..", "testdata", "evaluation", "regression-v1.json"), &fixture)
	if len(fixture.Cases) != 10 {
		t.Fatal("historical ten-case regression archive was overwritten")
	}
	if fixture.Role != "development_regression_not_holdout" || fixture.Review.Eligible || fixture.Review.Status != "pending" || fixture.Review.Replacement != "frozen_draft_pending_review" {
		t.Fatal("exposed development data must not be presented as reviewed independent evaluation")
	}
	ids := make(map[string]bool)
	families := make(map[string]string)
	splits := make(map[string]int)
	for _, c := range fixture.Cases {
		if c.ID == "" || c.Query == "" || c.Family == "" {
			t.Fatal("incomplete evaluation case")
		}
		if ids[c.ID] {
			t.Errorf("duplicate scenario ID: %s", c.ID)
		}
		ids[c.ID] = true
		if c.Split != "tuning" && c.Split != "regression" {
			t.Errorf("unknown split: %s", c.Split)
		}
		if previous, ok := families[c.Family]; ok && previous != c.Split {
			t.Errorf("family %s leaks from %s into %s", c.Family, previous, c.Split)
		}
		families[c.Family] = c.Split
		splits[c.Split]++
	}
	if splits["tuning"] == 0 || splits["regression"] == 0 {
		t.Fatal("both development sets must contain cases")
	}
}

func TestSourceQueryProvenance(t *testing.T) {
	var source struct {
		Source string `json:"source_file"`
		Role   string `json:"dataset_role"`
		Cases  []struct {
			ID       string   `json:"id"`
			Row      int      `json:"source_row"`
			Query    string   `json:"query"`
			URL      string   `json:"expected_url"`
			Kind     string   `json:"target_kind"`
			Groups   []string `json:"group_ids"`
			Vendor   string   `json:"vendor_path"`
			Category string   `json:"category_id"`
		} `json:"cases"`
	}
	readJSON(t, filepath.Join("..", "testdata", "evaluation", "source-queries.json"), &source)
	if source.Source == "" || source.Role != "reference_smoke_not_holdout" {
		t.Fatal("source provenance missing or mislabeled as independent evaluation")
	}
	rows := make(map[int]bool)
	for _, c := range source.Cases {
		if rows[c.Row] {
			t.Errorf("duplicate source row: %d", c.Row)
		}
		rows[c.Row] = true
		if c.Query == "" || !strings.HasPrefix(c.URL, "https://store.softline.ru/") {
			t.Errorf("invalid source case: %s", c.ID)
		}
		switch c.Kind {
		case "product_group":
			if len(c.Groups) == 0 {
				t.Errorf("missing product mapping: %s", c.ID)
			}
		case "vendor_category":
			if c.Vendor == "" || c.Category == "" {
				t.Errorf("missing vendor/category mapping: %s", c.ID)
			}
		case "vendor_page":
			if c.Vendor == "" {
				t.Errorf("missing vendor mapping: %s", c.ID)
			}
		default:
			t.Errorf("unknown target kind: %s", c.Kind)
		}
	}
	for row := 2; row <= 11; row++ {
		if !rows[row] {
			t.Errorf("source row %d was not transferred", row)
		}
	}
}
