//go:build !windows

package platform

import (
	"fmt"
	"os"
	"path/filepath"
)

func DefaultLedgerPath() (string, error) {
	root, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate user config directory: %w", err)
	}
	return filepath.Join(root, "TLAN", "mesh-resources.json"), nil
}
