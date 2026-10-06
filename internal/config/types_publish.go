package config

import (
	"fmt"
	"path"
	"regexp"
	"slices"
	"strings"

	"github.com/samber/oops"
)

// Publish emitter names of [[publish.emitters]] and npm access values (docs/publish.md).
const (
	PublishEmitterTemplate  = "template"
	PublishEmitterCursor    = "cursor-team-marketplace"
	PublishEmitterPort      = "port"
	PublishEmitterAWS       = "aws-agent-registry"
	PublishEmitterKiro      = "kiro-steering"
	PublishAccessRestricted = "restricted"
	PublishAccessPublic     = "public"
)

var (
	publishScopePattern   = regexp.MustCompile(`^@[a-z0-9][a-z0-9._-]*$`)
	publishChannelPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,31}$`)
	publishRefPattern     = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]*$`)
	publishOCIPattern     = regexp.MustCompile(`^([A-Za-z0-9][A-Za-z0-9.-]*(:[0-9]+)?)/[a-z0-9]+([._-][a-z0-9]+)*(/[a-z0-9]+([._-][a-z0-9]+)*)*$`)
	publishOutputPattern  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
	publishOptionKey      = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)
)

// publishOptionMax bounds an emitter option value.
const publishOptionMax = 256

// PublishConfig is the [publish] table (docs/publish.md). It holds no
// credential: registries and forges authenticate through their own CLIs and
// environment, never through the repository.
type PublishConfig struct {
	// Runtimes restricts the published bundle to these plugin runtimes
	// (default: the [plugin] runtimes). `publish --runtime` overrides it.
	Runtimes []string `yaml:"runtimes,omitempty" json:"runtimes,omitempty" toml:"runtimes,omitempty"`
	// RequireSignature makes publish fail without a signature (`--sign`).
	RequireSignature bool `yaml:"require_signature,omitempty" json:"require_signature,omitempty" toml:"require_signature,omitempty"` //nolint:tagliatelle
	// RequireApproved makes publish fail unless every content item the
	// [governance] policy selects is approved in ai-rulez.lock.
	RequireApproved bool `yaml:"require_approved,omitempty" json:"require_approved,omitempty" toml:"require_approved,omitempty"` //nolint:tagliatelle
	// AllowDirty waives the clean-tree gate (the manifest records dirty = true).
	AllowDirty bool `yaml:"allow_dirty,omitempty" json:"allow_dirty,omitempty" toml:"allow_dirty,omitempty"` //nolint:tagliatelle

	GitHubRelease *PublishGitHubRelease `yaml:"github_release,omitempty" json:"github_release,omitempty" toml:"github_release,omitempty"` //nolint:tagliatelle
	OCI           *PublishOCI           `yaml:"oci,omitempty" json:"oci,omitempty" toml:"oci,omitempty"`
	NPM           *PublishNPM           `yaml:"npm,omitempty" json:"npm,omitempty" toml:"npm,omitempty"`
	Marketplace   *PublishMarketplace   `yaml:"marketplace,omitempty" json:"marketplace,omitempty" toml:"marketplace,omitempty"`
	Emitters      []PublishEmitter      `yaml:"emitters,omitempty" json:"emitters,omitempty" toml:"emitters,omitempty"`
}

// PublishGitHubRelease is [publish.github_release].
type PublishGitHubRelease struct {
	// Repo is OWNER/REPO (or HOST/OWNER/REPO); default [plugin] repository, else origin.
	Repo string `yaml:"repo,omitempty" json:"repo,omitempty" toml:"repo,omitempty"`
}

// PublishOCI is [publish.oci].
type PublishOCI struct {
	// Ref is the repository the artifact is pushed to (host/path, no tag or
	// digest); the tag is the plugin version.
	Ref string `yaml:"ref,omitempty" json:"ref,omitempty" toml:"ref,omitempty"`
}

// PublishNPM is [publish.npm].
type PublishNPM struct {
	// Scope is the npm scope the package is published under, "@acme".
	Scope string `yaml:"scope,omitempty" json:"scope,omitempty" toml:"scope,omitempty"`
	// Access is "restricted" (default) or "public".
	Access string `yaml:"access,omitempty" json:"access,omitempty" toml:"access,omitempty"`
	// Registry is the registry URL (https only); empty uses the npm client's.
	Registry string `yaml:"registry,omitempty" json:"registry,omitempty" toml:"registry,omitempty"`
}

// PublishMarketplace is [publish.marketplace].
type PublishMarketplace struct {
	// Channels maps a channel name to the git ref its pinned index points at.
	// A channel not listed here pins the release tag.
	Channels map[string]string `yaml:"channels,omitempty" json:"channels,omitempty" toml:"channels,omitempty"`
}

// PublishEmitter is one [[publish.emitters]] entry.
type PublishEmitter struct {
	Name string `yaml:"name" json:"name" toml:"name"`
	// Template is the project-relative text/template file of the "template" emitter.
	Template string `yaml:"template,omitempty" json:"template,omitempty" toml:"template,omitempty"`
	// Output renames the template's output file under <dist>/emit/.
	Output string `yaml:"output,omitempty" json:"output,omitempty" toml:"output,omitempty"`
	// Options are the emitter's own settings, such as blueprint for "port".
	Options map[string]string `yaml:"options,omitempty" json:"options,omitempty" toml:"options,omitempty"`
}

// PublishEmitterNames lists every emitter a [[publish.emitters]] entry may name.
var PublishEmitterNames = []string{PublishEmitterTemplate, PublishEmitterCursor, PublishEmitterPort, PublishEmitterAWS, PublishEmitterKiro}

// ValidChannel reports whether name is a usable marketplace channel name.
func ValidChannel(name string) bool { return publishChannelPattern.MatchString(name) }

// ValidOCIRepository reports whether ref is a repository reference without a tag or digest.
func ValidOCIRepository(ref string) bool {
	return publishOCIPattern.MatchString(ref) && !strings.Contains(ref, "..")
}

func (c *Config) validatePublish() error {
	p := c.Publish
	if p == nil {
		return nil
	}
	fail := func(field, format string, args ...any) error {
		return oops.With("field", "publish."+field).Errorf(format, args...)
	}
	for _, r := range p.Runtimes {
		if !slices.Contains(KnownPluginRuntimes, r) {
			return fail("runtimes", "unknown runtime %q (known: %s)", r, strings.Join(KnownPluginRuntimes, ", "))
		}
	}
	if p.OCI != nil && p.OCI.Ref != "" && !ValidOCIRepository(p.OCI.Ref) {
		return fail("oci.ref", "%q is not a repository reference (host/path, lower-case, no tag or digest)", p.OCI.Ref)
	}
	if n := p.NPM; n != nil {
		if n.Scope != "" && !publishScopePattern.MatchString(n.Scope) {
			return fail("npm.scope", "%q is not an npm scope such as @acme", n.Scope)
		}
		if !slices.Contains([]string{"", PublishAccessRestricted, PublishAccessPublic}, n.Access) {
			return fail("npm.access", "invalid access %q (use restricted or public)", n.Access)
		}
		if n.Registry != "" && !strings.HasPrefix(n.Registry, "https://") {
			return fail("npm.registry", "the registry must be an https:// URL")
		}
	}
	if m := p.Marketplace; m != nil {
		for name, ref := range m.Channels {
			if !ValidChannel(name) {
				return fail("marketplace.channels", "invalid channel name %q", name)
			}
			if !publishRefPattern.MatchString(ref) || strings.Contains(ref, "..") || strings.HasSuffix(ref, "/") {
				return fail("marketplace.channels", "channel %q has an invalid git ref %q", name, ref)
			}
		}
	}
	for i, e := range p.Emitters {
		if err := validatePublishEmitter(e); err != nil {
			return oops.With("field", fmt.Sprintf("publish.emitters[%d]", i)).Wrap(err)
		}
	}
	return nil
}

func validatePublishEmitter(e PublishEmitter) error {
	if !slices.Contains(PublishEmitterNames, e.Name) {
		return oops.Errorf("unknown emitter %q (known: %s)", e.Name, strings.Join(PublishEmitterNames, ", "))
	}
	for k, v := range e.Options {
		if !publishOptionKey.MatchString(k) || len(v) > publishOptionMax {
			return oops.Errorf("invalid option %q: keys are lower-case words, values at most %d characters", k, publishOptionMax)
		}
	}
	if e.Name != PublishEmitterTemplate {
		if e.Template != "" || e.Output != "" {
			return oops.Errorf("template and output apply to the %q emitter only", PublishEmitterTemplate)
		}
		return nil
	}
	if e.Template == "" {
		return oops.Errorf("the template emitter needs a template file")
	}
	clean := path.Clean(e.Template)
	if path.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, "../") || strings.Contains(e.Template, "\\") {
		return oops.Errorf("template %q must be a path inside the project", e.Template)
	}
	if e.Output != "" && !publishOutputPattern.MatchString(e.Output) {
		return oops.Errorf("output %q must be a plain file name", e.Output)
	}
	return nil
}
