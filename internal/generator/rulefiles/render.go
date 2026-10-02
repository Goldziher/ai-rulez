package rulefiles

import (
	"fmt"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/markdown"
	"github.com/Goldziher/ai-rulez/internal/templates"
	"github.com/samber/oops"
	"gopkg.in/yaml.v3"
)

// Render produces the full text of one rule file: YAML frontmatter (when the
// dialect emits fields), the generated banner (when t.Banner), the heading,
// the priority line (unless compact) and the body. Notes report fallbacks and
// soft-limit overruns; the content is never truncated.
//
// The "text" is measured before hash injection, so MaxChars counts runes of
// the content as rendered here (a few lines of hash comments are added later).
//
// Hash injection assumptions (see generator.injectHashes/stripHeader): with
// frontmatter, hash lines become YAML comments before the closing "---" and
// stripHeader removes frontmatter then banner; without frontmatter, the hash
// lines go inside the multi-line HTML banner. A file with neither (Banner
// false and no fields) cannot carry hashes, so callers should keep Banner on
// except for formats whose dialect always emits frontmatter (.mdc).
func Render(t Target, it Item, cfg *config.Config) (text string, notes []string, err error) {
	fm, notes := Frontmatter(t.Dialect, it)
	if fm == nil && !t.Banner {
		return "", nil, oops.With("rule", it.File.Name, "preset", t.Preset).
			Errorf("rule file would have neither frontmatter nor banner, so it could not carry hashes")
	}

	var b strings.Builder
	if fm != nil {
		marshaled, err := yaml.Marshal(fm) // yaml.v3 sorts map keys
		if err != nil {
			return "", nil, oops.With("rule", it.File.Name).Wrapf(err, "marshal frontmatter")
		}
		b.WriteString("---\n")
		b.Write(marshaled)
		b.WriteString("---\n")
	}
	if t.Banner {
		b.WriteString(templates.RuleBanner(sourceLabel(it.File.Path, cfg)))
	}

	b.WriteString("# " + it.File.Name + "\n\n")
	if !cfg.IsCompact() && it.File.Metadata != nil && it.File.Metadata.Priority != "" {
		b.WriteString("**Priority:** " + it.File.Metadata.Priority + "\n\n")
	}
	if t.Dialect == DialectJunie && len(it.Activation.Globs) > 0 && it.Activation.Mode == config.ActivationGlob {
		b.WriteString("_Applies to: " + strings.Join(it.Activation.Globs, ", ") + "_\n\n")
	}
	b.WriteString(strings.TrimRight(markdown.ProcessEmbeddedContent(it.File.Content), "\n"))
	b.WriteString("\n")

	out := b.String()
	if t.MaxChars > 0 && utf8.RuneCountInString(out) > t.MaxChars {
		notes = append(notes, fmt.Sprintf("%s %q: %d chars exceeds the %s limit of %d",
			kindLabel(it.Kind), it.File.Name, utf8.RuneCountInString(out), t.Preset, t.MaxChars))
	}
	return out, notes, nil
}

func kindLabel(k Kind) string {
	if k == KindContext {
		return "context"
	}
	return "rule"
}

// sourceLabel keeps the banner reproducible across checkouts: the path
// relative to the config directory, prefixed with its name (".ai-rulez/…" or
// ".config/ai-rulez/…"), or the base name when the file lies elsewhere.
func sourceLabel(p string, cfg *config.Config) string {
	if cfg != nil && cfg.ConfigDir != "" && p != "" {
		if rel, err := filepath.Rel(cfg.ConfigDir, p); err == nil && rel != ".." &&
			!strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			name := cfg.ConfigDirName
			if name == "" {
				name = filepath.Base(cfg.ConfigDir)
			}
			return name + "/" + filepath.ToSlash(rel)
		}
	}
	p = strings.ReplaceAll(p, "\\", "/")
	return p[strings.LastIndex(p, "/")+1:]
}
