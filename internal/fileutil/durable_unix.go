//go:build unix

package fileutil

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func replaceDurable(from, to, root string) error {
	if err := os.Rename(from, to); err != nil {
		return err
	}
	// Also commit ancestor entries up to the existing project root: a new
	// change's runtime directory may have been created since the last sync.
	for dir := filepath.Dir(to); ; dir = filepath.Dir(dir) {
		f, err := os.Open(dir)
		if err == nil {
			err = errors.Join(f.Sync(), f.Close())
		}
		if err != nil {
			return fmt.Errorf("syncing checkpoint directory %s: %w", dir, err)
		}
		if dir == root {
			return nil
		}
	}
}
