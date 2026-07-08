package plugin

import (
	"path/filepath"

	"github.com/Goldziher/ai-rulez/internal/config"
)

// renderStatusline copies the plugin's declared status-line script into the
// Claude plugin directory (.claude-plugin/statusline.sh). The script itself is
// hand-authored and passed through verbatim; the slash command that wires it
// into user settings is an ordinary bundled command, not synthesized here.
// Statusline is a Claude-only capability, so this is only invoked by the Claude
// renderer. Returns nothing when no statusline is declared.
func renderStatusline(m *Manifest, baseDir string) ([]config.OutputFile, error) {
	if m.Statusline == nil || m.Statusline.Script == "" {
		return nil, nil
	}
	dst := filepath.Join(baseDir, ".claude-plugin", "statusline.sh")
	out, err := passthroughFile(m.SourceDir, m.Statusline.Script, dst)
	if err != nil {
		return nil, err
	}
	return []config.OutputFile{out}, nil
}
