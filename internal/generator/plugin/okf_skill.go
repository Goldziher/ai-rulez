package plugin

import (
	"github.com/Goldziher/ai-rulez/v5/internal/agentplugins"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/frontmatter"
	"github.com/samber/oops"
	"gopkg.in/yaml.v3"
)

func projectNativeSkill(content *config.ContentFile, out config.OutputFile) (config.OutputFile, error) {
	out = projectNativeContent(content, out)
	if content.Metadata == nil || content.Metadata.OKFType == "" {
		return out, nil
	}
	block := frontmatter.SplitString(string(outputBytes(out)))
	var fields map[string]any
	if err := yaml.Unmarshal([]byte(block.Raw), &fields); err != nil {
		return out, oops.With("path", content.Path).Wrapf(err, "parse the projected skill frontmatter")
	}
	if _, explicit := fields["name"]; !explicit {
		if fields == nil {
			fields = map[string]any{}
		}
		fields["name"] = content.Name
		raw, err := yaml.Marshal(fields)
		if err != nil {
			return out, oops.With("path", content.Path).Wrapf(err, "encode the projected skill identity")
		}
		out.RawContent = []byte("---\n" + string(raw) + "---\n" + block.Body)
		out.Content = ""
	}
	if err := agentplugins.ValidateSkill(content.Name, outputBytes(out)); err != nil {
		return out, oops.With("path", content.Path).Wrapf(err, "validate the projected OKF skill")
	}
	return out, nil
}
