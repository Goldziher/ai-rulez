package plugin

import (
	"encoding/json"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// interfaceDoc is the rich UI block shared by Codex and Kimi manifests, with the
// camelCase JSON keys those runtimes expect.
type interfaceDoc struct {
	DisplayName       string   `json:"displayName,omitempty"`
	ShortDescription  string   `json:"shortDescription,omitempty"`
	LongDescription   string   `json:"longDescription,omitempty"`
	DeveloperName     string   `json:"developerName,omitempty"`
	Category          string   `json:"category,omitempty"`
	Capabilities      []string `json:"capabilities,omitempty"`
	DefaultPrompt     []string `json:"defaultPrompt,omitempty"`
	WebsiteURL        string   `json:"websiteURL,omitempty"`
	PrivacyPolicyURL  string   `json:"privacyPolicyURL,omitempty"`
	TermsOfServiceURL string   `json:"termsOfServiceURL,omitempty"`
	BrandColor        string   `json:"brandColor,omitempty"`
	ComposerIcon      string   `json:"composerIcon,omitempty"`
	Logo              string   `json:"logo,omitempty"`
	LogoDark          string   `json:"logoDark,omitempty"`
	Screenshots       []string `json:"screenshots,omitempty"`
}

// buildInterface converts the config interface block to its manifest shape, or
// nil when none is declared.
func buildInterface(in *config.PluginInterface) *interfaceDoc {
	if in == nil {
		return nil
	}
	return &interfaceDoc{
		DisplayName:       in.DisplayName,
		ShortDescription:  in.ShortDescription,
		LongDescription:   in.LongDescription,
		DeveloperName:     in.DeveloperName,
		Category:          in.Category,
		Capabilities:      in.Capabilities,
		DefaultPrompt:     in.DefaultPrompt,
		WebsiteURL:        in.WebsiteURL,
		PrivacyPolicyURL:  in.PrivacyPolicyURL,
		TermsOfServiceURL: in.TermsOfServiceURL,
		BrandColor:        in.BrandColor,
		ComposerIcon:      in.ComposerIcon,
		Logo:              in.Logo,
		LogoDark:          in.LogoDark,
		Screenshots:       in.Screenshots,
	}
}

// InterfaceFromDoc reads a manifest interface block (the camelCase keys Codex
// and Kimi manifests use, extensions.com.openai.interface in an Agent Plugins
// manifest) into the configuration shape.
func InterfaceFromDoc(doc map[string]any) (*config.PluginInterface, error) {
	raw, err := json.Marshal(doc)
	if err != nil {
		return nil, oops.Wrapf(err, "encode the interface block")
	}
	var in interfaceDoc
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, oops.Wrapf(err, "decode the interface block")
	}
	return &config.PluginInterface{
		DisplayName: in.DisplayName, ShortDescription: in.ShortDescription, LongDescription: in.LongDescription,
		DeveloperName: in.DeveloperName, Category: in.Category, Capabilities: in.Capabilities,
		DefaultPrompt: in.DefaultPrompt, WebsiteURL: in.WebsiteURL, PrivacyPolicyURL: in.PrivacyPolicyURL,
		TermsOfServiceURL: in.TermsOfServiceURL, BrandColor: in.BrandColor, ComposerIcon: in.ComposerIcon,
		Logo: in.Logo, LogoDark: in.LogoDark, Screenshots: in.Screenshots,
	}, nil
}
