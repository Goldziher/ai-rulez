package generator

import (
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/contentlock"
)

// LockOutputs renders every output in memory and returns the ones ai-rulez.lock
// pins. Nothing is written or deleted.
//
// The bytes are the rendering as the presets produced it: the Generated stamp is
// removed from the header, and the Content-Hash and Source-Hash lines are not
// there yet (they are injected at write time), so a digest is the same under
// every [header] hashes mode and on every run. Outputs that are partly the
// consumer's (a merged settings.json), that carry secrets, or that
// are machine-local are left out; the merged settings document is pinned through
// its hook, permission and role sources instead.
func (g *Generator) LockOutputs(profile string) ([]contentlock.Output, error) {
	return g.lockOutputs(profile, false)
}

// lockOutputs is LockOutputs; withPartial also returns the part of a merged
// document (settings.json) that ai-rulez renders, which a role pin needs because
// a role's skill_mode lands there.
func (g *Generator) lockOutputs(profile string, withPartial bool) ([]contentlock.Output, error) {
	// Render without the Generated stamp, which is the only timestamp a header
	// can carry; stripGeneratedStamp below covers a renderer that adds one anyway.
	cfg := *g.config
	header := config.HeaderConfig{}
	if cfg.Header != nil {
		header = *cfg.Header
	}
	noStamp := false
	header.Timestamp = &noStamp
	cfg.Header = &header
	original := g.config
	g.config = &cfg
	defer func() { g.config = original }()

	g.mu.Lock()
	defer g.mu.Unlock()
	g.beginRun()
	defer g.resetRunState()
	// The pins must not depend on the environment or the checkout: leave ${VAR}
	// and ${PROJECT_ROOT} as written instead of resolving them.
	g.lockRender = true
	defer func() { g.lockRender = false }()

	outputs, _, err := g.collectOutputs(profile)
	if err != nil {
		return nil, err
	}
	// Whatever still carries a secret (a literal one in args or a URL) is left
	// out of the pins, wherever the outputs were collected.
	g.markSensitiveOutputs(outputs)
	var pins []contentlock.Output
	for _, output := range outputs {
		if output.IsDir || (output.PartiallyOwned && !withPartial) || output.Sensitive || output.LocalOnly {
			continue
		}
		data := output.RawContent
		if data == nil {
			data = []byte(stripGeneratedStamp(output.Content, output.Path))
		}
		mode := output.Mode.Perm()
		if mode == 0 {
			mode = 0o644
		}
		pins = append(pins, contentlock.Output{
			Path: g.relSlash(g.absOutputPath(output.Path)),
			Mode: uint32(mode),
			Data: data,
		})
	}
	return pins, nil
}

// stripGeneratedStamp removes the Generated: stamp from the header of content,
// leaving the body untouched (a body line that starts with "Generated: " stays).
func stripGeneratedStamp(content, outputPath string) string {
	body := stripHeader(content, outputPath)
	if body == "" || !strings.HasSuffix(content, body) {
		return content
	}
	header := strings.TrimSuffix(content, body)
	return generatedStampPattern.ReplaceAllString(header, "") + body
}
