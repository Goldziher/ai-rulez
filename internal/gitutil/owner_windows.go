//go:build windows

package gitutil

import "io/fs"

// Windows has no uid; ACLs govern ownership and are not inspected here.
func ownedByCurrentUser(fs.FileInfo) bool { return true }

// writableByOthers is false on Windows: Go reports mode 0666 for every writable
// file there, so the bits say nothing about other users; ACLs govern writers and
// the per-user profile directories are private to the account.
func writableByOthers(fs.FileInfo) bool { return false }
