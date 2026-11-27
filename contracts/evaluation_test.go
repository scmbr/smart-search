package contracts_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"slices"
	"strings"
	"testing"
)

type evaluationFamily struct {
	Name   string   `json:"name"`
	Brand  string   `json:"brand_id"`
	Groups []string `json:"group_ids"`
}

type evaluationCase struct {
	ID, Split, Query string
	Brand            string   `json:"brand_id"`
	Scope            string   `json:"target_scope"`
	Targets          []string `json:"target_ids"`
	Groups           []string `json:"group_ids"`
	Tags             []string `json:"tags"`
	Exhaustive       bool     `json:"targets_exhaustive"`
	Behavior         string   `json:"expected_behavior"`
	Review           string   `json:"review_status"`
	Constraints      struct {
		Kind    string   `json:"offer_kind"`
		Exclude []string `json:"exclude_offer_kinds"`
	} `json:"constraints"`
}

func frozenHash(t *testing.T, raw json.RawMessage, want string) {
	t.Helper()
	var compact bytes.Buffer
	if err := json.Compact(&compact, raw); err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(compact.Bytes())
	if hex.EncodeToString(hash[:]) != want {
		t.Fatal("frozen targets/cases changed: review revision and exposure before updating hash")
	}
}

func TestHundredEvaluationCasesAreGroundedAndSeparated(t *testing.T) {
	ids, queries := map[string]bool{}, map[string]bool{}
	brandSplit, familySplit, groupSplit := map[string]string{}, map[string]string{}, map[string]string{}
	holdoutGroups := map[string]bool{}
	for _, suite := range []struct {
		file, split, role string
		count             int
	}{
		{"scenarios.json", "tuning", "development_tuning", 60},
		{"holdout-v2.json", "holdout", "frozen_holdout_user_reviewed", 40},
	} {
		var dataset struct {
			Version      string `json:"dataset_version"`
			Role         string `json:"dataset_role"`
			SHA          string `json:"source_sha256"`
			CaseSHA      string `json:"cases_sha256"`
			FamilySHA    string `json:"families_sha256"`
			ReviewRecord struct {
				Reviewer string `json:"reviewed_by"`
				Date     string `json:"reviewed_on"`
				Scope    string `json:"scope"`
			} `json:"review_record"`
			Review struct {
				Eligible  bool   `json:"eligible"`
				Status    string `json:"review_status"`
				Execution string `json:"execution_status"`
			} `json:"independent_evaluation"`
			Evidence map[string]struct {
				Name    string `json:"group_name"`
				Offers  int    `json:"offers"`
				Example string `json:"example_offer_id"`
				URL     string `json:"example_url"`
			} `json:"catalog_evidence"`
			Families json.RawMessage `json:"families"`
			Cases    json.RawMessage `json:"cases"`
		}
		readJSON(t, "../testdata/evaluation/"+suite.file, &dataset)
		if dataset.Version != "evaluation-v4-reviewed" || dataset.Role != suite.role || len(dataset.SHA) != 64 || dataset.Review.Eligible != (suite.split == "holdout") || dataset.Review.Status != "accepted_by_user" || dataset.ReviewRecord.Reviewer != "repository_owner" || dataset.ReviewRecord.Date != "2026-11-27" || dataset.ReviewRecord.Scope != "all_100_active_cases" {
			t.Fatalf("%s: user review/source provenance missing or tuning claimed independent", suite.file)
		}
		if suite.split == "holdout" && dataset.Review.Execution != "not_run" {
			t.Fatal("holdout was used during development")
		}
		frozenHash(t, dataset.Cases, dataset.CaseSHA)
		frozenHash(t, dataset.Families, dataset.FamilySHA)
		var cases []evaluationCase
		var families map[string]evaluationFamily
		if err := json.Unmarshal(dataset.Cases, &cases); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(dataset.Families, &families); err != nil {
			t.Fatal(err)
		}
		if len(cases) != suite.count {
			t.Fatalf("%s: got %d cases, want %d", suite.split, len(cases), suite.count)
		}
		usedFamilies, tags := map[string]bool{}, map[string]bool{}
		for _, c := range cases {
			query := strings.ToLower(strings.Join(strings.Fields(c.Query), " "))
			if c.ID == "" || ids[c.ID] || query == "" || queries[query] || c.Split != suite.split || c.Brand == "" || c.Review != "accepted_by_user" {
				t.Fatalf("incomplete/duplicate/wrong-split case: %s", c.ID)
			}
			ids[c.ID], queries[query] = true, true
			if previous := brandSplit[c.Brand]; previous != "" && previous != suite.split {
				t.Fatalf("brand leaks across splits: %s", c.Brand)
			}
			brandSplit[c.Brand] = suite.split
			if len(c.Tags) == 0 {
				t.Fatalf("missing scenario type: %s", c.ID)
			}
			for _, tag := range c.Tags {
				tags[tag] = true
			}
			if c.Constraints.Kind != "" && slices.Contains(c.Constraints.Exclude, c.Constraints.Kind) {
				t.Fatalf("contradictory license constraints: %s", c.ID)
			}
			if c.Scope == "empty" {
				if len(c.Targets) != 0 || len(c.Groups) != 0 || c.Behavior != "empty_without_random_fill" {
					t.Fatalf("negative case fabricates target: %s", c.ID)
				}
				continue
			}
			if c.Scope != "family" && c.Scope != "brand" {
				t.Fatalf("unknown target scope: %s", c.ID)
			}
			if c.Scope == "brand" && (c.Exhaustive || c.Behavior != "brand_browse_or_clarify" || !slices.Contains(c.Tags, "brand")) {
				t.Fatalf("broad brand incorrectly graded as exact family: %s", c.ID)
			}
			if len(c.Targets) == 0 || len(c.Groups) == 0 {
				t.Fatalf("missing targets: %s", c.ID)
			}
			allowedGroups := map[string]bool{}
			for _, target := range c.Targets {
				family, ok := families[target]
				if !ok || family.Name == "" || family.Brand != c.Brand || len(family.Groups) == 0 {
					t.Fatalf("wrong family/brand: %s/%s", c.ID, target)
				}
				usedFamilies[target] = true
				if previous := familySplit[target]; previous != "" && previous != suite.split {
					t.Fatalf("family leaks: %s", target)
				}
				familySplit[target] = suite.split
				for _, group := range family.Groups {
					allowedGroups[group] = true
				}
			}
			for _, group := range c.Groups {
				e, ok := dataset.Evidence[group]
				u, err := url.Parse(e.URL)
				if !ok || !allowedGroups[group] || e.Name == "" || e.Offers < 1 || e.Example == "" || err != nil || u.Scheme != "https" || u.Host != "store.softline.ru" || !strings.HasPrefix(u.Path, "/"+c.Brand+"/") {
					t.Fatalf("missing catalog evidence or wrong vendor scope: %s/%s", c.ID, group)
				}
				if previous := groupSplit[group]; previous != "" && previous != suite.split {
					t.Fatalf("group leaks: %s", group)
				}
				if slices.Contains(c.Tags, "exact_sku") && c.Query != e.Example {
					t.Fatalf("exact SKU query differs from catalog example: %s", c.ID)
				}
				groupSplit[group] = suite.split
				if suite.split == "holdout" {
					holdoutGroups[group] = true
				}
			}
		}
		if len(usedFamilies) != len(families) {
			t.Fatal("unused family definitions")
		}
		for _, required := range []string{"exact", "typo", "transliteration", "brand", "task_intent", "license_constraint", "slang", "exact_sku"} {
			if !tags[required] {
				t.Fatalf("%s lacks %s coverage", suite.split, required)
			}
		}
	}
	if len(ids) != 100 {
		t.Fatalf("expected exactly 100 active cases, got %d", len(ids))
	}
	// Source references and historical regression have already been exposed.
	for _, file := range []string{"source-queries.json", "regression-v1.json"} {
		var exposed struct {
			Cases []struct {
				Groups []string `json:"group_ids"`
				URL    string   `json:"expected_url"`
				Vendor string   `json:"vendor_path"`
			} `json:"cases"`
		}
		readJSON(t, "../testdata/evaluation/"+file, &exposed)
		for _, c := range exposed.Cases {
			for _, g := range c.Groups {
				if holdoutGroups[g] {
					t.Fatalf("holdout group already exposed: %s", g)
				}
			}
			path := c.Vendor
			if c.URL != "" {
				u, err := url.Parse(c.URL)
				if err != nil {
					t.Fatal(err)
				}
				path = u.Path
			}
			brand := strings.Split(strings.Trim(path, "/"), "/")[0]
			if brand != "" && brandSplit[brand] == "holdout" {
				t.Fatalf("holdout brand already exposed: %s", brand)
			}
		}
	}
}
