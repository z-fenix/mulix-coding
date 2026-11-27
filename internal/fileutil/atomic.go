// Package fileutil prepares complete files before replacing their targets.
package fileutil

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// WriteFile writes and syncs a same-directory temporary file before replacing
// path. It never truncates or removes the old target first. Replacement is
// atomic where the filesystem supports it; Go does not guarantee atomic
// Rename on non-Unix platforms. This is not a lock or multi-file transaction.
func WriteFile(path string, data []byte, mode fs.FileMode) error {
	return writeFileWith(path, data, mode, writeAndSync, os.Rename)
}

// WriteFileDurable additionally commits the replacement's directory metadata.
// Reserve it for checkpoints that must be on disk before an external side
// effect. The file must belong to root; the caller is responsible for checking
// path ownership and serializing writers. On error, the replacement can already
// be visible but must not be treated as durably committed.
func WriteFileDurable(root, path string, data []byte, mode fs.FileMode) error {
	root, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("fileutil: checkpoint must be inside its owning root")
	}
	return writeFileWith(path, data, mode, writeAndSync, func(from, to string) error {
		return replaceDurable(from, to, root)
	})
}

func writeAndSync(f *os.File, data []byte) error {
	n, err := f.Write(data)
	if err != nil {
		return err
	}
	if n != len(data) {
		return io.ErrShortWrite
	}
	return f.Sync()
}

func writeFileWith(path string, data []byte, mode fs.FileMode,
	write func(*os.File, []byte) error, replace func(string, string) error) error {
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("fileutil: %s is not a regular file", path)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("fileutil: inspecting %s: %w", path, err)
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("fileutil: creating parent of %s: %w", path, err)
	}
	f, err := os.CreateTemp(dir, ".mulix-write-*")
	if err != nil {
		return fmt.Errorf("fileutil: preparing %s: %w", path, err)
	}
	name := f.Name()
	defer os.Remove(name)
	closed := false
	defer func() {
		if !closed {
			_ = f.Close()
		}
	}()
	if err := f.Chmod(mode.Perm()); err != nil {
		return fmt.Errorf("fileutil: setting permissions for %s: %w", path, err)
	}
	if err := write(f, data); err != nil {
		return fmt.Errorf("fileutil: writing %s: %w", path, err)
	}
	err = f.Close()
	closed = true
	if err != nil {
		return fmt.Errorf("fileutil: closing temporary file for %s: %w", path, err)
	}
	if err := replace(name, path); err != nil {
		return fmt.Errorf("fileutil: replacing %s: %w", path, err)
	}
	return nil
}
