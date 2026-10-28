package scaffold

import (
	"errors"
	"io/fs"
	"os"
	"strings"

	"github.com/z-fenix/mulix-coding/internal/fileutil"
)

// Context files are local data, not vendor resources or update/force targets.
func ensureContextIgnore(root string) (bool, error) {
	p, err := fileutil.ControlPath(root, ".mulix/.gitignore", true)
	if err != nil {
		return false, err
	}
	data, err := os.ReadFile(p)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return false, err
	}
	mode := fs.FileMode(0644)
	if info, statErr := os.Stat(p); statErr == nil {
		mode = info.Mode().Perm()
	}
	original := string(data)
	newline := "\n"
	if strings.Contains(original, "\r\n") {
		newline = "\r\n"
	}
	lines := strings.Split(strings.ReplaceAll(original, "\r\n", "\n"), "\n")
	result := original
	for _, rule := range []string{"/context.json", "/sessions/", "/.locks/"} {
		found := false
		for _, line := range lines {
			if line == rule {
				found = true
				break
			}
		}
		if !found {
			if result != "" && !strings.HasSuffix(result, "\n") {
				result += newline
			}
			result += rule + newline
		}
	}
	if result == original {
		return false, nil
	}
	return true, fileutil.WriteFile(p, []byte(result), mode)
}
