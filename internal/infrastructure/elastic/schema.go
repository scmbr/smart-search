package elastic

import (
	"strings"

	app "github.com/scmbr/smart-search/internal/application/lexical"
	domain "github.com/scmbr/smart-search/internal/domain/search"
)

func mapping(spec app.Spec) map[string]any {
	properties := map[string]any{}
	for _, name := range []string{"OfferID", "GroupID", "FamilyID", "FamilySource", "URL", "CategoryID", "CatalogID", "ProductType", "OfferKind", "Version", "Platforms"} {
		properties[name] = map[string]any{"type": "keyword", "ignore_above": 8192}
	}
	for _, name := range []string{"FamilyConfirmed", "RoleConflict", "HasPrice", "Available"} {
		properties[name] = map[string]any{"type": "boolean"}
	}
	for _, name := range []string{"UsersLimit", "PriceMinor"} {
		properties[name] = map[string]any{"type": "long"}
	}
	for _, name := range []string{"Name", "ProductName", "Vendor", "FamilyName", "CategoryNames", "Description", "VendorCode"} {
		properties[name] = map[string]any{"type": "text", "analyzer": "lexical"}
	}
	properties["VendorCode"] = map[string]any{"type": "text", "analyzer": "lexical", "fields": map[string]any{"exact": map[string]any{"type": "keyword", "normalizer": "lower"}}}
	return map[string]any{"settings": map[string]any{"number_of_shards": 1, "number_of_replicas": 0, "refresh_interval": "-1", "analysis": map[string]any{
		"normalizer":  map[string]any{"lower": map[string]any{"type": "custom", "filter": []string{"lowercase"}}},
		"char_filter": map[string]any{"yo": map[string]any{"type": "mapping", "mappings": []string{"Ё => Е", "ё => е"}}},
		"filter":      map[string]any{"parts": map[string]any{"type": "word_delimiter_graph", "preserve_original": true, "catenate_words": true, "catenate_numbers": true}, "ru_stem": map[string]any{"type": "stemmer", "language": "russian"}, "ru_stop": map[string]any{"type": "stop", "stopwords": "_russian_"}},
		"analyzer":    map[string]any{"lexical": map[string]any{"type": "custom", "char_filter": []string{"html_strip", "yo"}, "tokenizer": "standard", "filter": []string{"parts", "flatten_graph", "lowercase", "ru_stop", "ru_stem"}}},
	}}, "mappings": map[string]any{"dynamic": "strict", "_meta": metadata{State: "staging", Spec: spec}, "properties": properties}}
}

func queryBody(r domain.Request, priced bool) map[string]any {
	return searchBody(r, priced, false)
}

func searchBody(r domain.Request, priced, expanded bool) map[string]any {
	term := func(field string, value any) map[string]any {
		return map[string]any{"term": map[string]any{field: value}}
	}
	filters := []any{term("HasPrice", priced), term("RoleConflict", false)}
	if r.Policy == "available_only" {
		filters = append(filters, term("Available", true))
	}
	if r.Version != "" {
		filters = append(filters, term("Version", r.Version))
	}
	if r.Kind != "" {
		filters = append(filters, term("OfferKind", r.Kind))
	}
	if r.Platform != "" {
		filters = append(filters, term("Platforms", r.Platform))
	}
	if r.ProductType != "" {
		filters = append(filters, term("ProductType", r.ProductType))
	}
	if r.MinUsers > 0 {
		filters = append(filters, map[string]any{"range": map[string]any{"UsersLimit": map[string]int{"gte": r.MinUsers}}})
	}
	clauses := []any{
		map[string]any{"term": map[string]any{"OfferID": map[string]any{"value": r.Query, "boost": 50}}},
		map[string]any{"term": map[string]any{"VendorCode.exact": map[string]any{"value": strings.ToLower(r.Query), "boost": 30}}},
		map[string]any{"multi_match": map[string]any{"query": r.Query, "fields": []string{"ProductName^24", "Name^8", "FamilyName^4", "VendorCode^12", "Vendor^2", "CategoryNames^3", "Description"}, "operator": "and"}},
	}
	if expanded {
		for _, variant := range r.Plan.Variants {
			var required []any
			for _, word := range variant.Terms {
				match := map[string]any{"query": word.Text, "fields": []string{"ProductName^24", "Name^8", "FamilyName^4"}, "operator": "and"}
				if word.Fuzziness > 0 {
					match["fuzziness"] = word.Fuzziness
					match["prefix_length"] = 1
					match["max_expansions"] = 3
					match["fuzzy_transpositions"] = true
				}
				required = append(required, map[string]any{"multi_match": match})
			}
			if len(required) > 0 {
				clauses = append(clauses, map[string]any{"bool": map[string]any{"must": required, "boost": 0.5}})
			}
		}
	}
	text := map[string]any{"bool": map[string]any{"minimum_should_match": 1, "should": clauses}}
	boolQuery := map[string]any{"must": []any{text}, "filter": filters, "should": []any{map[string]any{"term": map[string]any{"OfferKind": map[string]any{"value": "new_license", "boost": 0.15}}}}}
	if r.Kind == "" {
		boolQuery["must_not"] = []any{map[string]any{"terms": map[string]any{"OfferKind": []string{"renewal", "upgrade", "support"}}}}
	}
	return map[string]any{"size": r.Limit, "timeout": "5s", "track_total_hits": false, "query": map[string]any{"bool": boolQuery}, "collapse": map[string]any{"field": "FamilyID"}, "sort": []any{map[string]string{"_score": "desc"}, map[string]string{"Available": "desc"}, map[string]string{"OfferID": "asc"}}}
}
