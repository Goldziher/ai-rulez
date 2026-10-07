package config

import (
	"net/url"
	"strings"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/ard"
)

// ARDConfig is the [ard] table (docs/ard.md): who publishes the Agentic Resource
// Discovery manifest and under which namespace. Identifiers are
// urn:air:<publisher>:<namespace>:<name>.
type ARDConfig struct {
	// Publisher is the fully qualified domain name the identifiers are anchored
	// to, normally the domain that serves /.well-known/ard.json.
	Publisher string `yaml:"publisher,omitempty" json:"publisher,omitempty" toml:"publisher,omitempty"`
	// Namespace is the identifier segment between publisher and name; colons
	// separate sub-segments.
	Namespace string `yaml:"namespace,omitempty" json:"namespace,omitempty" toml:"namespace,omitempty"`
	// BaseURL is the https URL the published skill files and server cards are
	// served from. Without it entries point at the release of the repository.
	BaseURL string `yaml:"base_url,omitempty" json:"base_url,omitempty" toml:"base_url,omitempty"` //nolint:tagliatelle
	// PluginType overrides the media type of plugin entries, for when the spec
	// registers one.
	PluginType string `yaml:"plugin_type,omitempty" json:"plugin_type,omitempty" toml:"plugin_type,omitempty"` //nolint:tagliatelle
}

// validateARD checks the [ard] table when present.
func (c *Config) validateARD() error {
	a := c.ARD
	if a == nil {
		return nil
	}
	hint := "Set [ard] publisher to the domain that serves /.well-known/ard.json and namespace to a short name"
	if strings.TrimSpace(a.Publisher) == "" {
		return oops.With("field", "ard.publisher").Hint(hint).Errorf("ard.publisher is required")
	}
	if err := ard.ValidatePublisher(a.Publisher); err != nil {
		return oops.With("field", "ard.publisher").Hint(hint).Wrapf(err, "ard.publisher")
	}
	if strings.TrimSpace(a.Namespace) == "" {
		return oops.With("field", "ard.namespace").Hint(hint).Errorf("ard.namespace is required")
	}
	if _, err := ard.NewIdentifier(a.Publisher, a.Namespace, "x"); err != nil {
		return oops.With("field", "ard.namespace").Hint(hint).Wrapf(err, "ard.namespace")
	}
	if a.BaseURL != "" {
		u, err := url.Parse(a.BaseURL)
		if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
			return oops.With("field", "ard.base_url").Hint("Use an absolute https URL without credentials").
				Errorf("ard.base_url %q must be an https URL", a.BaseURL)
		}
	}
	if a.PluginType != "" && (!strings.Contains(a.PluginType, "/") || strings.ContainsAny(a.PluginType, " \t")) {
		return oops.With("field", "ard.plugin_type").Errorf("ard.plugin_type %q must be a media type (type/subtype)", a.PluginType)
	}
	return nil
}
