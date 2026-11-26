// Package families loads the reviewed-scope mapping; unresolved groups stay provisional.
package families

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"

	domain "github.com/scmbr/smart-search/internal/domain/search"
)

//go:embed rules.json
var rules []byte

func Load() (*domain.Families, string, error) {
	var config struct {
		Version string
		Rules   []domain.FamilyRule
	}
	if err := json.Unmarshal(rules, &config); err != nil {
		return nil, "", err
	}
	r, err := domain.NewFamilies(config.Rules)
	canonical, marshalErr := json.Marshal(config)
	if marshalErr != nil {
		return nil, "", marshalErr
	}
	h := sha256.Sum256(canonical)
	return r, config.Version + "-" + hex.EncodeToString(h[:]), err
}
