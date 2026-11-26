//go:build windows

package platform

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

// DefaultLedgerPath stores the Windows resource ledger under ProgramData so
// medium-integrity processes of the same user cannot modify cleanup targets
// executed by the elevated client.
func DefaultLedgerPath() (string, error) {
	if path, err := windows.KnownFolderPath(windows.FOLDERID_ProgramData, windows.KF_FLAG_DEFAULT); err == nil {
		if pd := strings.TrimSpace(path); pd != "" {
			return filepath.Join(pd, "TLAN", "mesh-resources.json"), nil
		}
	}
	root, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate ledger directory: %w", err)
	}
	return filepath.Join(root, "TLAN", "mesh-resources.json"), nil
}
