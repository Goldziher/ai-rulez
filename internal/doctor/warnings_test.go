package doctor

import (
	"context"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/diag"
	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

// Doctor loads the project twice (the lock check reads it without the local
// overlay) and renders it twice (drift, gitignore); a warning about the project
// is still said once.
func TestRun_SaysEachWarningOnce(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
		want  string
	}{
		{
			name: "hook event a harness cannot express",
			files: map[string]string{".ai-rulez/config.toml": baseConfig +
				"\n[[hooks]]\nevent = \"pre_tool_use\"\n[[hooks.hooks]]\ncommand = \"echo hi\"\n"},
			want: "[[hooks]] not generated for claude",
		},
		{
			name: "malformed frontmatter",
			files: map[string]string{
				".ai-rulez/config.toml": baseConfig,
				".ai-rulez/rules/r.md":  "---\ndescription: x\nbogus: [\n---\nbody\n",
			},
			want: "malformed YAML frontmatter",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir := project(t, tt.files)
			rec := &testutil.LogRecorder{}
			t.Cleanup(diag.SetDefaultSink(rec.Warn))
			load := func(ctx context.Context, opts ...config.LoadOption) (*config.Config, error) {
				opts = append([]config.LoadOption{config.WithHost(ambient.Host{Log: rec})}, opts...)
				return config.LoadConfig(ctx, dir, append(opts, config.WithoutRemote())...)
			}

			// Act
			Run(context.Background(), Options{Load: load, LookPath: func(n string) (string, error) { return n, nil }})

			// Assert
			n := 0
			for _, line := range rec.Level("WARN") {
				if strings.Contains(line, tt.want) {
					n++
				}
			}
			if n != 1 {
				t.Fatalf("%q warned %d times, want once:\n%s", tt.want, n, rec.String())
			}
		})
	}
}
