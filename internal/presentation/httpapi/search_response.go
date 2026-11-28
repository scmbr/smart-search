package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	d "github.com/scmbr/smart-search/internal/domain/searchsession"
	"net/url"
	"strings"
)

type priceDTO struct {
	Status   string `json:"status"`
	Amount   *int64 `json:"amount_minor"`
	Currency string `json:"currency"`
}
type offerDTO struct {
	ID         string   `json:"offer_id"`
	Name       string   `json:"name"`
	URL        string   `json:"url"`
	Available  bool     `json:"available"`
	Price      priceDTO `json:"price"`
	Kind       string   `json:"offer_kind"`
	Parameters []any    `json:"parameters"`
}
type cardDTO struct {
	Family    string   `json:"family_id"`
	Confirmed bool     `json:"family_confirmed"`
	Product   string   `json:"product_id"`
	Name      string   `json:"name"`
	Category  string   `json:"category_id"`
	Offer     offerDTO `json:"selected_offer"`
	Reasons   []string `json:"reasons"`
}
type rankedDTO struct {
	Section string `json:"section"`
	Product string `json:"product_id"`
	Family  string `json:"family_id"`
}
type responseDTO struct {
	ID              string            `json:"search_id"`
	Query           string            `json:"query"`
	Corrections     []any             `json:"corrections"`
	Context         map[string]string `json:"context"`
	Catalog         string            `json:"catalog_version"`
	Index           string            `json:"index_version"`
	Mode            string            `json:"mode"`
	Degraded        []string          `json:"degraded_reasons"`
	Matches         []cardDTO         `json:"matches"`
	Alternatives    []any             `json:"alternatives"`
	Recommendations []any             `json:"recommendations"`
	Total           *int              `json:"total_matches"`
	Clarification   []string          `json:"clarification"`
	Duration        int64             `json:"duration_ms"`
	Cursor          *string           `json:"next_cursor"`
	Top4            []rankedDTO       `json:"top4"`
	Shortfall       *string           `json:"shortfall_reason"`
	Unpriced        bool              `json:"unpriced_fallback_used"`
}

func mapSearch(record d.Record) (responseDTO, error) {
	x := record.Result
	if !d.ValidID(record.ID) || !d.ValidID(x.CatalogID) || !d.ValidID(x.IndexVersion) || strings.TrimSpace(x.Query) == "" || record.DurationMS < 0 {
		return responseDTO{}, d.ErrUnavailable
	}
	response := responseDTO{ID: record.ID, Query: x.Query, Catalog: x.CatalogID, Index: x.IndexVersion, Mode: "lexical", Corrections: []any{}, Context: map[string]string{"origin": "unspecified", "segment": "anonymous"}, Degraded: []string{}, Matches: []cardDTO{}, Alternatives: []any{}, Recommendations: []any{}, Clarification: []string{}, Top4: []rankedDTO{}, Duration: record.DurationMS, Unpriced: x.UnpricedFallbackUsed}
	if x.SearchMode == "hybrid" {
		response.Mode = "hybrid"
	}
	for _, hit := range x.Hits {
		doc := hit.Document
		u, err := url.Parse(doc.URL)
		if err != nil || u.Scheme != "https" || u.Host != "store.softline.ru" || u.User != nil || !d.ValidID(doc.OfferID) || !d.ValidID(doc.FamilyID) || !d.ValidID(doc.CategoryID) || doc.Name == "" {
			return responseDTO{}, d.ErrUnavailable
		}
		switch doc.OfferKind {
		case "new_license", "renewal", "upgrade", "support", "unknown":
		default:
			return responseDTO{}, d.ErrUnavailable
		}
		name := doc.ProductName
		if name == "" {
			name = doc.FamilyName
		}
		if name == "" {
			name = doc.Name
		}
		vendor := strings.Split(strings.Trim(u.Path, "/"), "/")[0]
		identity := vendor + "/group/" + doc.GroupID
		if doc.GroupID == "" {
			identity = vendor + "/offer/" + doc.OfferID
		}
		hash := sha256.Sum256([]byte(identity))
		product := "product-" + hex.EncodeToString(hash[:])
		price := priceDTO{Status: "unknown", Currency: "RUB"}
		if doc.PriceMinor != nil {
			if *doc.PriceMinor < 1 {
				return responseDTO{}, d.ErrUnavailable
			}
			price.Status = "known"
			price.Amount = doc.PriceMinor
		}
		card := cardDTO{Family: doc.FamilyID, Confirmed: doc.FamilyConfirmed, Product: product, Name: name, Category: doc.CategoryID, Offer: offerDTO{ID: doc.OfferID, Name: doc.Name, URL: doc.URL, Available: doc.Available, Price: price, Kind: doc.OfferKind, Parameters: []any{}}, Reasons: []string{"Совпадение по поисковому запросу; условия лицензии требуют проверки перед покупкой"}}
		response.Matches = append(response.Matches, card)
		if doc.FamilyConfirmed && len(response.Top4) < 4 {
			response.Top4 = append(response.Top4, rankedDTO{Section: "matches", Product: product, Family: doc.FamilyID})
		}
	}
	if len(x.Hits) == 0 {
		zero := 0
		response.Total = &zero
	}
	if len(response.Top4) < 4 {
		reason := "insufficient_relevant_families"
		if len(x.Hits) >= 4 {
			reason = "verification_incomplete"
		}
		response.Shortfall = &reason
		response.Clarification = append(response.Clarification, "Меньше четырёх подтверждённых семейств; проверенные допродажи пока недоступны")
	}
	return response, nil
}
