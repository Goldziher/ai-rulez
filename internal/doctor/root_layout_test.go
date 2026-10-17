package doctor

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

func TestCheckSource_RootConfigFile(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
		want  []Finding
	}{
		{"a root layout alone has nothing to report", map[string]string{"ai-rulez.toml": baseConfig}, nil},
		{
			"a .ai-rulez directory shadows the file",
			map[string]string{"ai-rulez.toml": baseConfig, ".ai-rulez/config.toml": baseConfig},
			[]Finding{{
				Check: CheckSource, Severity: SeverityWarning, Path: "ai-rulez.toml",
				Message: "ai-rulez.toml is ignored: .ai-rulez/ takes precedence",
				Hint:    "keep one configuration; move settings you still need into .ai-rulez/config.toml, or remove the directory to use ai-rulez.toml",
			}},
		},
		{
			"the .config convention shadows the file",
			map[string]string{"ai-rulez.toml": baseConfig, ".config/ai-rulez/config.toml": baseConfig},
			[]Finding{{
				Check: CheckSource, Severity: SeverityWarning, Path: "ai-rulez.toml",
				Message: "ai-rulez.toml is ignored: .config/ai-rulez/ takes precedence",
				Hint:    "keep one configuration; move settings you still need into .config/ai-rulez/config.toml, or remove the directory to use ai-rulez.toml",
			}},
		},
		{"a config directory alone has nothing to report", map[string]string{".ai-rulez/config.toml": baseConfig}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir := project(t, tt.files)

			// Act
			got := runSource(t, dir)

			// Assert
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestShadowedRootConfig_IgnoresAContentlessConfig(t *testing.T) {
	// Arrange
	dir := project(t, map[string]string{"ai-rulez.toml": baseConfig})
	cfg, err := config.LoadConfig(context.Background(), dir, config.WithoutRemote())
	require.NoError(t, err)

	// Act
	got := shadowedRootConfig(cfg)

	// Assert
	assert.True(t, cfg.Contentless())
	assert.Empty(t, got)
}

func TestRun_RootLayoutReportsNoErrors(t *testing.T) {
	// Arrange
	dir := project(t, map[string]string{"ai-rulez.toml": baseConfig})

	// Act
	report := Run(context.Background(), Options{
		Load: func(ctx context.Context, opts ...config.LoadOption) (*config.Config, error) {
			return config.LoadConfig(ctx, dir, append(opts, config.WithoutRemote())...)
		},
		LookPath: func(name string) (string, error) { return "/bin/" + name, nil },
	})

	// Assert
	require.NotEmpty(t, report.Findings, "control: the run reports its checks, so the loop below has something to check")
	for _, f := range report.Findings {
		assert.NotEqual(t, SeverityError, f.Severity, "%+v", f)
	}
}
