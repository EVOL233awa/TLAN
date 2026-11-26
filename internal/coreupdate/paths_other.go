//go:build !windows

package coreupdate

func canonicalizeShortPath(path string) (string, error) {
	return path, nil
}
