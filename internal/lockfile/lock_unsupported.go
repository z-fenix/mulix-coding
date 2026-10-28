//go:build !windows && !linux && !darwin && !freebsd && !netbsd && !openbsd && !dragonfly

package lockfile

import "os"

func tryLock(*os.File) (bool, error) { return false, ErrUnsupported }
func unlock(*os.File) error          { return ErrUnsupported }
