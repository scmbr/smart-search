package catalog

import (
	"bufio"
	"compress/gzip"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"strings"
)

type guardedReader struct {
	ctx           context.Context
	r             io.Reader
	read          int64
	documentLimit int64
	budget        int64
}

func (r *guardedReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	if r.read >= r.documentLimit {
		var probe [1]byte
		n, err := r.r.Read(probe[:])
		if n > 0 {
			return 0, ErrDocumentTooLarge
		}
		return 0, err
	}
	if r.budget <= 0 {
		return 0, ErrRecordTooLarge
	}
	n := int64(len(p))
	if remaining := r.documentLimit - r.read; n > remaining {
		n = remaining
	}
	if n > r.budget {
		n = r.budget
	}
	got, err := r.r.Read(p[:n])
	r.read += int64(got)
	r.budget -= int64(got)
	return got, err
}

type rawField struct {
	XMLName    xml.Name
	Text       string     `xml:",chardata"`
	Attributes []xml.Attr `xml:",any,attr"`
}

type rawOffer struct {
	Fields []rawField `xml:",any"`
}

type rawCategory struct {
	ID     string `xml:"id,attr"`
	Parent string `xml:"parentId,attr"`
	Name   string `xml:",chardata"`
}

// Read determines compression by signature and emits provisional records as it
// parses. It does not close input, persist snapshots, or access external DTDs.
func Read(ctx context.Context, input io.Reader, sink Sink, opts Options) (report Report, err error) {
	if opts.MaxRecordBytes < 0 || opts.MaxDocumentBytes < 0 || opts.MaxIssues < 0 {
		return report, fmt.Errorf("negative catalog limit")
	}
	if opts.MaxRecordBytes == 0 {
		opts.MaxRecordBytes = 1 << 20
	}
	if opts.MaxDocumentBytes == 0 {
		opts.MaxDocumentBytes = 1 << 30
	}
	if opts.MaxIssues == 0 {
		opts.MaxIssues = 100
	}
	if input == nil {
		return report, fmt.Errorf("nil catalog reader")
	}
	if err := ctx.Err(); err != nil {
		return report, err
	}
	buffer := bufio.NewReader(input)
	signature, peekErr := buffer.Peek(2)
	if peekErr != nil && peekErr != io.EOF {
		return report, fmt.Errorf("inspect catalog: %w", peekErr)
	}
	var decoded io.Reader = buffer
	report.Format = "xml"
	if len(signature) == 2 && signature[0] == 0x1f && signature[1] == 0x8b {
		report.Format = "gzip"
		gz, e := gzip.NewReader(buffer)
		if e != nil {
			return report, fmt.Errorf("open gzip: %w", e)
		}
		defer gz.Close() // The underlying input is owned by the caller.
		decoded = gz
	}
	guard := &guardedReader{ctx: ctx, r: decoded, documentLimit: opts.MaxDocumentBytes}
	defer func() {
		report.DecodedBytes = guard.read
		report.CanActivate = err == nil && report.Complete && report.Accepted > 0 && report.IssueCount == 0
	}()
	decoder := xml.NewDecoder(guard)
	categories := make(map[string]Category)
	offerIDs := make(map[string]bool)
	var path []string
	var rootSeen, shopSeen, categoriesSeen, categoriesDone, offersSeen, offersDone bool
	for {
		if e := ctx.Err(); e != nil {
			return report, e
		}
		guard.budget = opts.MaxRecordBytes
		offset := decoder.InputOffset()
		token, e := decoder.Token()
		if e == io.EOF {
			break
		}
		if e != nil {
			return report, fmt.Errorf("read XML at byte %d: %w", decoder.InputOffset(), e)
		}
		if decoder.InputOffset()-offset > opts.MaxRecordBytes {
			return report, ErrRecordTooLarge
		}
		switch v := token.(type) {
		case xml.StartElement:
			if v.Name.Space != "" {
				return report, fmt.Errorf("%w: unsupported namespace", ErrInvalidStructure)
			}
			if len(path) == 0 {
				if rootSeen || v.Name.Local != "yml_catalog" {
					return report, fmt.Errorf("%w: expected one yml_catalog root", ErrInvalidStructure)
				}
				rootSeen = true
				report.CatalogDate = strings.TrimSpace(attribute(v.Attr, "date"))
			}
			path = append(path, v.Name.Local)
			current := strings.Join(path, "/")
			switch current {
			case "yml_catalog/shop":
				if shopSeen {
					return report, fmt.Errorf("%w: duplicate shop", ErrInvalidStructure)
				}
				shopSeen = true
			case "yml_catalog/shop/categories":
				if categoriesSeen || offersSeen {
					return report, fmt.Errorf("%w: categories must occur once before offers", ErrInvalidStructure)
				}
				categoriesSeen = true
			case "yml_catalog/shop/offers":
				if offersSeen || !categoriesDone {
					return report, fmt.Errorf("%w: offers require completed categories", ErrInvalidStructure)
				}
				offersSeen = true
			case "yml_catalog/shop/categories/category":
				var raw rawCategory
				if e := decodeRecord(decoder, guard, &raw, v, offset, opts.MaxRecordBytes); e != nil {
					return report, e
				}
				path = path[:len(path)-1]
				report.CategoriesSeen++
				c := Category{ID: strings.TrimSpace(raw.ID), ParentID: strings.TrimSpace(raw.Parent), Name: strings.TrimSpace(raw.Name)}
				if c.ID == "" || c.Name == "" {
					report.addIssue(opts.MaxIssues, "invalid_category", c.ID, "Category ID and name are required")
					continue
				}
				if _, exists := categories[c.ID]; exists {
					report.addIssue(opts.MaxIssues, "duplicate_category_id", c.ID, "Repeated category ID")
					continue
				}
				categories[c.ID] = c
				if sink != nil {
					if e := sink.WriteCategory(ctx, c); e != nil {
						return report, fmt.Errorf("write category %s: %w", c.ID, e)
					}
				}
			case "yml_catalog/shop/offers/offer":
				report.OffersSeen++
				var raw rawOffer
				if e := decodeRecord(decoder, guard, &raw, v, offset, opts.MaxRecordBytes); e != nil {
					return report, e
				}
				path = path[:len(path)-1]
				id := strings.TrimSpace(attribute(v.Attr, "id"))
				if id != "" && offerIDs[id] {
					report.addIssue(opts.MaxIssues, "duplicate_offer_id", id, "Repeated offer ID, including rejected records")
					report.Rejected++
					continue
				}
				if id != "" {
					offerIDs[id] = true
				}
				offer, issue := normalizeOffer(v.Attr, raw, categories)
				if issue != nil {
					report.addIssue(opts.MaxIssues, issue.Code, id, issue.Message)
					report.Rejected++
					continue
				}
				if sink != nil {
					if e := sink.WriteOffer(ctx, offer); e != nil {
						return report, fmt.Errorf("write offer %s: %w", id, e)
					}
				}
				report.Accepted++
				if offer.Available {
					report.AvailableOffers++
				}
				if offer.PriceMinor == nil {
					report.UnknownPrices++
				} else {
					report.KnownPrices++
				}
			default:
				if v.Name.Local == "offer" || v.Name.Local == "category" || v.Name.Local == "shop" || v.Name.Local == "offers" || v.Name.Local == "categories" {
					return report, fmt.Errorf("%w: unexpected %s", ErrInvalidStructure, current)
				}
			}
		case xml.EndElement:
			current := strings.Join(path, "/")
			if current == "yml_catalog/shop/categories" {
				categoriesDone = true
				validateCategories(categories, &report, opts.MaxIssues)
			}
			if current == "yml_catalog/shop/offers" {
				offersDone = true
			}
			if len(path) == 0 {
				return report, ErrInvalidStructure
			}
			path = path[:len(path)-1]
		case xml.CharData:
			if len(path) == 0 && strings.TrimSpace(string(v)) != "" {
				return report, fmt.Errorf("%w: content outside root", ErrInvalidStructure)
			}
		}
	}
	if !rootSeen || !shopSeen || !categoriesDone || !offersDone || len(path) != 0 {
		return report, fmt.Errorf("%w: incomplete catalog", ErrInvalidStructure)
	}
	if report.OffersSeen == 0 {
		report.addIssue(opts.MaxIssues, "empty_catalog", "", "Snapshot has no offers")
	}
	report.Complete = true
	return report, nil
}

func decodeRecord(decoder *xml.Decoder, guard *guardedReader, target any, start xml.StartElement, offset, max int64) error {
	guard.budget = max
	if err := decoder.DecodeElement(target, &start); err != nil {
		return fmt.Errorf("decode %s at byte %d: %w", start.Name.Local, offset, err)
	}
	if decoder.InputOffset()-offset > max {
		return ErrRecordTooLarge
	}
	return nil
}

func attribute(attrs []xml.Attr, name string) string {
	for _, a := range attrs {
		if a.Name.Space == "" && a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}
