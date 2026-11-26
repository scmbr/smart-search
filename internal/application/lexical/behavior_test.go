package lexical

import (
	"context"
	"errors"
	"testing"

	domain "github.com/scmbr/smart-search/internal/domain/search"
)

type behaviorBackend struct {
	result domain.Result
	err    error
	calls  int
}

func (b *behaviorBackend) Search(context.Context, domain.Request) (domain.Result, error) {
	b.calls++
	return b.result, b.err
}

// An untrusted adapter must not bypass license constraints or claim four families.
func TestSearchBehaviorAtAdapterBoundary(t *testing.T) {
	for _, scenario := range []string{
		"valid", "unknown capacity", "insufficient capacity", "sufficient capacity",
		"unknown platform", "wrong platform", "matching platform",
		"unavailable", "unknown version", "renewal by default", "explicit renewal",
		"upgrade by default", "conflicting role", "unpriced fallback", "fallback with price",
		"mixed price tiers", "duplicate SKU", "empty result", "forged four families",
		"backend unavailable", "not ready", "cancelled", "invalid query",
	} {
		t.Run(scenario, func(t *testing.T) {
			price := int64(100)
			d := domain.Document{OfferID: "sku", FamilyID: "family", FamilyConfirmed: true, CatalogID: "catalog", PriceMinor: &price, Available: true, OfferKind: "new_license"}
			r := domain.Request{Query: "Backup", Limit: 4}
			b := &behaviorBackend{result: domain.Result{CatalogID: "catalog", IndexVersion: "index"}}
			var want error
			switch scenario {
			case "unknown capacity", "insufficient capacity", "sufficient capacity":
				r.MinUsers = 10
				if scenario != "unknown capacity" {
					n := 5
					if scenario == "sufficient capacity" {
						n = 10
					}
					d.UsersLimit = &n
				}
				if scenario != "sufficient capacity" {
					want = domain.ErrUnavailable
				}
			case "unknown platform", "wrong platform", "matching platform":
				r.Platform = "linux"
				if scenario == "wrong platform" {
					d.Platforms = []string{"windows"}
				}
				if scenario == "matching platform" {
					d.Platforms = []string{"linux"}
				} else {
					want = domain.ErrUnavailable
				}
			case "unavailable":
				r.Policy = "available_only"
				d.Available = false
				want = domain.ErrUnavailable
			case "unknown version":
				r.Version = "5.0"
				want = domain.ErrUnavailable
			case "renewal by default", "explicit renewal":
				d.OfferKind = "renewal"
				if scenario == "explicit renewal" {
					r.Kind = "renewal"
				} else {
					want = domain.ErrUnavailable
				}
			case "upgrade by default":
				d.OfferKind = "upgrade"
				want = domain.ErrUnavailable
			case "conflicting role":
				d.RoleConflict = true
				want = domain.ErrUnavailable
			case "unpriced fallback", "fallback with price", "mixed price tiers":
				b.result.UnpricedFallbackUsed = true
				if scenario == "unpriced fallback" {
					d.PriceMinor = nil
					d.Available = false
				} else {
					want = domain.ErrUnavailable
				}
			case "backend unavailable":
				b.err = domain.ErrUnavailable
				want = b.err
			case "not ready":
				b.err = domain.ErrNotReady
				want = b.err
			case "cancelled":
				b.err = context.Canceled
				want = b.err
			case "invalid query":
				r.Query = " "
				want = domain.ErrInvalidQuery
			}
			b.result.Hits = []domain.Hit{{Document: d}}
			if scenario == "duplicate SKU" || scenario == "mixed price tiers" {
				other := d
				other.FamilyID = "other"
				if scenario == "mixed price tiers" {
					other.OfferID = "other"
					other.PriceMinor = nil
				}
				b.result.Hits = append(b.result.Hits, domain.Hit{Document: other})
				want = domain.ErrUnavailable
			}
			if scenario == "empty result" {
				b.result.Hits = nil
			}
			if scenario == "forged four families" {
				b.result.FourFamiliesConfirmed = true
				b.result.Hits[0].Document.FamilyConfirmed = false
			}
			result, err := Search(context.Background(), b, r)
			if !errors.Is(err, want) {
				t.Fatalf("got error %v, want %v", err, want)
			}
			if scenario == "invalid query" && b.calls != 0 {
				t.Fatal("invalid input reached backend")
			}
			if err == nil && (result.FourFamiliesConfirmed || result.ShortfallReason != "insufficient_confirmed_families") {
				t.Fatal("shortfall hidden or fabricated extra families")
			}
		})
	}
}
