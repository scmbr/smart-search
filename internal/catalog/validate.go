package catalog

import (
	"encoding/xml"
	"math"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

func normalizeOffer(attrs []xml.Attr, raw rawOffer, categories map[string]Category) (Offer, *Issue) {
	o := Offer{ID: strings.TrimSpace(attribute(attrs, "id")), Attributes: make(map[string]string), RawFields: make(map[string][]string), Parameters: make([]Parameter, 0)}
	fail := func(code, message string) (Offer, *Issue) {
		return o, &Issue{Code: code, EntityID: o.ID, Message: message}
	}
	for _, a := range attrs {
		key := a.Name.Local
		if a.Name.Space != "" {
			key = a.Name.Space + ":" + a.Name.Local
		}
		if _, exists := o.Attributes[key]; exists {
			return fail("duplicate_attribute", "Repeated offer attribute")
		}
		o.Attributes[key] = a.Value
	}
	if o.ID == "" {
		return fail("invalid_offer_id", "Offer ID is required")
	}
	scalar := map[string]bool{"name": true, "vendor": true, "vendorUrl": true, "vendorCode": true, "url": true, "price": true, "oldprice": true, "currencyId": true, "categoryId": true, "group_id": true, "description": true}
	for _, f := range raw.Fields {
		name := f.XMLName.Local
		if f.XMLName.Space != "" {
			continue
		}
		if scalar[name] && len(o.RawFields[name]) > 0 {
			return fail("duplicate_field", "Repeated scalar field: "+name)
		}
		o.RawFields[name] = append(o.RawFields[name], f.Text)
		if name == "param" {
			p := Parameter{Name: strings.TrimSpace(attribute(f.Attributes, "name")), RawValue: f.Text, Unit: strings.TrimSpace(attribute(f.Attributes, "unit"))}
			if p.Name == "" {
				return fail("invalid_parameter", "Parameter name is required")
			}
			p.Value = strings.TrimSpace(f.Text)
			switch strings.ToLower(p.Value) {
			case "", "не определено", "другое или отсутствует", "другое", "нет данных", "не указано":
				p.Value = ""
			default:
				p.Known = true
			}
			o.Parameters = append(o.Parameters, p)
		}
	}
	value := func(key string) string {
		if fields := o.RawFields[key]; len(fields) > 0 {
			return strings.TrimSpace(fields[0])
		}
		return ""
	}
	o.Name, o.Vendor, o.VendorURL, o.VendorCode = value("name"), value("vendor"), value("vendorUrl"), value("vendorCode")
	o.URL, o.CategoryID, o.Description = value("url"), value("categoryId"), value("description")
	if o.Name == "" {
		return fail("invalid_name", "Offer name is required")
	}
	if !safeURL(o.URL) {
		return fail("invalid_url", "Offer URL must be absolute HTTPS without credentials")
	}
	if o.VendorURL != "" && !safeURL(o.VendorURL) {
		return fail("invalid_vendor_url", "Vendor URL must be absolute HTTPS without credentials")
	}
	for _, p := range o.RawFields["picture"] {
		p = strings.TrimSpace(p)
		if !safeURL(p) {
			return fail("invalid_picture_url", "Picture URL must be absolute HTTPS without credentials")
		}
		o.PictureURLs = append(o.PictureURLs, p)
	}
	if _, exists := categories[o.CategoryID]; !exists {
		return fail("unknown_category", "Offer references an unknown category")
	}
	switch strings.TrimSpace(attribute(attrs, "available")) {
	case "true":
		o.Available = true
	case "false":
		o.Available = false
	default:
		return fail("invalid_availability", "available must be true or false")
	}
	o.GroupID = strings.TrimSpace(attribute(attrs, "group_id"))
	if group := value("group_id"); group != "" {
		if o.GroupID != "" && o.GroupID != group {
			return fail("group_id_conflict", "Attribute and element group_id disagree")
		}
		o.GroupID = group
	}
	o.OriginalCurrency = value("currencyId")
	switch strings.ToUpper(o.OriginalCurrency) {
	case "RUR", "RUB":
		o.Currency = "RUB"
	default:
		return fail("unsupported_currency", "Only RUR and RUB are supported")
	}
	o.RawPrice = value("price")
	var code string
	o.PriceMinor, code = parsePrice(o.RawPrice)
	if code != "" {
		return fail(code, "Invalid price: "+o.RawPrice)
	}
	o.RawOldPrice = value("oldprice")
	o.OldPriceMinor, code = parsePrice(o.RawOldPrice)
	if code != "" {
		return fail("invalid_old_price", "Invalid oldprice: "+o.RawOldPrice)
	}
	return o, nil
}

func safeURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.Hostname() != "" && u.User == nil && u.Opaque == ""
}

func parsePrice(raw string) (*int64, string) {
	if raw == "" {
		return nil, ""
	}
	if strings.HasPrefix(raw, "-") {
		return nil, "negative_price"
	}
	parts := strings.Split(raw, ".")
	if len(parts) > 2 || parts[0] == "" {
		return nil, "invalid_price"
	}
	for _, p := range parts {
		if p == "" {
			return nil, "invalid_price"
		}
		for _, c := range p {
			if c < '0' || c > '9' {
				return nil, "invalid_price"
			}
		}
	}
	whole, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return nil, "price_overflow"
	}
	var fraction int64
	if len(parts) == 2 {
		if len(parts[1]) > 2 {
			return nil, "invalid_price"
		}
		fraction = int64(parts[1][0]-'0') * 10
		if len(parts[1]) == 2 {
			fraction += int64(parts[1][1] - '0')
		}
	}
	if whole > (math.MaxInt64-fraction)/100 {
		return nil, "price_overflow"
	}
	minor := whole*100 + fraction
	if minor == 0 {
		return nil, ""
	}
	return &minor, ""
}

func validateCategories(categories map[string]Category, r *Report, limit int) {
	ids := make([]string, 0, len(categories))
	for id := range categories {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	done := make(map[string]bool)
	for _, id := range ids {
		c := categories[id]
		if c.ParentID != "" {
			if _, exists := categories[c.ParentID]; !exists {
				r.addIssue(limit, "unknown_category_parent", id, "Category parent does not exist")
			}
		}
		if done[id] {
			continue
		}
		var path []string
		positions := make(map[string]bool)
		current := id
		for current != "" && !done[current] {
			c, exists := categories[current]
			if !exists {
				break
			}
			if positions[current] {
				r.addIssue(limit, "category_cycle", current, "Category parent chain contains a cycle")
				break
			}
			positions[current] = true
			path = append(path, current)
			current = c.ParentID
		}
		for _, visited := range path {
			done[visited] = true
		}
	}
}
