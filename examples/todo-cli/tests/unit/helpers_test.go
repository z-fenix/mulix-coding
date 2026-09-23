package tests

import (
	"bufio"
	"io"
)

func readAll(r io.Reader) (string, error) {
	data, err := io.ReadAll(bufio.NewReader(r))
	if err != nil {
		return "", err
	}
	return string(data), nil
}
