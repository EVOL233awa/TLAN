package core

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	// CurrentVersion is the semantic release version shared by server and client.
	// Versioning rules:
	//   - Major: breaking / major overhaul
	//   - Minor: any server-side change (client & server Major.Minor must match)
	//   - Patch: client-only updates under the same Major.Minor
	CurrentVersion    = "1.3.0"
	CurrentVersionTag = "v" + CurrentVersion
)

// ParseMajorMinor extracts the Major and Minor components from a semantic
// version string such as "1.1.0", "v1.1.2", or "1.1.0-rc.1".
func ParseMajorMinor(raw string) (int, int, error) {
	trimmed := strings.TrimSpace(raw)
	trimmed = strings.TrimPrefix(trimmed, "v")
	trimmed = strings.TrimPrefix(trimmed, "V")
	if trimmed == "" {
		return 0, 0, fmt.Errorf("empty version string")
	}
	if idx := strings.IndexAny(trimmed, "-+"); idx >= 0 {
		trimmed = trimmed[:idx]
	}
	parts := strings.Split(trimmed, ".")
	if len(parts) < 2 || len(parts) > 3 {
		return 0, 0, fmt.Errorf("invalid version %q", raw)
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil || major < 0 {
		return 0, 0, fmt.Errorf("invalid major version in %q", raw)
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil || minor < 0 {
		return 0, 0, fmt.Errorf("invalid minor version in %q", raw)
	}
	if len(parts) == 3 {
		patch, err := strconv.Atoi(parts[2])
		if err != nil || patch < 0 {
			return 0, 0, fmt.Errorf("invalid patch version in %q", raw)
		}
	}
	return major, minor, nil
}

// MajorMinorCompatible verifies that serverVersion and clientVersion share the
// exact same Major and Minor numbers.
func MajorMinorCompatible(serverVersion, clientVersion string) error {
	sMajor, sMinor, err := ParseMajorMinor(serverVersion)
	if err != nil {
		return fmt.Errorf("version_mismatch: invalid server version %q: %w", serverVersion, err)
	}
	cMajor, cMinor, err := ParseMajorMinor(clientVersion)
	if err != nil {
		return fmt.Errorf("version_mismatch: invalid client version %q: %w", clientVersion, err)
	}
	if sMajor != cMajor || sMinor != cMinor {
		return fmt.Errorf("version_mismatch: client %s is incompatible with server %s (major.minor must match)", strings.TrimSpace(clientVersion), strings.TrimSpace(serverVersion))
	}
	return nil
}
