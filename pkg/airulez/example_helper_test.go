package airulez_test

import (
	"os"
	"path/filepath"
)

// tempProject writes a one-preset project into a new directory.
func tempProject() (string, error) {
	dir, err := os.MkdirTemp("", "airulez-example-")
	if err != nil {
		return "", err //nolint:wrapcheck // example helper
	}
	files := map[string]string{
		".ai-rulez/config.toml":    "version = \"5.0\"\nname = \"demo\"\npresets = [\"claude\"]\nagents_md = false\n",
		".ai-rulez/rules/style.md": "# Style\n\nBe concise.\n",
	}
	for name, content := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return "", err //nolint:wrapcheck // example helper
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil { //nolint:gosec // example fixture
			return "", err //nolint:wrapcheck // example helper
		}
	}
	return dir, nil
}
