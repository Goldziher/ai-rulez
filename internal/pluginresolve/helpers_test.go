package pluginresolve_test

import (
	"context"
	"net/http"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/pluginresolve"
	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

func TestMain(m *testing.M) {
	testutil.CeilGit()
	os.Exit(m.Run())
}

// isolate keeps the fetch cache out of the real home.
func isolate(t *testing.T) {
	t.Helper()
	testutil.IsolateHome(t)
	testutil.GitIdentity(t)
}

// load loads the project at dir with the plugin resolvers wired in.
func load(t *testing.T, dir string, opts ...config.LoadOption) (*config.Config, error) {
	t.Helper()
	return loadCtx(context.Background(), dir, opts...)
}

func loadCtx(ctx context.Context, dir string, opts ...config.LoadOption) (*config.Config, error) {
	all := append([]config.LoadOption{config.WithResolvers(pluginresolve.Resolvers("", nil)), config.WithoutLocal()}, opts...)
	return config.LoadConfig(ctx, dir, all...)
}

func mustLoad(t *testing.T, dir string, opts ...config.LoadOption) *config.Config {
	t.Helper()
	cfg, err := load(t, dir, opts...)
	require.NoError(t, err)
	return cfg
}

func skillNames(cfg *config.Config) []string {
	var out []string
	for _, s := range cfg.Content.Skills {
		out = append(out, s.Name)
	}
	return out
}

func fileNames(files []config.ContentFile) []string {
	out := []string{}
	for _, f := range files {
		out = append(out, f.Name)
	}
	return out
}

// WithClient is a context carrying the client an https marketplace index is fetched with.
func WithClient(c *http.Client) context.Context {
	return pluginresolve.WithHTTPClient(context.Background(), c)
}
