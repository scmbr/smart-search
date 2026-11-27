// Package embeddingpilot defines bounded, candidate-only embedding experiments.
package embeddingpilot

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

const Model = "text-embedding-3-small"
const Dimensions = 1536
const TextVersion = "embedding-text-v1"
const ProjectCapNano int64 = 5000000000
const RunCapNano int64 = 50000000

type Document struct{ ID, GroupID, Vendor, Text, SourceURL string }
type Query struct {
	ID, Text, Scope, Split, Vendor string
	Groups                         []string
}
type Dataset struct {
	Version, Role, SourceSHA, TuningSHA      string
	FullGroups, FullOffers                   int
	FullGroupTokenBound, FullOfferTokenBound int64
	Documents                                []Document
	Queries                                  []Query
}
type Rank struct {
	ID    string
	Score float64
}

func TokenBound(text string) (int64, error) {
	if !utf8.ValidString(text) || strings.TrimSpace(text) == "" || len(text) > 2048 {
		return 0, errors.New("invalid pilot text")
	}
	return int64(len(text) + 16), nil
}
func Cost(tokens int64) int64 { return tokens * 20 }
func CacheKey(text string) string {
	h := sha256.Sum256([]byte(TextVersion + "\x00" + Model + "\x00" + strconv.Itoa(Dimensions) + "\x00" + text))
	return hex.EncodeToString(h[:])
}
func Fingerprint(x Dataset) string {
	b, _ := json.Marshal(x)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func Validate(x Dataset) error {
	if x.FullGroups < 0 || x.FullOffers < 0 || x.FullGroupTokenBound < 0 || x.FullOfferTokenBound < 0 || x.FullGroupTokenBound > math.MaxInt64/20 || x.FullOfferTokenBound > math.MaxInt64/20 {
		return errors.New("invalid full-corpus projection")
	}
	if x.Version != TextVersion || x.Role != "development_tuning_pilot" || len(x.Documents) < 1 || len(x.Documents) > 512 || len(x.Queries) < 1 || len(x.Queries) > 100 {
		return errors.New("invalid pilot scope")
	}
	if b, err := hex.DecodeString(x.SourceSHA); err != nil || len(b) != 32 {
		return errors.New("invalid feed SHA")
	}
	seen := map[string]bool{}
	for _, d := range x.Documents {
		if seen[d.ID] || d.ID == "" || d.GroupID == "" || d.Vendor == "" {
			return errors.New("invalid/duplicate document")
		}
		seen[d.ID] = true
		if _, err := TokenBound(d.Text); err != nil {
			return err
		}
	}
	seen = map[string]bool{}
	for _, q := range x.Queries {
		if seen[q.ID] || !(q.Split == "tuning" && strings.HasPrefix(q.ID, "tune-") || q.Split == "pilot_probe" && strings.HasPrefix(q.ID, "probe-")) {
			return errors.New("non-development query or duplicate ID")
		}
		seen[q.ID] = true
		if q.Scope != "family" && q.Scope != "brand" && q.Scope != "empty" || q.Scope == "family" && len(q.Groups) == 0 || q.Scope == "brand" && q.Vendor == "" {
			return errors.New("invalid target scope")
		}
		if _, err := TokenBound(q.Text); err != nil {
			return err
		}
	}
	return nil
}
func Cosine(a, b []float64) (float64, error) {
	if len(a) == 0 || len(a) != len(b) {
		return 0, errors.New("vector dimensions differ")
	}
	var dot, aa, bb float64
	for i, v := range a {
		if math.IsNaN(v) || math.IsInf(v, 0) || math.IsNaN(b[i]) || math.IsInf(b[i], 0) {
			return 0, errors.New("non-finite vector")
		}
		dot += v * b[i]
		aa += v * v
		bb += b[i] * b[i]
	}
	if aa == 0 || bb == 0 {
		return 0, errors.New("zero vector")
	}
	score := dot / math.Sqrt(aa*bb)
	if math.IsNaN(score) || math.IsInf(score, 0) || math.IsInf(aa*bb, 0) {
		return 0, errors.New("vector arithmetic overflow")
	}
	return score, nil
}
func ValidateVector(v []float64) error {
	if len(v) != Dimensions {
		return errors.New("invalid embedding length")
	}
	_, err := Cosine(v, v)
	return err
}
func RankVectors(docs []Document, vectors map[string][]float64, q string, dim int) ([]Rank, error) {
	query := vectors[CacheKey(q)]
	if dim < 1 || dim > len(query) {
		return nil, errors.New("missing query embedding")
	}
	var ranks []Rank
	for _, doc := range docs {
		v := vectors[CacheKey(doc.Text)]
		if len(v) < dim {
			return nil, errors.New("missing document embedding")
		}
		s, err := Cosine(query[:dim], v[:dim])
		if err != nil {
			return nil, err
		}
		ranks = append(ranks, Rank{ID: doc.ID, Score: s})
	}
	sort.Slice(ranks, func(i, j int) bool {
		if ranks[i].Score == ranks[j].Score {
			return ranks[i].ID < ranks[j].ID
		}
		return ranks[i].Score > ranks[j].Score
	})
	if len(ranks) > 4 {
		ranks = ranks[:4]
	}
	return ranks, nil
}
