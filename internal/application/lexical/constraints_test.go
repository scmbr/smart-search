package lexical

import (
	"context"
	"testing"

	domain "github.com/scmbr/smart-search/internal/domain/search"
)

func TestUnsupportedConstraintDoesNotIssueAnUnverifiedSearch(t *testing.T) {
	b := &behaviorBackend{}
	r, err := Search(context.Background(), b, domain.Request{Query: "Backup на 2 дня", Limit: 4})
	if err != nil || b.calls != 0 || len(r.Hits) != 0 || r.ShortfallReason != "unsupported_query_constraints" || len(r.UnsupportedConstraints) == 0 {
		t.Fatalf("result=%+v calls=%d err=%v", r, b.calls, err)
	}
}

func TestApplicationRechecksEachCapacityUnit(t *testing.T) {
	for _, unit := range []string{"ПК", "IP", "участников"} {
		for _, scenario := range []string{"known", "unknown", "too small", "different unit"} {
			n, price := 5, int64(100)
			d := domain.Document{OfferID: "sku", FamilyID: "f", CatalogID: "c", PriceMinor: &price}
			if scenario == "too small" {
				n = 4
			}
			if scenario != "unknown" {
				switch unit {
				case "ПК":
					d.DevicesLimit = &n
				case "IP":
					d.IPAddressesLimit = &n
				case "участников":
					d.ParticipantsLimit = &n
				}
			}
			if scenario == "different unit" {
				d.DevicesLimit, d.IPAddressesLimit, d.ParticipantsLimit = nil, nil, nil
				d.UsersLimit = &n
			}
			b := &behaviorBackend{result: domain.Result{CatalogID: "c", IndexVersion: "i", Hits: []domain.Hit{{Document: d}}}}
			_, err := Search(context.Background(), b, domain.Request{Query: "Product на 5 " + unit, Limit: 4})
			if (err == nil) != (scenario == "known") {
				t.Fatalf("%s %s: %v", unit, scenario, err)
			}
		}
	}
}

func TestApplicationRechecksTermEditionAndDeployment(t *testing.T) {
	for _, scenario := range []string{"valid", "wrong edition", "unknown term", "wrong term", "unknown deployment"} {
		t.Run(scenario, func(t *testing.T) {
			price := int64(100)
			months := 12
			doc := domain.Document{OfferID: "sku", FamilyID: "family", CatalogID: "catalog", OfferKind: "new_license", Edition: "standard", TermKind: "fixed", TermMonths: &months, Deployment: "server", PriceMinor: &price}
			switch scenario {
			case "wrong edition":
				doc.Edition = "enterprise"
			case "unknown term":
				doc.TermMonths = nil
			case "wrong term":
				months = 24
			case "unknown deployment":
				doc.Deployment = ""
			}
			b := &behaviorBackend{result: domain.Result{CatalogID: "catalog", IndexVersion: "index", Hits: []domain.Hit{{Document: doc}}}}
			_, err := Search(context.Background(), b, domain.Request{Query: "Product редакция Standard на год для сервера", Limit: 4})
			if (err == nil) != (scenario == "valid") {
				t.Fatalf("%s: err=%v", scenario, err)
			}
		})
	}
}
