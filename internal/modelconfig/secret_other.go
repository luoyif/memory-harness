//go:build !darwin && !windows

package modelconfig

// Unsupported desktop platforms retain a process-local secret store. macOS and
// Windows use their native credential vaults in platform-specific files.
func NewDefaultSecretStore(_ string) SecretStore { return NewMemorySecretStore() }
