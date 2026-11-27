//go:build !unix && !windows

package fileutil

import "errors"

func replaceDurable(_, _, _ string) error {
	return errors.New("durable checkpoint replacement is unsupported on this platform")
}
