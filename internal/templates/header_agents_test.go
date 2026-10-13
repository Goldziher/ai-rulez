package templates_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/templates"
)

func TestHeaderNamesTheAgentsSource(t *testing.T) {
	tests := []struct {
		name       string
		cfg        config.Config
		wantSource string
	}{
		{
			name: "config in .agents",
			cfg: config.Config{
				AgentsLayout: true, BaseDir: "/p", ConfigDir: "/p/.agents/.ai-rulez", ConfigDirName: ".agents/.ai-rulez",
				ConfigFile: "ai-rulez.toml", ConfigFilePath: "/p/.agents/ai-rulez.toml",
			},
			wantSource: "Source: .agents/ai-rulez.toml",
		},
		{
			name: "source key in .ai-rulez",
			cfg: config.Config{
				Source: config.SourceAgents, BaseDir: "/p", ConfigDir: "/p/.ai-rulez", ConfigDirName: ".ai-rulez",
				ConfigFile: "config.toml",
			},
			wantSource: "Source: .ai-rulez/config.toml",
		},
	}
	for _, tt := range tests {
		for _, style := range []string{"minimal", "compact", "detailed"} {
			t.Run(tt.name+"/"+style, func(t *testing.T) {
				// Arrange
				cfg := tt.cfg
				cfg.Name = "demo"
				cfg.Header = &config.HeaderConfig{Style: style}
				data := createTemplateData("demo", &cfg)
				data.ConfigFile = cfg.ConfigFile

				// Act
				header := templates.GenerateHeader(data)

				// Assert
				assert.Contains(t, header, tt.wantSource)
				assert.Contains(t, header, ".agents/")
				assert.NotContains(t, header, ".agents/.ai-rulez")
				assert.NotContains(t, header, "modify .ai-rulez/")
			})
		}
	}
}
