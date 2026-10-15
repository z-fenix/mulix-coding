package scaffold

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallState_DoesNotTrustEqualLegacyBytes(t *testing.T) {
	root := t.TempDir()
	rel := ".mulix/templates/spec-template.md"
	writeProjectFile(t, root, rel, "CUSTOM\n")
	writeProjectFile(t, root, ".mulix/.installed/"+rel, "CUSTOM\n")
	st, err := loadInstallState(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Files) != 0 || st.trustedVendor(rel, []byte("CUSTOM\n"), []byte("vendor\n")) {
		t.Fatal("legacy equality silently became trusted")
	}
}

func TestInstallState_RoundTripAndDigestProof(t *testing.T) {
	root := t.TempDir()
	rel := ".mulix/templates/unit.md"
	st, err := loadInstallState(root)
	if err != nil {
		t.Fatal(err)
	}
	st.Hosts = []Host{HostDSH}
	st.Files[rel] = vendorRecord{SHA256: contentDigest([]byte("vendor\n"))}
	if err := saveInstallState(root, st); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadInstallState(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Hosts) != 1 || loaded.Hosts[0] != HostDSH {
		t.Fatalf("hosts = %v", loaded.Hosts)
	}
	if !loaded.trustedVendor(rel, []byte("vendor\n"), []byte("new vendor\n")) {
		t.Fatal("matching recorded vendor was not trusted")
	}
	if loaded.trustedVendor(rel, []byte("edited baseline\n"), []byte("new vendor\n")) {
		t.Fatal("modified baseline retained trust")
	}
	if !loaded.trustedVendor(rel, []byte("new vendor\n"), []byte("new vendor\n")) {
		t.Fatal("exact incoming vendor was not provable")
	}
	if loaded.trustedVendor(rel, []byte{}, nil) {
		t.Fatal("unknown empty obsolete baseline was trusted")
	}
}

func TestInstallState_RejectsInvalidMetadata(t *testing.T) {
	validDigest := strings.Repeat("a", 64)
	cases := map[string]string{
		"malformed":       "{",
		"null":            "null",
		"missing schema":  `{"files":{}}`,
		"schema":          `{"schema":"future","hosts":[],"files":{}}`,
		"unknown field":   `{"schema":"mulix.install.v1","files":{},"extra":true}`,
		"trailing object": `{"schema":"mulix.install.v1","files":{}} {}`,
		"host":            `{"schema":"mulix.install.v1","hosts":["unknown"],"files":{}}`,
		"digest":          `{"schema":"mulix.install.v1","files":{".mulix/templates/unit.md":{"vendor_sha256":"short"}}}`,
		"outside path":    `{"schema":"mulix.install.v1","files":{"../outside":{"vendor_sha256":"` + validDigest + `"}}}`,
		"pending escape":  `{"schema":"mulix.install.v1","files":{},"pending":{".mulix/templates/unit.md":{"stage":"conflict","incoming_sha256":"` + validDigest + `","local_sha256":"` + validDigest + `","candidate":"../outside"}}}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			writeProjectFile(t, root, ".mulix/install-state.json", body)
			if _, err := loadInstallState(root); err == nil {
				t.Fatal("invalid metadata was accepted")
			}
		})
	}
}

func TestInstallState_RejectsSymlinkParent(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	writeProjectFile(t, outside, "install-state.json", `{"schema":"mulix.install.v1","files":{}}`)
	if err := os.Symlink(outside, filepath.Join(root, ".mulix")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := loadInstallState(root); err == nil {
		t.Fatal("metadata loader followed a symlink parent")
	}
	if err := saveInstallState(root, newInstallState()); err == nil {
		t.Fatal("metadata writer followed a symlink parent")
	}
}

func TestInstallState_ReadErrorIsNotMissing(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".mulix", "install-state.json"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := loadInstallState(root); err == nil {
		t.Fatal("metadata directory was silently treated as missing")
	}
}

func TestInstallState_PendingValidation(t *testing.T) {
	st := &installState{
		Schema: installSchema,
		Files:  map[string]vendorRecord{},
		Pending: map[string]pendingUpdate{
			".dsh/skills/mulix-using/SKILL.md": {
				Stage: "accepted-cleanup", IncomingSHA256: strings.Repeat("a", 64),
				LocalSHA256: strings.Repeat("b", 64), AcceptedSHA256: strings.Repeat("c", 64),
				BackupDir: ".mulix/.backups/update-123",
				Sources:   []pendingSource{{Rel: ".dsh/skills/using-mulix/SKILL.md", LocalSHA256: strings.Repeat("d", 64)}},
			},
		},
	}
	root := t.TempDir()
	if err := saveInstallState(root, st); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(root, ".mulix", "install-state.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range []string{".claude/skills/using-mulix/SKILL.md", ".dsh/skills/custom/SKILL.md"} {
		var invalid installState
		if err := json.Unmarshal(body, &invalid); err != nil {
			t.Fatal(err)
		}
		p := invalid.Pending[".dsh/skills/mulix-using/SKILL.md"]
		p.Sources[0].Rel = source
		invalid.Pending[".dsh/skills/mulix-using/SKILL.md"] = p
		if err := saveInstallState(root, &invalid); err == nil {
			t.Fatalf("foreign rename source accepted: %s", source)
		}
	}
}

func TestManagedRel_RejectsEscapesAndOverrides(t *testing.T) {
	for _, rel := range []string{"", "../outside", "/absolute", `C:\outside`, `.claude\skills\x`, ".mulix/templates/overrides/custom.md", ".mulix/templates/overrides", "AGENTS.md", ".mulix/.backups/x", ".dsh/skills/a/../../outside"} {
		if validManagedRel(rel) {
			t.Fatalf("accepted %q", rel)
		}
	}
	for _, rel := range []string{".claude/skills/a/SKILL.md", ".dsh/skills/a/scripts/run", ".mulix/templates/unit.md", ".mulix/superpowers/VERSION"} {
		if !validManagedRel(rel) {
			t.Fatalf("rejected managed path %q", rel)
		}
	}
}
