package preset

import (
	"testing"
	"time"
)

func TestRegistry_LoadWithNoFileReturnsEmpty(t *testing.T) {
	root := t.TempDir()

	reg, err := LoadRegistry(root)
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	if len(reg.Presets) != 0 {
		t.Fatalf("expected empty registry, got %+v", reg.Presets)
	}
}

func TestRegistry_AddSaveLoadRoundTrip(t *testing.T) {
	root := t.TempDir()

	entry := Entry{Version: "1.0.0", Source: "local", ManifestHash: "abc", Enabled: true, Priority: 5, InstalledAt: time.Now().UTC()}
	if err := Add(root, "lean", entry); err != nil {
		t.Fatalf("Add: %v", err)
	}

	reg, err := LoadRegistry(root)
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	got, ok := reg.Presets["lean"]
	if !ok {
		t.Fatal("expected 'lean' entry to round-trip")
	}
	if got.Version != "1.0.0" || got.Priority != 5 {
		t.Errorf("unexpected round-tripped entry: %+v", got)
	}
}

func TestRegistry_RemoveUnknownFails(t *testing.T) {
	root := t.TempDir()
	if err := Remove(root, "nope"); err == nil {
		t.Fatal("expected Remove to fail for an unknown preset")
	}
}

func TestRegistry_SetEnabledAndPriority(t *testing.T) {
	root := t.TempDir()
	if err := Add(root, "lean", Entry{Version: "1.0.0", Priority: 10, Enabled: true}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	if err := SetEnabled(root, "lean", false); err != nil {
		t.Fatalf("SetEnabled: %v", err)
	}
	if err := SetPriority(root, "lean", 1); err != nil {
		t.Fatalf("SetPriority: %v", err)
	}

	reg, err := LoadRegistry(root)
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	if reg.Presets["lean"].Enabled {
		t.Error("expected 'lean' to be disabled")
	}
	if reg.Presets["lean"].Priority != 1 {
		t.Errorf("expected priority 1, got %d", reg.Presets["lean"].Priority)
	}
}

func TestListByPriority_SortsByPriorityThenID(t *testing.T) {
	root := t.TempDir()
	if err := Add(root, "b-preset", Entry{Priority: 10, Enabled: true}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := Add(root, "a-preset", Entry{Priority: 10, Enabled: true}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := Add(root, "top-priority", Entry{Priority: 1, Enabled: true}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := Add(root, "disabled", Entry{Priority: 0, Enabled: false}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	reg, err := LoadRegistry(root)
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}

	got := ListByPriority(reg, false)
	want := []string{"top-priority", "a-preset", "b-preset"}
	if len(got) != len(want) {
		t.Fatalf("expected %d enabled presets, got %d: %+v", len(want), len(got), got)
	}
	for i, id := range want {
		if got[i].ID != id {
			t.Errorf("position %d: expected %q, got %q", i, id, got[i].ID)
		}
	}

	all := ListByPriority(reg, true)
	if len(all) != 4 {
		t.Fatalf("expected 4 entries when including disabled, got %d", len(all))
	}
}
