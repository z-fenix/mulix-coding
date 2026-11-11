package sessionctx

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/z-fenix/mulix-coding/internal/layout"
)

// scratchDir only derives a path; resolving context never creates scratch files.
// The root supplied by Store is absolute and has already resolved symlink aliases.
func scratchDir(root, change, sessionID string) (string, error) {
	temp := os.TempDir()
	if !filepath.IsAbs(temp) {
		return "", fmt.Errorf("system temporary directory must be absolute, got %q; set TMPDIR (or TEMP/TMP) to an absolute path", temp)
	}
	root = filepath.Clean(root)
	if runtime.GOOS == "windows" {
		root = strings.ToLower(root)
	}
	digest := sha256.Sum256([]byte(root + "\x00" + change + "\x00" + sessionID))
	return filepath.Join(temp, fmt.Sprintf("%s%x", layout.ScratchPrefix, digest)), nil
}
