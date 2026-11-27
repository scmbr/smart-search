// Package feed adapts the legacy streaming reader to the import source port.
package feed

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"

	reader "github.com/scmbr/smart-search/internal/catalog"
	domain "github.com/scmbr/smart-search/internal/domain/catalog"
)

const maxSourceBytes = int64(1 << 30)

type Source struct {
	file     *os.File
	identity domain.Identity
	options  reader.Options
}

// Open copies the source into a private file, so subsequent writes to the
// original cannot corrupt a partially committed import or its resume identity.
func Open(ctx context.Context, path, cacheDir string, options reader.Options) (_ *Source, err error) {
	if options.MaxRecordBytes == 0 {
		options.MaxRecordBytes = 1 << 20
	}
	if options.MaxDocumentBytes == 0 {
		options.MaxDocumentBytes = 1 << 30
	}
	if options.MaxIssues == 0 {
		options.MaxIssues = 100
	}
	if options.MaxRecordBytes < 0 || options.MaxDocumentBytes < 0 || options.MaxIssues < 0 {
		return nil, errors.New("negative reader option")
	}
	input, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open source: %w", err)
	}
	var spool *os.File
	defer func() {
		err = errors.Join(err, input.Close())
		if err != nil && spool != nil {
			err = errors.Join(err, spool.Close(), os.Remove(spool.Name()))
		}
	}()
	if err := os.MkdirAll(cacheDir, 0700); err != nil {
		return nil, err
	}
	spool, err = os.CreateTemp(cacheDir, "feed-*")
	if err != nil {
		return nil, err
	}
	h := sha256.New()
	writer := io.MultiWriter(spool, h)
	limited := io.LimitReader(input, maxSourceBytes+1)
	buf := make([]byte, 128<<10)
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		n, readErr := limited.Read(buf)
		total += int64(n)
		if total > maxSourceBytes {
			return nil, errors.New("source exceeds 1 GiB")
		}
		if n > 0 {
			if _, err := writer.Write(buf[:n]); err != nil {
				return nil, err
			}
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return nil, readErr
		}
	}
	return &Source{file: spool, options: options, identity: domain.Identity{
		SourceSHA256:  hex.EncodeToString(h.Sum(nil)),
		SchemaVersion: fmt.Sprintf("catalog-v1/record=%d/document=%d/issues=%d", options.MaxRecordBytes, options.MaxDocumentBytes, options.MaxIssues),
	}}, nil
}

func (s *Source) Identity() domain.Identity { return s.identity }

func (s *Source) Close() error {
	return errors.Join(s.file.Close(), os.Remove(s.file.Name()))
}

func (s *Source) Read(ctx context.Context, emit func(context.Context, domain.Record) error) (domain.Validation, error) {
	if _, err := s.file.Seek(0, io.SeekStart); err != nil {
		return domain.Validation{}, err
	}
	// Keep sequential file reads large even when the XML decoder requests small chunks.
	r, err := reader.Read(ctx, bufio.NewReaderSize(s.file, 128<<10), sink{emit}, s.options)
	v := domain.Validation{Complete: r.Complete, CatalogDate: r.CatalogDate,
		Categories: r.CategoriesSeen, Offers: r.OffersSeen, Accepted: r.Accepted, Rejected: r.Rejected, Issues: r.IssueCount}
	for _, issue := range r.Issues {
		v.Diagnostics = append(v.Diagnostics, domain.Issue{Code: issue.Code, EntityID: issue.EntityID, Message: issue.Message})
	}
	return v, err
}

type sink struct {
	emit func(context.Context, domain.Record) error
}

func (s sink) WriteCategory(ctx context.Context, c reader.Category) error {
	return s.emit(ctx, domain.Record{Category: &domain.Category{ID: c.ID, ParentID: c.ParentID, Name: c.Name}})
}
func (s sink) WriteOffer(ctx context.Context, o reader.Offer) error {
	parameters := make([]domain.Parameter, len(o.Parameters))
	for i, p := range o.Parameters {
		parameters[i] = domain.Parameter{Name: p.Name, RawValue: p.RawValue, Value: p.Value, Unit: p.Unit, Known: p.Known}
	}
	return s.emit(ctx, domain.Record{Offer: &domain.Offer{
		ID: o.ID, GroupID: o.GroupID, Name: o.Name, Vendor: o.Vendor, VendorURL: o.VendorURL,
		VendorCode: o.VendorCode, URL: o.URL, CategoryID: o.CategoryID, Available: o.Available,
		RawPrice: o.RawPrice, PriceMinor: o.PriceMinor, RawOldPrice: o.RawOldPrice, OldPriceMinor: o.OldPriceMinor,
		OriginalCurrency: o.OriginalCurrency, Currency: o.Currency, Description: o.Description,
		PictureURLs: o.PictureURLs, Parameters: parameters, Attributes: o.Attributes, RawFields: o.RawFields,
	}})
}
