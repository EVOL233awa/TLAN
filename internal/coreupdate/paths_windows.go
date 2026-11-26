//go:build windows

package coreupdate

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

func canonicalizeShortPath(path string) (string, error) {
	in, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return "", err
	}
	buf := make([]uint16, windows.MAX_PATH)
	for {
		n, err := windows.GetLongPathName(in, &buf[0], uint32(len(buf)))
		if err != nil {
			if errors.Is(err, os.ErrNotExist) || errors.Is(err, windows.ERROR_FILE_NOT_FOUND) || errors.Is(err, windows.ERROR_PATH_NOT_FOUND) {
				return path, nil
			}
			return "", err
		}
		if n == 0 {
			return path, nil
		}
		if int(n) < len(buf) {
			return windows.UTF16ToString(buf[:n]), nil
		}
		buf = make([]uint16, n)
	}
}
