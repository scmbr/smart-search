package postgres

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	a "github.com/scmbr/smart-search/internal/application/lexical"
	reader "github.com/scmbr/smart-search/internal/catalog"
	c "github.com/scmbr/smart-search/internal/domain/catalog"
	"github.com/scmbr/smart-search/internal/infrastructure/feed"
)

const indexSpoolLimit int64 = 4 << 30

type CatalogFileSource struct {
	pool        *pgxpool.Pool
	path, cache string
}

func NewCatalogFileSource(pool *pgxpool.Pool, path, cache string) *CatalogFileSource {
	return &CatalogFileSource{pool: pool, path: path, cache: cache}
}

type fileIndexFeed struct {
	*catalogFeed
	file     *os.File
	decoder  *json.Decoder
	position int64
	closed   bool
}
type boundedIndexWriter struct {
	writer    io.Writer
	remaining int64
}

func (w *boundedIndexWriter) Write(p []byte) (int, error) {
	if int64(len(p)) > w.remaining {
		return 0, errors.New("index source spool exceeds 4 GiB")
	}
	n, err := w.writer.Write(p)
	w.remaining -= int64(n)
	return n, err
}

// Open validates the complete private feed before exposing any indexing rows.
func (s *CatalogFileSource) Open(ctx context.Context) (_ a.Feed, err error) {
	metadata, err := NewCatalogSource(s.pool).openMetadata(ctx)
	if err != nil {
		return nil, err
	}
	result := &fileIndexFeed{catalogFeed: metadata}
	defer func() {
		if err != nil {
			err = errors.Join(err, result.Close())
		}
	}()
	raw, err := feed.Open(ctx, s.path, s.cache, reader.Options{})
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, raw.Close()) }()
	identity := raw.Identity()
	identity.SchemaVersion += "/" + c.NormalizationVersion
	if identity.ID() != metadata.info.ID || identity.SchemaVersion != metadata.info.SchemaVersion {
		return nil, errors.New("local feed does not match active snapshot SHA/reader limits/normalization")
	}
	result.file, err = os.CreateTemp(s.cache, "index-rows-*")
	if err != nil {
		return nil, err
	}
	buffered := bufio.NewWriterSize(result.file, 128<<10)
	encoder := json.NewEncoder(&boundedIndexWriter{writer: buffered, remaining: indexSpoolLimit})
	var ordinal int64
	validation, err := raw.Read(ctx, func(ctx context.Context, record c.Record) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		ordinal++
		if record.Category != nil {
			metadata.categories = append(metadata.categories, *record.Category)
			return nil
		}
		if record.Offer == nil {
			return errors.New("empty local index record")
		}
		offer := minimalIndexOffer(*record.Offer)
		return encoder.Encode(a.Row{Ordinal: ordinal, Offer: offer})
	})
	if err != nil {
		return nil, err
	}
	if !validation.Eligible() || validation.Accepted != metadata.info.Offers || validation.Categories != metadata.validation.Categories || ordinal != int64(validation.Categories+validation.Accepted) {
		return nil, errors.New("local index source validation differs from active snapshot")
	}
	if err = buffered.Flush(); err != nil {
		return nil, err
	}
	if _, err = result.file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	result.decoder = json.NewDecoder(bufio.NewReaderSize(result.file, 128<<10))
	return result, nil
}

// Drop only fields BuildDocument never reads, after using the full normalization input.
func minimalIndexOffer(o c.Offer) c.Offer {
	chars := c.Normalize(o)
	for _, attribute := range []*c.TextAttribute{&chars.DeliveryMethod, &chars.ProductType, &chars.OfferKind, &chars.LicenseType, &chars.Version, &chars.Edition, &chars.TermKind, &chars.Deployment} {
		attribute.Evidence = nil
		attribute.Reason = ""
	}
	for _, attribute := range []*c.SetAttribute{&chars.Platforms, &chars.Architectures} {
		attribute.Evidence = nil
		attribute.Reason = ""
	}
	for _, attribute := range []*c.IntegerAttribute{&chars.UsersLimit, &chars.DevicesLimit, &chars.IPAddressesLimit, &chars.ParticipantsLimit, &chars.Nodes, &chars.TermMonths} {
		attribute.Evidence = nil
		attribute.Reason = ""
	}
	return c.Offer{ID: o.ID, GroupID: o.GroupID, Name: o.Name, Vendor: o.Vendor, VendorCode: o.VendorCode, URL: o.URL, CategoryID: o.CategoryID, Available: o.Available, PriceMinor: o.PriceMinor, Description: o.Description, Characteristics: &chars}
}

func (f *fileIndexFeed) Read(ctx context.Context, after int64, size int) ([]a.Row, error) {
	if f.closed {
		return nil, os.ErrClosed
	}
	if size < 1 || size > 1000 || after < 0 {
		return nil, errors.New("invalid local index page")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if after < f.position {
		if _, err := f.file.Seek(0, io.SeekStart); err != nil {
			return nil, err
		}
		f.decoder = json.NewDecoder(bufio.NewReaderSize(f.file, 128<<10))
		f.position = 0
	}
	var rows []a.Row
	for len(rows) < size {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var row a.Row
		if err := f.decoder.Decode(&row); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, err
		}
		if row.Ordinal <= f.position {
			return nil, errors.New("nonmonotonic local index spool")
		}
		f.position = row.Ordinal
		if row.Ordinal > after {
			rows = append(rows, row)
		}
	}
	return rows, nil
}
func (f *fileIndexFeed) Close() error {
	if f.closed {
		return nil
	}
	f.closed = true
	var err error
	if f.file != nil {
		err = errors.Join(f.file.Close(), os.Remove(f.file.Name()))
	}
	if f.catalogFeed != nil {
		err = errors.Join(err, f.catalogFeed.Close())
	}
	return err
}
