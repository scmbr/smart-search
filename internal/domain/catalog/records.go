// Package catalog owns catalog snapshot invariants, independent of storage and XML.
package catalog

type Category struct {
	ID, ParentID, Name string
}

type Parameter struct {
	Name, RawValue, Value, Unit string
	Known                       bool
}

type Offer struct {
	ID, GroupID, Name, Vendor, VendorURL, VendorCode, URL, CategoryID string
	Available                                                         bool
	RawPrice                                                          string
	PriceMinor                                                        *int64
	RawOldPrice                                                       string
	OldPriceMinor                                                     *int64
	OriginalCurrency, Currency, Description                           string
	PictureURLs                                                       []string
	Parameters                                                        []Parameter
	Attributes                                                        map[string]string
	RawFields                                                         map[string][]string
}

// Record contains exactly one category or offer, in source order.
type Record struct {
	Category *Category
	Offer    *Offer
}
