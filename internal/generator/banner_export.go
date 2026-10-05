package generator

// HasGeneratedBanner reports whether data starts with the generated-file banner
// ai-rulez writes (after optional frontmatter), judged by the file's extension.
// Importers use it to avoid reading generated output back as source.
func HasGeneratedBanner(path string, data []byte) bool {
	return hasGeneratedBanner(path, data)
}

// GeneratedManifestNames are the files, relative to the config directory, that
// list every path a generate run wrote ({"files": [...]}, relative to the project).
func GeneratedManifestNames() []string {
	return []string{generatedManifestName, generatedLocalManifestName}
}
