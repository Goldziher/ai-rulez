package plugin

import "github.com/Goldziher/ai-rulez/internal/config"

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
