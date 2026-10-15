package fileutil

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestWriteFile_ReplaceFailureKeepsOriginal(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "state.yaml")
	if err := os.WriteFile(target, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	failure := errors.New("replacement failed")
	err := writeFileWith(target, []byte("new\n"), 0o644, writeAndSync,
		func(string, string) error { return failure })
	if !errors.Is(err, failure) {
		t.Fatalf("error = %v", err)
	}
	assertContent(t, target, "old\n")
	assertNoTemporaryFiles(t, dir)
}

func TestWriteFile_WriteFailureKeepsOriginal(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "active")
	if err := os.WriteFile(target, []byte("original\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	failure := errors.New("write failed")
	replaced := false
	err := writeFileWith(target, []byte("replacement\n"), 0o644,
		func(f *os.File, data []byte) error {
			if _, err := f.Write(data[:3]); err != nil {
				return err
			}
			return failure
		}, func(string, string) error {
			replaced = true
			return nil
		})
	if !errors.Is(err, failure) || replaced {
		t.Fatalf("write error = %v, replaced = %v", err, replaced)
	}
	assertContent(t, target, "original\n")
	assertNoTemporaryFiles(t, dir)
}

func TestWriteFile_CreatesParentsAndReplacesWholeContent(t *testing.T) {
	target := filepath.Join(t.TempDir(), "nested", "state.yaml")
	for _, content := range []string{"a long old value\n", "short\n", "an even longer replacement value\n"} {
		if err := WriteFile(target, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		assertContent(t, target, content)
		assertNoTemporaryFiles(t, filepath.Dir(target))
	}
}

func TestWriteFile_RejectsDirectoryTarget(t *testing.T) {
	target := t.TempDir()
	if err := WriteFile(target, []byte("do not replace directory"), 0o644); err == nil {
		t.Fatal("directory target was accepted")
	}
	info, err := os.Stat(target)
	if err != nil || !info.IsDir() {
		t.Fatalf("directory changed: %v", err)
	}
}

func TestWriteFile_RejectsSymlinkTarget(t *testing.T) {
	dir := t.TempDir()
	original := filepath.Join(dir, "original")
	link := filepath.Join(dir, "link")
	if err := os.WriteFile(original, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(original, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := WriteFile(link, []byte("replacement"), 0o644); err == nil {
		t.Fatal("symlink target was accepted")
	}
	assertContent(t, original, "keep")
}

func TestWriteFile_AppliesRequestedPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no Unix permission bits")
	}
	target := filepath.Join(t.TempDir(), "script")
	if err := WriteFile(target, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(target)
	if err != nil || info.Mode().Perm() != 0o755 {
		t.Fatalf("script permissions = %v, error = %v", info, err)
	}
}

func assertContent(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil || string(data) != want {
		t.Fatalf("%s = %q, want %q (error %v)", path, data, want, err)
	}
}

func assertNoTemporaryFiles(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if len(entry.Name()) >= len(".mulix-write-") && entry.Name()[:len(".mulix-write-")] == ".mulix-write-" {
			t.Fatalf("temporary file leaked: %s", entry.Name())
		}
	}
}
