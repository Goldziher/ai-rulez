package providers

import "path/filepath"

// absSlash is the absolute, platform spelling of a slash-separated path. On
// Windows a path such as /proj is rooted but has no drive, so it is not absolute
// and lies outside every workspace. The paths these tests use name nothing on
// disk, so any absolute spelling will do.
func absSlash(p string) string {
	abs, err := filepath.Abs(filepath.FromSlash(p))
	if err != nil {
		panic(err)
	}
	return abs
}
