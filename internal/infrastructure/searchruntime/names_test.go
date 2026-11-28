package searchruntime

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestNamesRejectInvalidFilesBeforeDependencies(t *testing.T) {
	for _, body := range []string{`{}`, `{"Version":"wrong"}`, `{"Version":"name-catalog-v1","SourceSHA":"bad","Entries":[{}]}`, string(make([]byte, (4<<20)+1))} {
		path := filepath.Join(t.TempDir(), "names.json")
		if err := os.WriteFile(path, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := WithNames(context.Background(), nil, path, nil, nil); err == nil {
			t.Fatal("invalid manifest accepted")
		}
	}
}
