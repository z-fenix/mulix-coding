package cliutil

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/z-fenix/mulix-coding/internal/scaffold"
)

func TestUpdateReview_WindowsSharingViolationReportsCompletedLocalWrite(t *testing.T) {
	root := chdirTemp(t)
	if _, err := scaffold.Init(scaffold.InitOptions{Root: root, Hosts: []scaffold.Host{scaffold.HostDSH}}); err != nil {
		t.Fatal(err)
	}
	rel := ".mulix/templates/spec-template.md"
	vendor, err := os.ReadFile(filepath.FromSlash(rel))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.FromSlash(rel), []byte("CUSTOM LOCAL\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(root, ".mulix", ".installed", filepath.FromSlash(rel))
	if err := os.WriteFile(base, []byte("OLD VENDOR\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	wide, err := syscall.UTF16PtrFromString(base)
	if err != nil {
		t.Fatal(err)
	}
	// Keep B readable for preflight/backup while denying replacement/deletion.
	handle, err := syscall.CreateFile(wide, syscall.GENERIC_READ, syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE, nil, syscall.OPEN_EXISTING, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer syscall.CloseHandle(handle)
	out, err := runPresetCLI(t, "update", "--force")
	if err == nil {
		t.Fatalf("locked baseline unexpectedly replaced: %s", out)
	}
	local, readErr := os.ReadFile(filepath.FromSlash(rel))
	if readErr != nil || string(local) != string(vendor) {
		t.Fatalf("local write not completed: %v", readErr)
	}
	if !strings.Contains(out, "1 partial") || !strings.Contains(out, "- partial: "+rel) || !strings.Contains(out, "local written; failed: vendor baseline") || !strings.Contains(out, "backup:") {
		t.Fatalf("same-file mutation hidden: %s\nerror: %v", out, err)
	}
}
