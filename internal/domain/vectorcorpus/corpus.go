// Package vectorcorpus owns the versioned, label-free catalog embedding manifest.
package vectorcorpus

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"hash"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	embedding "github.com/scmbr/smart-search/internal/domain/embeddingpilot"
)

const Version = "vector-corpus-v1"
const MaxDocuments = 20000
const MaxRunNano int64 = 150000000

type Document struct{ ID, GroupID, OfferID, Vendor, Text, SourceURL string }
type Corpus struct {
	Version, TextVersion, Model string
	Dimensions                  int
	SourceSHA                   string
	Offers                      int
	Documents                   []Document
	SHA                         string
}

var vendorPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,127}$`)

func digestField(h hash.Hash, s string) {
	var n [4]byte
	binary.BigEndian.PutUint32(n[:], uint32(len(s)))
	_, _ = h.Write(n[:])
	_, _ = h.Write([]byte(s))
}

// Fingerprint uses length-prefixed UTF-8 fields, independent of JSON formatting.
func Fingerprint(x Corpus) string {
	h := sha256.New()
	for _, s := range []string{x.Version, x.TextVersion, x.Model, strconv.Itoa(x.Dimensions), x.SourceSHA, strconv.Itoa(x.Offers), strconv.Itoa(len(x.Documents))} {
		digestField(h, s)
	}
	for _, d := range x.Documents {
		for _, s := range []string{d.ID, d.GroupID, d.OfferID, d.Vendor, d.Text, d.SourceURL} {
			digestField(h, s)
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}
func Validate(x Corpus) error {
	bad := errors.New("invalid vector corpus manifest")
	if x.Version != Version || x.TextVersion != embedding.TextVersion || x.Model != embedding.Model || x.Dimensions != embedding.Dimensions || len(x.Documents) < 1 || len(x.Documents) > MaxDocuments || x.Offers < len(x.Documents) {
		return bad
	}
	if b, err := hex.DecodeString(x.SourceSHA); err != nil || len(b) != 32 || x.SourceSHA != strings.ToLower(x.SourceSHA) {
		return bad
	}
	previous := ""
	for _, d := range x.Documents {
		key := d.Vendor + "/group/" + d.GroupID
		if d.OfferID != "" {
			key = d.Vendor + "/offer/" + d.OfferID
		}
		if (d.GroupID == "") == (d.OfferID == "") || len(d.GroupID) > 512 || len(d.OfferID) > 512 || !utf8.ValidString(d.GroupID+d.OfferID) || d.ID != key || d.ID <= previous || !vendorPattern.MatchString(d.Vendor) {
			return bad
		}
		u, err := url.Parse(d.SourceURL)
		if err != nil || len(d.SourceURL) > 8192 || u.Scheme != "https" || u.Host != "store.softline.ru" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || !strings.HasPrefix(u.Path, "/"+d.Vendor+"/") {
			return bad
		}
		if _, err := embedding.TokenBound(d.Text); err != nil {
			return err
		}
		previous = d.ID
	}
	if x.SHA != Fingerprint(x) {
		return errors.New("vector corpus fingerprint mismatch")
	}
	return nil
}
