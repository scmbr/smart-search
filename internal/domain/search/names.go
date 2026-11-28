package search

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

const NamePolicyVersion = "catalog-names-v1"

type NameEntity struct{ Kind, Name, Scope string }
type NameAlias struct{ Alias, Name, Kind string }
type NameCandidate struct {
	Entity   NameEntity
	Distance int
}
type NameResolution struct {
	Status        string
	Candidates    []NameCandidate
	DictionarySHA string
}
type nameKey struct {
	Text   string
	Entity NameEntity
}
type NameDictionary struct {
	keys []nameKey
	sha  string
}

func nameNormalized(s string) string {
	return strings.ReplaceAll(strings.ToLower(strings.Join(strings.Fields(s), " ")), "ё", "е")
}

// NewNameDictionary copies and sorts catalog identities. Aliases without catalog targets
// are ignored; they can never manufacture products absent from this snapshot.
func NewNameDictionary(snapshot string, entities []NameEntity, aliases []NameAlias) (*NameDictionary, error) {
	if snapshot == "" {
		return nil, errors.New("missing name dictionary snapshot")
	}
	d := &NameDictionary{}
	seen := map[nameKey]bool{}
	add := func(text string, e NameEntity) {
		k := nameKey{nameNormalized(text), e}
		if k.Text != "" && !seen[k] {
			seen[k] = true
			d.keys = append(d.keys, k)
		}
	}
	for _, e := range entities {
		if (e.Kind != "brand" && e.Kind != "product") || strings.TrimSpace(e.Name) == "" || !strings.HasPrefix(e.Scope, "/") || !strings.HasSuffix(e.Scope, "/") || e.Scope == "/" {
			return nil, errors.New("invalid catalog name identity")
		}
		add(e.Name, e)
		for _, a := range aliases {
			if a.Kind == e.Kind && nameNormalized(a.Name) == nameNormalized(e.Name) {
				add(a.Alias, e)
			}
		}
	}
	sort.Slice(d.keys, func(i, j int) bool {
		a, b := d.keys[i], d.keys[j]
		if a.Text != b.Text {
			return a.Text < b.Text
		}
		if a.Entity.Kind != b.Entity.Kind {
			return a.Entity.Kind < b.Entity.Kind
		}
		if a.Entity.Scope != b.Entity.Scope {
			return a.Entity.Scope < b.Entity.Scope
		}
		return a.Entity.Name < b.Entity.Name
	})
	data, _ := json.Marshal(struct {
		Snapshot, Policy string
		Keys             []nameKey
	}{snapshot, NamePolicyVersion, d.keys})
	sum := sha256.Sum256(data)
	d.sha = hex.EncodeToString(sum[:])
	return d, nil
}
func (d *NameDictionary) Fingerprint() string { return d.sha }

// ResolveName interprets only complete short names. It never drops conditions,
// corrects identifiers, or chooses one of equally plausible catalog identities.
func (d *NameDictionary) ResolveName(query string) NameResolution {
	r := NameResolution{Status: "unresolved", DictionarySHA: d.sha}
	q := nameNormalized(query)
	if utf8.RuneCountInString(q) < 4 || utf8.RuneCountInString(q) > 80 || len(strings.Fields(q)) > 4 || strings.ContainsFunc(q, func(c rune) bool { return !unicode.IsLetter(c) && !unicode.IsSpace(c) }) {
		return r
	}
	best := map[NameEntity]int{}
	for _, k := range d.keys {
		distance := nameDistance(q, k.Text)
		// Transliteration is a secondary hypothesis, never stronger than exact spelling.
		if latin := toLatin(q); latin != q {
			distance = min(distance, nameDistance(latin, k.Text)+1)
		}
		limit := 10
		if utf8.RuneCountInString(q) >= 6 {
			limit = 20
		}
		if distance > limit {
			continue
		}
		if previous, ok := best[k.Entity]; !ok || distance < previous {
			best[k.Entity] = distance
		}
	}
	for e, distance := range best {
		r.Candidates = append(r.Candidates, NameCandidate{e, distance})
	}
	sort.Slice(r.Candidates, func(i, j int) bool {
		a, b := r.Candidates[i], r.Candidates[j]
		if a.Distance != b.Distance {
			return a.Distance < b.Distance
		}
		if a.Entity.Kind != b.Entity.Kind {
			return a.Entity.Kind < b.Entity.Kind
		}
		if a.Entity.Name != b.Entity.Name {
			return a.Entity.Name < b.Entity.Name
		}
		return a.Entity.Scope < b.Entity.Scope
	})
	if len(r.Candidates) == 0 {
		return r
	}
	r.Status = "resolved"
	if len(r.Candidates) > 1 && r.Candidates[1].Distance-r.Candidates[0].Distance < 5 {
		r.Status = "ambiguous"
	}
	if len(r.Candidates) > 5 {
		r.Candidates = r.Candidates[:5]
	}
	return r
}

// Bounded optimal-string-alignment distance includes adjacent transpositions and
// errors in the first character; scores use 10 units per edit.
func nameDistance(a, b string) int {
	x, y := []rune(a), []rune(b)
	if absName(len(x)-len(y)) > 2 {
		return 1000
	}
	if len(x) > 80 || len(y) > 80 {
		return 1000
	}
	m := make([][]int, len(x)+1)
	for i := range m {
		m[i] = make([]int, len(y)+1)
		m[i][0] = i
	}
	for j := range m[0] {
		m[0][j] = j
	}
	for i := 1; i <= len(x); i++ {
		for j := 1; j <= len(y); j++ {
			cost := 0
			if x[i-1] != y[j-1] {
				cost = 1
			}
			m[i][j] = min(m[i-1][j]+1, m[i][j-1]+1, m[i-1][j-1]+cost)
			if i > 1 && j > 1 && x[i-1] == y[j-2] && x[i-2] == y[j-1] {
				m[i][j] = min(m[i][j], m[i-2][j-2]+1)
			}
		}
	}
	return m[len(x)][len(y)] * 10
}
func absName(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
