package elastic

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"strings"
	"testing"

	app "github.com/scmbr/smart-search/internal/application/lexical"
	catalog "github.com/scmbr/smart-search/internal/domain/catalog"
	domain "github.com/scmbr/smart-search/internal/domain/search"
)

func TestElasticsearchIndexAndSearch(t *testing.T) {
	endpoint := os.Getenv("TEST_ELASTICSEARCH_URL")
	if endpoint == "" {
		t.Skip("TEST_ELASTICSEARCH_URL not set: Elasticsearch integration test")
	}
	ctx := context.Background()
	var nonce [12]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		t.Fatal(err)
	}
	id := hex.EncodeToString(nonce[:])
	c, err := New(endpoint, "", "test-search-"+id)
	if err != nil {
		t.Fatal(err)
	}
	spec := app.NewSpec(app.CatalogInfo{ID: "test-" + id, Offers: 13}, "fixture-v3")
	t.Cleanup(func() {
		m, exists, err := c.meta(context.Background(), spec.Name)
		if err != nil {
			t.Error(err)
			return
		}
		if !exists {
			return
		}
		if m.Spec != spec {
			t.Error("refusing to delete index with foreign metadata")
			return
		}
		if _, _, err := c.request(context.Background(), "DELETE", "/"+spec.Name, nil); err != nil {
			t.Error(err)
		}
	})
	ready, err := c.Prepare(ctx, spec)
	if err != nil {
		t.Fatal(err)
	}
	if ready {
		t.Fatal("unexpected existing random test index")
	}
	var rules []domain.FamilyRule
	for _, group := range []string{"hardware", "software", "csp", "backup", "legacy", "astra", "viewer"} {
		groups := []string{group}
		if group == "backup" {
			groups = append(groups, "backup-advanced")
		}
		rules = append(rules, domain.FamilyRule{ID: group, Name: "Common " + group, Groups: groups, VendorPath: "/v/", SourceURL: "https://example.invalid/" + group})
	}
	families, err := domain.NewFamilies(rules)
	if err != nil {
		t.Fatal(err)
	}
	type fixture struct {
		id, group, name, product, license string
		price                             int64
		available                         bool
	}
	rows := []fixture{
		{"hardware", "hardware", "Common Сервер", "электроника", "", 10000, true},
		{"software", "software", "Common ПО для сервера", "софт", "Полная версия", 1000, true},
		{"csp-4", "csp", "Common КриптоПро CSP 4.0", "софт", "Полная версия", 300, true},
		{"csp-5", "csp", "Common КриптоПро CSP 5.0", "софт", "Полная версия", 600, true},
		{"support", "csp", "Common КриптоПро CSP 5.0", "софт", "Техподдержка", 50, true},
		{"backup", "backup", "Common Backup", "софт", "Полная версия", 700, true},
		{"backup-advanced", "backup-advanced", "Common Backup Advanced", "софт", "Полная версия", 800, true},
		{"legacy", "legacy", "Legacy product", "софт", "Полная версия", 0, false},
		{"astra", "astra", "Операционная система Astra Linux Special Edition для рабочей станции", "софт", "Полная версия", 1000, true},
		{"viewer", "viewer", "Astra Linux viewer", "софт", "Полная версия", 100, true},
		{"suite-std-12", "backup", "Common Backup на 6 пользователей для сервера", "софт", "Полная версия", 1000, true},
		{"suite-std-24", "backup", "Common Backup на 6 пользователей для сервера", "софт", "Полная версия", 100, true},
		{"suite-ent-12", "backup", "Common Backup на 6 пользователей для сервера", "софт", "Полная версия", 50, true},
	}
	var docs []domain.Document
	for _, row := range rows {
		o := catalog.Offer{ID: row.id, GroupID: row.group, Name: row.name, VendorCode: row.id, URL: "https://store.softline.ru/v/" + row.id, CategoryID: "c", Available: row.available, Parameters: []catalog.Parameter{{Name: "Тип продукта", Value: row.product, RawValue: row.product, Known: true}}}
		if row.license != "" {
			o.Parameters = append(o.Parameters, catalog.Parameter{Name: "Тип лицензии", Value: row.license, RawValue: row.license, Known: true})
		}
		if row.price > 0 {
			price := row.price
			o.PriceMinor = &price
		}
		if strings.HasPrefix(row.id, "suite-") {
			months, edition := "12 мес.", "Standard"
			if row.id == "suite-std-24" {
				months = "24 мес."
			}
			if row.id == "suite-ent-12" {
				edition = "Enterprise"
			}
			o.Parameters = append(o.Parameters, catalog.Parameter{Name: "Срок действия", Value: months, Known: true}, catalog.Parameter{Name: "Редакция", Value: edition, Known: true})
		}
		chars := catalog.Normalize(o)
		o.Characteristics = &chars
		d, err := domain.BuildDocument(o, map[string]catalog.Category{"c": {ID: "c", Name: "Catalog"}, "astra": {ID: "astra", Name: "Astra Linux Special Edition"}, "viewer": {ID: "viewer", Name: "Granulex Viewer"}}, families, spec.CatalogID)
		if err != nil {
			t.Fatal(err)
		}
		docs = append(docs, d)
	}
	if err := c.Write(ctx, spec.Name, docs); err != nil {
		t.Fatal(err)
	}
	if err := c.Finish(ctx, spec); err != nil {
		t.Fatal(err)
	}
	if err := c.Activate(ctx, spec); err != nil {
		t.Fatal(err)
	}
	if reused, err := c.Prepare(ctx, spec); err != nil || !reused {
		t.Fatalf("reuse=%v err=%v", reused, err)
	}
	if err := c.Finish(ctx, spec); err != nil {
		t.Fatal("ready finish retry:", err)
	}
	if err := c.Write(ctx, spec.Name, docs[:1]); err == nil {
		t.Fatal("ready index accepted writes")
	}
	for _, tc := range []struct {
		query, policy, want string
		fallback            bool
	}{
		{"сервер", "", "hardware", false}, {"ПО для сервера", "", "software", false},
		{"astra linux", "", "astra", false},
		{"asrta linux", "", "astra", false},
		{"астра линукс", "", "astra", false},
		{"bakcup", "", "backup", false},
		{"legcay", "", "legacy", true},
		{"крипто про цсп 5.0", "", "csp-5", false},
		{"КриптоПро CSP 5.1", "", "", false},
		{"csp-6", "", "", false},
		{"Common Backup редакция Standard на год для сервера на 6 пользователей", "", "suite-std-12", false},
		{"КриптоПро CSP 5.0", "", "csp-5", false}, {"csp-5", "", "csp-5", false},
		{"Legacy", "", "legacy", true}, {"Legacy", "available_only", "", false},
	} {
		t.Run(tc.query+tc.policy, func(t *testing.T) {
			r, err := app.Search(ctx, c, domain.Request{Query: tc.query, Policy: tc.policy, Limit: 4})
			if err != nil {
				t.Fatal(err)
			}
			if tc.want == "" {
				if len(r.Hits) != 0 {
					t.Fatalf("unexpected hits=%+v", r.Hits)
				}
			} else if len(r.Hits) == 0 || r.Hits[0].Document.OfferID != tc.want {
				t.Fatalf("hits=%+v", r.Hits)
			}
			if r.UnpricedFallbackUsed != tc.fallback {
				t.Fatalf("fallback=%v", r.UnpricedFallbackUsed)
			}
		})
	}
	common, err := app.Search(ctx, c, domain.Request{Query: "Common", Limit: 4})
	if err != nil || !common.FourFamiliesConfirmed || len(common.Hits) != 4 {
		t.Fatalf("four families=%+v err=%v", common, err)
	}
	backup, err := app.Search(ctx, c, domain.Request{Query: "Backup", Limit: 4})
	if err != nil || len(backup.Hits) != 1 || backup.Hits[0].Document.FamilyID != "backup" || backup.FourFamiliesConfirmed {
		t.Fatalf("editions in different source groups took separate family slots: %+v err=%v", backup, err)
	}
}
