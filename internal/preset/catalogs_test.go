package preset

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCatalogRefs_AddListRemove(t *testing.T) {
	root := t.TempDir()

	if err := AddCatalogRef(root, "community", "https://example.com/catalog.json"); err != nil {
		t.Fatalf("AddCatalogRef: %v", err)
	}

	refs, err := LoadCatalogRefs(root)
	if err != nil {
		t.Fatalf("LoadCatalogRefs: %v", err)
	}
	if len(refs) != 1 || refs[0].Name != "community" {
		t.Fatalf("expected one 'community' catalog ref, got %+v", refs)
	}

	if err := RemoveCatalogRef(root, "community"); err != nil {
		t.Fatalf("RemoveCatalogRef: %v", err)
	}
	refs, err = LoadCatalogRefs(root)
	if err != nil {
		t.Fatalf("LoadCatalogRefs: %v", err)
	}
	if len(refs) != 0 {
		t.Fatalf("expected no catalog refs after removal, got %+v", refs)
	}
}

func TestCatalogRefs_RejectsReservedNameAndDuplicate(t *testing.T) {
	root := t.TempDir()

	if err := AddCatalogRef(root, "default", "https://example.com/catalog.json"); err == nil {
		t.Fatal("expected AddCatalogRef to reject the reserved name 'default'")
	}

	if err := AddCatalogRef(root, "community", "https://example.com/a.json"); err != nil {
		t.Fatalf("AddCatalogRef: %v", err)
	}
	if err := AddCatalogRef(root, "community", "https://example.com/b.json"); err == nil {
		t.Fatal("expected AddCatalogRef to reject a duplicate name")
	}
}

func TestCatalogRefs_RemoveUnknownFails(t *testing.T) {
	root := t.TempDir()
	if err := RemoveCatalogRef(root, "nope"); err == nil {
		t.Fatal("expected RemoveCatalogRef to fail for an unconfigured catalog")
	}
}

func TestFetchCatalog_DownloadsAndParses(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(testCatalogJSON))
	}))
	defer server.Close()

	cat, err := FetchCatalog(CatalogRef{Name: "community", URL: server.URL})
	if err != nil {
		t.Fatalf("FetchCatalog: %v", err)
	}
	if len(cat.Entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(cat.Entries))
	}
}
