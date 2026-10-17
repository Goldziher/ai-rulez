package lint

import "path/filepath"

// scanPluginFiles runs the security rules over the files an installed plugin
// delivers besides its content files: the hook documents and the scripts they
// run, and the MCP documents and the files their commands run. They are
// scanned at the plugin entry's trust level, like the plugin's skills.
func (r *runner) scanPluginFiles() {
	force, on := r.importLevel()
	if !on {
		return
	}
	defer func() { r.forceSev = "" }()
	for _, f := range r.cfg.ImportedScan {
		if !filepath.IsAbs(f.Path) {
			continue
		}
		r.docs[f.Path] = parseDoc(f.Content)
		r.forceSev = importForceFor(f.Trust, force)
		r.securityScan(f.Path, f.Content)
	}
}
