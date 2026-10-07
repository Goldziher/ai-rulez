package includes

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/diag"
)

type warnRecorder struct{ warns []string }

func (r *warnRecorder) Debug(string, ...any) {}
func (r *warnRecorder) Info(string, ...any)  {}
func (r *warnRecorder) Warn(msg string, _ ...any) {
	r.warns = append(r.warns, msg)
}
func (r *warnRecorder) Error(string, ...any) {}

// An unresolved include that fails the command is reported once, by the error:
// the warning would repeat the same text just before it.
func TestResolveIncludes_UnresolvedIncludeWarnsOnlyWhenTolerated(t *testing.T) {
	tests := []struct {
		name      string
		tolerated bool
		wantErr   bool
		wantWarns int
	}{
		{"fails: the error carries the message", false, true, 0},
		{"tolerated: the warning is the only report", true, false, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir := t.TempDir()
			cfgDir := filepath.Join(dir, ".ai-rulez")
			require.NoError(t, os.MkdirAll(cfgDir, 0o755))
			rec := &warnRecorder{}
			cfg := &config.Config{
				BaseDir: dir, ConfigDir: cfgDir, Diag: diag.New(nil), Host: ambient.Host{Log: rec},
				Includes: []config.IncludeConfig{{Name: "gone", Source: filepath.Join(dir, "missing")}},
				Content:  &config.ContentTree{Domains: map[string]*config.Domain{}},
			}
			ctx := context.Background()
			if tt.tolerated {
				ctx = config.WithUnresolvedIncludesTolerated(ctx)
			}

			// Act
			_, err := NewResolver(dir, "").ResolveIncludes(ctx, cfg)

			// Assert
			assert.Equal(t, tt.wantErr, err != nil)
			assert.Len(t, rec.warns, tt.wantWarns)
		})
	}
}
