//go:build !windows

package clientidentity

// Non-Windows platforms lack Windows DPAPI, so client secrets are stored in
// plaintext. The desktop client targets Windows only; these stubs exist so
// server and shared-package unit tests compile and run on Linux.
func ProtectSecret(value string) (string, error)         { return value, nil }
func UnprotectSecret(value string) (string, error)       { return value, nil }
func UnprotectSecretStrict(value string) (string, error) { return value, nil }
