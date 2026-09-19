package preset

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"strings"
)

// CatalogEntry is one preset's discovery metadata, the mulix equivalent of
// one entry in spec-kit's catalog.json/catalog.community.json. mulix ships
// only a metadata catalog (no bundled preset content the way spec-kit
// bundles lean/constitution-sync): DownloadURL is how `preset add <id>`
// resolves an id from the catalog into something InstallFromURL can fetch.
type CatalogEntry struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	DownloadURL string   `json:"download_url,omitempty"`
	SHA256      string   `json:"sha256,omitempty"`
}

// Catalog is a named, ordered set of entries. Name is shown by `preset
// catalog list`.
type Catalog struct {
	Name    string
	Entries map[string]CatalogEntry
}

// catalogFile is the on-disk/embedded JSON shape: {"schema_version": "1.0", "presets": {...}}.
type catalogFile struct {
	SchemaVersion string                  `json:"schema_version"`
	Presets       map[string]CatalogEntry `json:"presets"`
}

// LoadBundledCatalog reads mulix's built-in catalog from the embedded
// assets.Presets filesystem (assets/presets/catalog.json).
func LoadBundledCatalog(presetsFS fs.FS) (Catalog, error) {
	data, err := fs.ReadFile(presetsFS, "presets/catalog.json")
	if err != nil {
		return Catalog{}, fmt.Errorf("preset: reading bundled catalog: %w", err)
	}
	return parseCatalog("default", data)
}

// ParseCatalogJSON parses raw catalog JSON data (as fetched from a custom
// catalog URL) into a Catalog named name.
func ParseCatalogJSON(name string, data []byte) (Catalog, error) {
	return parseCatalog(name, data)
}

func parseCatalog(name string, data []byte) (Catalog, error) {
	var cf catalogFile
	if err := json.Unmarshal(data, &cf); err != nil {
		return Catalog{}, fmt.Errorf("preset: decoding catalog %q: %w", name, err)
	}
	if cf.Presets == nil {
		cf.Presets = map[string]CatalogEntry{}
	}
	for id, entry := range cf.Presets {
		entry.ID = id
		cf.Presets[id] = entry
	}
	return Catalog{Name: name, Entries: cf.Presets}, nil
}

// Search returns every entry whose id, name, description, or tags contain
// query (case-insensitive). An empty query returns every entry.
func (c Catalog) Search(query string) []CatalogEntry {
	q := strings.ToLower(strings.TrimSpace(query))
	var out []CatalogEntry
	for _, e := range c.Entries {
		if q == "" || matchesCatalogEntry(e, q) {
			out = append(out, e)
		}
	}
	return out
}

func matchesCatalogEntry(e CatalogEntry, q string) bool {
	if strings.Contains(strings.ToLower(e.ID), q) ||
		strings.Contains(strings.ToLower(e.Name), q) ||
		strings.Contains(strings.ToLower(e.Description), q) {
		return true
	}
	for _, tag := range e.Tags {
		if strings.Contains(strings.ToLower(tag), q) {
			return true
		}
	}
	return false
}
