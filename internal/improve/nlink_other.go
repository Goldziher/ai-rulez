//go:build !unix

package improve

import "io/fs"

func hardLinked(fs.FileInfo) bool { return false }
