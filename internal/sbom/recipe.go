package sbom

import (
	"encoding/json"
	"strings"
	"time"
)

// RecipeOf reads how a committed CycloneDX document made by `ai-rulez sbom` was
// built, as far as the document says: the slice (files, profile, role), whether
// the generated outputs are listed, and the time it records. ok is false for a
// document that is not an ai-rulez CycloneDX SBOM, or that cannot be rebuilt
// from what it records: reviewer identities replaced by a hash (the key is not
// at hand) or a recorded attestation check (--verify).
//
// Whether approvals were left out (--no-approvals) is not recorded: a caller that
// compares the rebuilt document with the committed one tries both.
func RecipeOf(data []byte) (opts Options, ok bool) {
	var doc struct {
		BOMFormat string `json:"bomFormat"`
		Metadata  struct {
			Timestamp string `json:"timestamp"`
			Component struct {
				Properties []Property `json:"properties"`
			} `json:"component"`
		} `json:"metadata"`
		Components []struct {
			BOMRef     string     `json:"bom-ref"`
			Properties []Property `json:"properties"`
		} `json:"components"`
	}
	if json.Unmarshal(data, &doc) != nil || doc.BOMFormat != "CycloneDX" {
		return Options{}, false
	}
	project := map[string]string{}
	for _, p := range doc.Metadata.Component.Properties {
		project[p.Name] = p.Value
	}
	if project[PropertyPrefix+"tree"] == "" || project[PropertyPrefix+"signature"] != "" {
		return Options{}, false
	}
	opts = Options{
		Files:   project[PropertyPrefix+"files"],
		Profile: project[PropertyPrefix+"profile"],
		Role:    project[PropertyPrefix+"role"],
	}
	for _, c := range doc.Components {
		if strings.HasPrefix(c.BOMRef, "ai-rulez:output:") {
			opts.IncludeOutputs = true
		}
		for _, p := range c.Properties {
			if p.Name == PropertyPrefix+"approvers" && strings.HasPrefix(p.Value, "reviewer-") {
				return Options{}, false
			}
		}
	}
	if t, err := time.Parse(time.RFC3339, doc.Metadata.Timestamp); err == nil {
		opts.Timestamp = t.UTC()
	}
	return opts, true
}
