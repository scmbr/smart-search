package vectorcache

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestManifestRejectsExtraLabelsAndTrailingJSON(t *testing.T) {
	raw, err := json.Marshal(fullManifest())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "corpus.json")
	if err = os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = LoadManifest(path); err != nil {
		t.Fatal(err)
	}
	for _, bad := range [][]byte{append(append([]byte(nil), raw...), []byte(" {}")...), append(append([]byte(nil), raw[:len(raw)-1]...), []byte(",\"Queries\":[]}")...)} {
		if err = os.WriteFile(path, bad, 0600); err != nil {
			t.Fatal(err)
		}
		if _, err = LoadManifest(path); err == nil {
			t.Fatal("accepted non-manifest data")
		}
	}
}
