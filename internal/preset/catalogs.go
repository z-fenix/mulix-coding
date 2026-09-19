package preset

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

// catalogsFile records the extra (non-bundled) catalogs a project has
// added via `mulix preset catalog add`, the mulix equivalent of spec-kit's
// .specify/preset-catalogs.yml (kept as JSON here for consistency with the
// rest of mulix's own config files rather than introducing YAML for a
// single small list).
const catalogsFile = ".mulix/presets/catalogs.json"

// CatalogRef is one configured catalog: a name plus the URL its JSON is
// fetched from. The bundled catalog is not stored here; it always exists
// implicitly as "default".
type CatalogRef struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

func catalogsPath(root string) string {
	return filepath.Join(root, catalogsFile)
}

// LoadCatalogRefs returns the extra catalogs configured for root. A
// missing file means none have been added yet, not an error.
func LoadCatalogRefs(root string) ([]CatalogRef, error) {
	path := catalogsPath(root)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("preset: reading %s: %w", path, err)
	}
	var refs []CatalogRef
	if err := json.Unmarshal(data, &refs); err != nil {
		return nil, fmt.Errorf("preset: decoding %s: %w", path, err)
	}
	return refs, nil
}

func saveCatalogRefs(root string, refs []CatalogRef) error {
	path := catalogsPath(root)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("preset: creating %s: %w", filepath.Dir(path), err)
	}
	data, err := json.MarshalIndent(refs, "", "  ")
	if err != nil {
		return fmt.Errorf("preset: encoding catalogs: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("preset: writing %s: %w", path, err)
	}
	return nil
}

// AddCatalogRef records a new named catalog, rejecting a duplicate name
// (including "default", which is reserved for the bundled catalog).
func AddCatalogRef(root, name, url string) error {
	if name == "default" {
		return fmt.Errorf("preset: %q is reserved for the bundled catalog", name)
	}
	refs, err := LoadCatalogRefs(root)
	if err != nil {
		return err
	}
	for _, r := range refs {
		if r.Name == name {
			return fmt.Errorf("preset: catalog %q already configured (url: %s)", name, r.URL)
		}
	}
	refs = append(refs, CatalogRef{Name: name, URL: url})
	return saveCatalogRefs(root, refs)
}

// RemoveCatalogRef deletes a configured catalog by name.
func RemoveCatalogRef(root, name string) error {
	refs, err := LoadCatalogRefs(root)
	if err != nil {
		return err
	}
	out := refs[:0]
	found := false
	for _, r := range refs {
		if r.Name == name {
			found = true
			continue
		}
		out = append(out, r)
	}
	if !found {
		return fmt.Errorf("preset: catalog %q is not configured", name)
	}
	return saveCatalogRefs(root, out)
}

// FetchCatalog downloads and parses the catalog at ref.URL.
func FetchCatalog(ref CatalogRef) (Catalog, error) {
	resp, err := http.Get(ref.URL)
	if err != nil {
		return Catalog{}, fmt.Errorf("preset: fetching catalog %q: %w", ref.Name, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Catalog{}, fmt.Errorf("preset: fetching catalog %q: unexpected status %s", ref.Name, resp.Status)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return Catalog{}, fmt.Errorf("preset: reading catalog %q response: %w", ref.Name, err)
	}
	return ParseCatalogJSON(ref.Name, data)
}
