// Package searchsession owns saved search identities and stable replay records.
package searchsession

import (
	"errors"
	s "github.com/scmbr/smart-search/internal/domain/search"
	"regexp"
)

var ErrInvalid = errors.New("invalid search request")
var ErrConflict = errors.New("idempotency conflict")
var ErrUnavailable = errors.New("search temporarily unavailable")
var idPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)
var hashPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

func ValidID(value string) bool { return idPattern.MatchString(value) }

type Identity struct{ Session, Key, BodyHash string }

func (i Identity) Validate() error {
	if !ValidID(i.Session) || !ValidID(i.Key) || !hashPattern.MatchString(i.BodyHash) {
		return ErrInvalid
	}
	return nil
}

type Record struct {
	ID         string
	DurationMS int64
	Result     s.Result
}
