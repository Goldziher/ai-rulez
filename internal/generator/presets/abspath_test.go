package presets

import "path/filepath"

// absSlash is the absolute, platform spelling of a slash-separated path. On
// Windows a path such as /home/u is rooted but has no drive, so it is not
// absolute: a user-scope home built from it is rejected and a project directory
// built from it lies outside every workspace. The paths these tests use name
// nothing on disk, so any absolute spelling will do.
func absSlash(p string) string {
	abs, err := filepath.Abs(filepath.FromSlash(p))
	if err != nil {
		panic(err)
	}
	return abs
}
