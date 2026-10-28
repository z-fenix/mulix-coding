package fileutil

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ControlPath checks each existing component without following links. It is
// workflow hardening, not a filesystem sandbox against concurrent attackers.
func ControlPath(root, rel string, createParents bool) (string, error) {
	if rel == "" || filepath.IsAbs(rel) || strings.ContainsAny(rel, `\:`) {
		return "", fmt.Errorf("control path: invalid relative path %q", rel)
	}
	parts := strings.Split(filepath.ToSlash(rel), "/")
	p, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	for i, part := range parts {
		if part == "" || part == "." || part == ".." {
			return "", fmt.Errorf("control path: invalid segment in %q", rel)
		}
		p = filepath.Join(p, part)
		info, err := os.Lstat(p)
		if os.IsNotExist(err) {
			if i < len(parts)-1 && createParents {
				if err := os.Mkdir(p, 0700); err != nil && !os.IsExist(err) {
					return "", err
				}
				info, err = os.Lstat(p)
			} else {
				continue
			}
		}
		if err != nil {
			return "", err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("control path: refusing link %s", p)
		}
		if i < len(parts)-1 {
			if !info.IsDir() {
				return "", fmt.Errorf("control path: %s is not a directory", p)
			}
		} else if !info.Mode().IsRegular() {
			return "", fmt.Errorf("control path: %s is not a regular file", p)
		}
	}
	return p, nil
}
