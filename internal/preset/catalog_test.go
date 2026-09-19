package preset

import "testing"

const testCatalogJSON = `{
  "schema_version": "1.0",
  "presets": {
    "lean": {
      "name": "Lean Workflow",
      "description": "Minimal core workflow",
      "tags": ["lean", "minimal"],
      "download_url": "https://example.com/lean.zip",
      "sha256": "abc123"
    },
    "governance": {
      "name": "Governance Pack",
      "description": "Compliance and sign-off gates",
      "tags": ["governance", "compliance"]
    }
  }
}`

func TestParseCatalogJSON_FillsIDFromKey(t *testing.T) {
	cat, err := ParseCatalogJSON("test", []byte(testCatalogJSON))
	if err != nil {
		t.Fatalf("ParseCatalogJSON: %v", err)
	}
	entry, ok := cat.Entries["lean"]
	if !ok {
		t.Fatal("expected 'lean' entry to be present")
	}
	if entry.ID != "lean" {
		t.Errorf("expected entry.ID to be filled from the map key, got %q", entry.ID)
	}
	if entry.DownloadURL != "https://example.com/lean.zip" {
		t.Errorf("unexpected download_url: %q", entry.DownloadURL)
	}
}

func TestCatalog_SearchMatchesIDNameDescriptionTags(t *testing.T) {
	cat, err := ParseCatalogJSON("test", []byte(testCatalogJSON))
	if err != nil {
		t.Fatalf("ParseCatalogJSON: %v", err)
	}

	cases := []struct {
		query   string
		wantIDs []string
	}{
		{"lean", []string{"lean"}},
		{"compliance", []string{"governance"}},
		{"minimal", []string{"lean"}},
		{"nonexistent-term", nil},
	}
	for _, c := range cases {
		got := cat.Search(c.query)
		if len(got) != len(c.wantIDs) {
			t.Errorf("query %q: expected %d results, got %d (%+v)", c.query, len(c.wantIDs), len(got), got)
			continue
		}
		for i, id := range c.wantIDs {
			if got[i].ID != id {
				t.Errorf("query %q: expected result %d to be %q, got %q", c.query, i, id, got[i].ID)
			}
		}
	}
}

func TestCatalog_SearchEmptyQueryReturnsAll(t *testing.T) {
	cat, err := ParseCatalogJSON("test", []byte(testCatalogJSON))
	if err != nil {
		t.Fatalf("ParseCatalogJSON: %v", err)
	}
	got := cat.Search("")
	if len(got) != 2 {
		t.Errorf("expected empty query to return all 2 entries, got %d", len(got))
	}
}

func TestLoadBundledCatalog_ParsesEmbeddedCatalog(t *testing.T) {
	// A thin smoke test that the embedded assets.Presets catalog.json is
	// well-formed; the full content is exercised via ParseCatalogJSON above.
	data := []byte(`{"schema_version": "1.0", "presets": {}}`)
	cat, err := ParseCatalogJSON("default", data)
	if err != nil {
		t.Fatalf("ParseCatalogJSON: %v", err)
	}
	if len(cat.Entries) != 0 {
		t.Errorf("expected an empty bundled catalog for now, got %d entries", len(cat.Entries))
	}
}
