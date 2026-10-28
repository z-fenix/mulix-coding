// Package lockfile provides bounded cross-process locks for mulix metadata.
package lockfile

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/z-fenix/mulix-coding/internal/fileutil"
)

const ContextKey = "context"

var ErrTimeout = errors.New("project lock wait timed out")
var ErrUnsupported = errors.New("project locks unsupported on this platform")

func ChangeKey(change string) string { return fmt.Sprintf("change-%x", sha256.Sum256([]byte(change))) }

// With never removes the lock file: unlinking it could create a second lock
// inode while another process still owns the first. Callbacks are not reentrant.
func With(ctx context.Context, root, key string, fn func() error) (err error) {
	if key == "" || strings.Trim(key, "abcdefghijklmnopqrstuvwxyz0123456789-") != "" {
		return fmt.Errorf("invalid lock key %q", key)
	}
	p, err := fileutil.ControlPath(root, ".mulix/.locks/"+key+".lock", true)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, f.Close()) }()
	wait, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	for {
		if err := wait.Err(); err != nil {
			return errors.Join(ErrTimeout, err)
		}
		acquired, err := tryLock(f)
		if err != nil {
			return err
		}
		if acquired {
			defer func() { err = errors.Join(err, unlock(f)) }()
			return fn()
		}
		timer := time.NewTimer(20 * time.Millisecond)
		select {
		case <-wait.Done():
			timer.Stop()
			return errors.Join(ErrTimeout, wait.Err())
		case <-timer.C:
		}
	}
}
