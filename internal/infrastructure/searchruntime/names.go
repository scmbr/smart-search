package searchruntime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5/pgxpool"
	names "github.com/scmbr/smart-search/internal/application/names"
	catalog "github.com/scmbr/smart-search/internal/domain/catalog"
	d "github.com/scmbr/smart-search/internal/domain/search"
	"io"
	"os"
	"strings"
)

// WithNames loads a bounded compact vocabulary once, without reading offer payloads.
func WithNames(ctx context.Context, pool *pgxpool.Pool, path string, named, fallback names.Backend) (names.SearchBackend, error) {
	var result names.SearchBackend
	f, err := os.Open(path)
	if err != nil {
		return result, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return result, err
	}
	if info.Size() > 4<<20 {
		return result, errors.New("name catalog exceeds 4 MiB")
	}
	raw, err := io.ReadAll(io.LimitReader(f, (4<<20)+1))
	if err != nil {
		return result, err
	}
	if len(raw) > 4<<20 {
		return result, errors.New("name catalog exceeds 4 MiB")
	}
	var manifest struct {
		Version, SourceSHA string
		Entries            []struct {
			Entity d.NameEntity
			Groups []string
		}
	}
	if err = json.Unmarshal(raw, &manifest); err != nil {
		return result, err
	}
	if manifest.Version != "name-catalog-v1" || len(manifest.Entries) == 0 || len(manifest.SourceSHA) != 64 {
		return result, errors.New("invalid name catalog")
	}
	if _, err = hex.DecodeString(manifest.SourceSHA); err != nil || pool == nil {
		return result, errors.New("invalid name catalog source")
	}
	var catalogID, schema string
	err = pool.QueryRow(ctx, `SELECT s.id,s.schema_version FROM catalog_snapshots s JOIN catalog_active a ON a.snapshot_id=s.id WHERE s.status='ready' AND s.source_sha256=$1`, manifest.SourceSHA).Scan(&catalogID, &schema)
	if err != nil || !strings.HasSuffix(schema, "/"+catalog.NormalizationVersion) {
		return result, errors.New("name catalog differs from active snapshot")
	}
	entities := []d.NameEntity{}
	groups := map[d.NameEntity][]string{}
	aliases := []d.NameAlias{{Alias: "адоб", Name: "Adobe Systems", Kind: "brand"}, {Alias: "Adobe", Name: "Adobe Systems", Kind: "brand"}, {Alias: "адоб", Name: "Adobe", Kind: "brand"}, {Alias: "фотошоп", Name: "Adobe Photoshop", Kind: "product"}}
	for _, entry := range manifest.Entries {
		if _, exists := groups[entry.Entity]; exists {
			return result, errors.New("duplicate name entity")
		}
		if entry.Entity.Kind == "product" && len(entry.Groups) == 0 {
			return result, errors.New("missing product groups")
		}
		for _, group := range entry.Groups {
			if group == "" || len(group) > 512 {
				return result, errors.New("invalid product group")
			}
		}
		entities = append(entities, entry.Entity)
		groups[entry.Entity] = entry.Groups
	}
	// Derive vendor-less names using catalog vendors and their configured aliases.
	prefixes := map[string][]string{}
	for _, brand := range entities {
		if brand.Kind != "brand" {
			continue
		}
		prefixes[brand.Scope] = append(prefixes[brand.Scope], brand.Name)
		for _, a := range aliases {
			if a.Kind == "brand" && a.Name == brand.Name {
				prefixes[brand.Scope] = append(prefixes[brand.Scope], a.Alias)
			}
		}
	}
	for _, e := range entities {
		if e.Kind != "product" {
			continue
		}
		for _, prefix := range prefixes[e.Scope] {
			if short, ok := strings.CutPrefix(e.Name, prefix+" "); ok && len(short) >= 4 {
				aliases = append(aliases, d.NameAlias{Alias: short, Name: e.Name, Kind: "product"})
			}
		}
	}
	sum := sha256.Sum256(raw)
	dictionary, err := d.NewNameDictionary(catalogID+"/"+hex.EncodeToString(sum[:]), entities, aliases)
	if err != nil {
		return result, err
	}
	return names.SearchBackend{Dictionary: dictionary, CatalogID: catalogID, Groups: groups, Named: named, Fallback: fallback}, nil
}
