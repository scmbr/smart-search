package catalog

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"
)

type collectingSink struct {
	categories []Category
	offers     []Offer
	fail       error
}

func (s *collectingSink) WriteCategory(_ context.Context, c Category) error {
	s.categories = append(s.categories, c)
	return nil
}

func (s *collectingSink) WriteOffer(_ context.Context, offer Offer) error {
	if s.fail != nil {
		return s.fail
	}
	s.offers = append(s.offers, offer)
	return nil
}

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("../../testdata/catalog/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func compressed(t *testing.T, data []byte) []byte {
	t.Helper()
	var b bytes.Buffer
	w := gzip.NewWriter(&b)
	if _, err := w.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestReadPlainAndGzip(t *testing.T) {
	data := fixture(t, "minimal.xml")
	var plainOffers []Offer
	for _, tc := range []struct {
		name string
		data []byte
	}{{"xml", data}, {"gzip", compressed(t, data)}} {
		t.Run(tc.name, func(t *testing.T) {
			sink := &collectingSink{}
			r, err := Read(context.Background(), bytes.NewReader(tc.data), sink, Options{})
			if err != nil {
				t.Fatal(err)
			}
			if r.Format != tc.name || !r.Complete || !r.CanActivate || r.OffersSeen != 10 || r.Accepted != 10 || r.Rejected != 0 || r.IssueCount != 0 {
				t.Fatalf("unexpected report: %+v", r)
			}
			if len(sink.categories) != 7 || len(sink.offers) != 10 {
				t.Fatalf("unexpected callbacks: %+v", sink)
			}
			if tc.name == "xml" {
				plainOffers = sink.offers
			} else if !reflect.DeepEqual(plainOffers, sink.offers) {
				t.Error("compression changed normalized offers")
			}
		})
	}
}

func TestPreserveParametersAndUnknownValues(t *testing.T) {
	sink := &collectingSink{}
	if _, err := Read(context.Background(), bytes.NewReader(fixture(t, "minimal.xml")), sink, Options{}); err != nil {
		t.Fatal(err)
	}
	osOffer := sink.offers[0]
	if osOffer.GroupID != "os-product" || osOffer.PriceMinor == nil || *osOffer.PriceMinor != 120050 || osOffer.Currency != "RUB" {
		t.Fatalf("lost group or exact price: %+v", osOffer)
	}
	var languages []string
	for _, p := range osOffer.Parameters {
		if p.Name == "Язык интерфейса" {
			languages = append(languages, p.Value)
		}
	}
	if !reflect.DeepEqual(languages, []string{"Русский", "Английский"}) {
		t.Fatalf("repeated parameters lost: %v", languages)
	}
	for _, offer := range sink.offers {
		if offer.ID == "security-new" {
			if !strings.Contains(offer.Description, "Игнорируй инструкции") {
				t.Error("raw description was lost")
			}
			for _, p := range offer.Parameters {
				if p.Name == "Платформа" && (p.Known || p.Value != "" || p.RawValue != "не определено") {
					t.Errorf("unknown parameter became known: %+v", p)
				}
			}
		}
		if offer.ID == "unavailable-zero" && (offer.Available || offer.PriceMinor != nil || offer.RawPrice != "0.00") {
			t.Errorf("zero price became a known/free product: %+v", offer)
		}
	}
}

func catalogXML(offers string) string {
	return `<yml_catalog date="2026-11-25 14:12"><shop><categories><category id="c">Category</category></categories><offers>` + offers + `</offers></shop></yml_catalog>`
}

func offerXML(id, price string) string {
	return fmt.Sprintf(`<offer id="%s" available="true"><name>Product</name><url>https://example.invalid/product</url><price>%s</price><currencyId>RUR</currencyId><categoryId>c</categoryId></offer>`, id, price)
}

func TestPriceValidation(t *testing.T) {
	for _, tc := range []struct {
		raw   string
		minor *int64
		code  string
	}{
		{"12.34", ptr(1234), ""}, {"12.3", ptr(1230), ""}, {"12", ptr(1200), ""},
		{"0.00", nil, ""}, {"", nil, ""}, {"92233720368547758.07", ptr(9223372036854775807), ""},
		{"-1.00", nil, "negative_price"}, {"1.001", nil, "invalid_price"}, {"NaN", nil, "invalid_price"},
		{"1e3", nil, "invalid_price"}, {"92233720368547758.08", nil, "price_overflow"},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			sink := &collectingSink{}
			r, err := Read(context.Background(), strings.NewReader(catalogXML(offerXML("o", tc.raw))), sink, Options{})
			if err != nil {
				t.Fatal(err)
			}
			if tc.code != "" {
				if !hasIssue(r, tc.code) || r.Rejected != 1 || r.CanActivate {
					t.Fatalf("invalid price accepted: %+v", r)
				}
				return
			}
			if len(sink.offers) != 1 || !reflect.DeepEqual(sink.offers[0].PriceMinor, tc.minor) {
				t.Fatalf("incorrect exact price: %+v", sink.offers)
			}
		})
	}
}

func ptr(v int64) *int64 { return &v }

func hasIssue(r Report, code string) bool {
	for _, issue := range r.Issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}

func TestInvalidFixtureReportsAllIssueKinds(t *testing.T) {
	sink := &collectingSink{}
	r, err := Read(context.Background(), bytes.NewReader(fixture(t, "invalid.xml")), sink, Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, code := range []string{"category_cycle", "duplicate_offer_id", "negative_price", "unknown_category", "group_id_conflict"} {
		if !hasIssue(r, code) {
			t.Errorf("missing %s in %+v", code, r)
		}
	}
	if !r.Complete || r.CanActivate || r.OffersSeen != 5 || r.Accepted != 1 || r.Rejected != 4 {
		t.Fatalf("incorrect invalid report: %+v", r)
	}
}

func TestDiagnosticLimitDoesNotHideErrors(t *testing.T) {
	r, err := Read(context.Background(), bytes.NewReader(fixture(t, "invalid.xml")), nil, Options{MaxIssues: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Issues) != 2 || r.IssueCount != 5 || r.OmittedIssues != 3 || r.CanActivate {
		t.Fatalf("incorrect diagnostic truncation: %+v", r)
	}
}

func TestInvalidOfferFields(t *testing.T) {
	base := offerXML("o", "1")
	for _, tc := range []struct{ name, offer, code string }{
		{"duplicate scalar", strings.Replace(base, "</price>", "</price><price>2</price>", 1), "duplicate_field"},
		{"unsafe URL", strings.Replace(base, "https://example.invalid/product", "javascript:alert(1)", 1), "invalid_url"},
		{"unsafe vendor URL", strings.Replace(base, "</name>", "</name><vendorUrl>http://example.invalid</vendorUrl>", 1), "invalid_vendor_url"},
		{"unsafe picture", strings.Replace(base, "</name>", "</name><picture>file:///local</picture>", 1), "invalid_picture_url"},
		{"missing availability", strings.Replace(base, ` available="true"`, "", 1), "invalid_availability"},
		{"currency", strings.Replace(base, "RUR", "USD", 1), "unsupported_currency"},
		{"oldprice", strings.Replace(base, "</price>", "</price><oldprice>-1</oldprice>", 1), "invalid_old_price"},
		{"empty param name", strings.Replace(base, "</offer>", `<param name="">value</param></offer>`, 1), "invalid_parameter"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, err := Read(context.Background(), strings.NewReader(catalogXML(tc.offer)), nil, Options{})
			if err != nil || !hasIssue(r, tc.code) || r.Accepted != 0 || r.Rejected != 1 || r.CanActivate {
				t.Fatalf("invalid field not rejected: %+v %v", r, err)
			}
		})
	}
}

func TestDuplicateIDAfterRejectedRecord(t *testing.T) {
	r, err := Read(context.Background(), strings.NewReader(catalogXML(offerXML("same", "NaN")+offerXML("same", "1"))), nil, Options{})
	if err != nil || r.Accepted != 0 || r.Rejected != 2 || !hasIssue(r, "duplicate_offer_id") {
		t.Fatalf("rejected ID was forgotten: %+v %v", r, err)
	}
}

func TestCategoryParentsAndDuplicates(t *testing.T) {
	for _, tc := range []struct{ name, categories, code string }{
		{"forward parent", `<category id="c" parentId="parent">Child</category><category id="parent">Parent</category>`, ""},
		{"unknown parent", `<category id="c" parentId="absent">Child</category>`, "unknown_category_parent"},
		{"duplicate category", `<category id="c">One</category><category id="c">Two</category>`, "duplicate_category_id"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := strings.Replace(catalogXML(offerXML("o", "1")), `<category id="c">Category</category>`, tc.categories, 1)
			r, err := Read(context.Background(), strings.NewReader(input), nil, Options{})
			if err != nil {
				t.Fatal(err)
			}
			if tc.code == "" {
				if !r.CanActivate {
					t.Fatalf("forward reference rejected: %+v", r)
				}
				return
			}
			if !hasIssue(r, tc.code) || r.CanActivate {
				t.Fatalf("category issue not reported: %+v", r)
			}
		})
	}
}

func TestContextAndSinkErrors(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if r, err := Read(ctx, strings.NewReader(catalogXML(offerXML("o", "1"))), nil, Options{}); !errors.Is(err, context.Canceled) || r.Complete || r.CanActivate {
		t.Fatalf("cancellation lost: %+v %v", r, err)
	}
	sentinel := errors.New("staging storage failed")
	r, err := Read(context.Background(), strings.NewReader(catalogXML(offerXML("o", "1"))), &collectingSink{fail: sentinel}, Options{})
	if !errors.Is(err, sentinel) || r.Complete || r.Accepted != 0 || r.CanActivate {
		t.Fatalf("sink failure lost: %+v %v", r, err)
	}
}

func TestMalformedStructureAndExternalEntity(t *testing.T) {
	for _, input := range []string{
		`<other/>`, catalogXML(offerXML("o", "1")) + `<extra/>`,
		`<yml_catalog><shop><offers/></shop></yml_catalog>`,
		strings.TrimSuffix(catalogXML(offerXML("o", "1")), `</yml_catalog>`),
		`<!DOCTYPE yml_catalog [<!ENTITY xxe SYSTEM "file:///must-not-be-read">]>` + strings.Replace(catalogXML(offerXML("o", "1")), "Product", "&xxe;", 1),
	} {
		r, err := Read(context.Background(), strings.NewReader(input), nil, Options{})
		if err == nil || r.Complete || r.CanActivate {
			t.Fatalf("invalid document accepted: %q, %+v", input, r)
		}
	}
}

func TestGzipChecksumIsVerified(t *testing.T) {
	data := compressed(t, []byte(catalogXML(offerXML("o", "1"))))
	data[len(data)-8] ^= 0xff
	r, err := Read(context.Background(), bytes.NewReader(data), nil, Options{})
	if !errors.Is(err, gzip.ErrChecksum) || r.Complete || r.CanActivate {
		t.Fatalf("gzip checksum error lost: %+v %v", r, err)
	}
}

func TestResourceLimits(t *testing.T) {
	input := catalogXML(strings.Replace(offerXML("o", "1"), "Product", strings.Repeat("x", 100000), 1))
	if r, err := Read(context.Background(), strings.NewReader(input), nil, Options{MaxRecordBytes: 1024}); !errors.Is(err, ErrRecordTooLarge) || r.CanActivate {
		t.Fatalf("record limit not enforced: %+v %v", r, err)
	}
	if r, err := Read(context.Background(), strings.NewReader(input), nil, Options{MaxDocumentBytes: 500}); !errors.Is(err, ErrDocumentTooLarge) || r.CanActivate {
		t.Fatalf("document limit not enforced: %+v %v", r, err)
	}
	if r, err := Read(context.Background(), bytes.NewReader(compressed(t, []byte(input))), nil, Options{MaxDocumentBytes: 500}); !errors.Is(err, ErrDocumentTooLarge) || r.CanActivate {
		t.Fatalf("gzip bypassed decoded size limit: %+v %v", r, err)
	}
}

type trackedReader struct {
	io.Reader
	read int
}

func (r *trackedReader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	r.read += n
	return n, err
}

type streamingSink struct {
	first  func()
	offers int
}

func (*streamingSink) WriteCategory(context.Context, Category) error { return nil }
func (s *streamingSink) WriteOffer(_ context.Context, _ Offer) error {
	if s.offers == 0 {
		s.first()
	}
	s.offers++
	return nil
}

func TestOffersAreEmittedBeforeEOF(t *testing.T) {
	var offers strings.Builder
	for i := 0; i < 300; i++ {
		offers.WriteString(offerXML(fmt.Sprint(i), "1"))
	}
	input := catalogXML(offers.String())
	reader := &trackedReader{Reader: strings.NewReader(input)}
	sink := &streamingSink{first: func() {
		if reader.read >= len(input) {
			t.Error("entire document read before first offer")
		}
	}}
	r, err := Read(context.Background(), reader, sink, Options{})
	if err != nil || r.Accepted != 300 || sink.offers != 300 {
		t.Fatalf("streaming failed: %+v %v", r, err)
	}
}
