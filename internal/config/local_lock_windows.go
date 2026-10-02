//go:build windows

package config

// lockLocalConfig is a no-op on Windows: overlay edits are not serialized
// across processes there.
func lockLocalConfig(_ string) (func(), error) {
	return func() {}, nil
}
