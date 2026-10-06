//go:build windows

package gitutil

import "io/fs"

// Windows has no uid; ACLs govern ownership and are not inspected here.
func ownedByCurrentUser(fs.FileInfo) bool { return true }
