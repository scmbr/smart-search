package vectorcache

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"

	d "github.com/scmbr/smart-search/internal/domain/vectorcorpus"
)

func LoadManifest(path string) (d.Corpus, error) {
	var x d.Corpus
	f, err := os.Open(path)
	if err != nil {
		return x, err
	}
	raw, readErr := io.ReadAll(io.LimitReader(f, 64*1024*1024+1))
	closeErr := f.Close()
	if readErr != nil || closeErr != nil || len(raw) > 64*1024*1024 {
		return x, errors.New("cannot read bounded vector corpus")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&x); err != nil {
		return x, errors.New("invalid vector corpus JSON")
	}
	if err = decoder.Decode(new(any)); err != io.EOF {
		return x, errors.New("trailing vector corpus JSON")
	}
	return x, d.Validate(x)
}
