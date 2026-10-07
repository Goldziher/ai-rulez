package setup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/skillsearch"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// userConfig points XDG_CONFIG_HOME at a directory holding the given user config.
func userConfig(t *testing.T, body string) func(string) string {
	t.Helper()
	dir := t.TempDir()
	if body != "" {
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "ai-rulez"), 0o750))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "ai-rulez", "config.toml"), []byte(body), 0o600))
	}
	return func(name string) string {
		if name == "XDG_CONFIG_HOME" {
			return dir
		}
		return ""
	}
}

func projectConfig(search *skillsearch.Config) *config.Config {
	return &config.Config{ConfigDir: "/proj/.ai-rulez", Search: search}
}

func TestResolve_DefaultsToLexical(t *testing.T) {
	t.Parallel()
	r, err := Resolve(projectConfig(nil), Options{Getenv: userConfig(t, "")})
	require.NoError(t, err)
	assert.Equal(t, skillsearch.ModeLexical, r.Search.Mode)
	assert.False(t, r.Wants())
	assert.Equal(t, skillsearch.DefaultIndexDir, r.Search.IndexDir)
	assert.Empty(t, r.Command)
}

func TestResolve_ModePrecedence(t *testing.T) {
	t.Parallel()
	user := userConfig(t, "[search]\nmode = \"vector\"\n")
	repo := projectConfig(&skillsearch.Config{Mode: skillsearch.ModeHybrid})
	tests := []struct {
		name string
		cfg  *config.Config
		opts Options
		want string
	}{
		{"repo only", repo, Options{Getenv: userConfig(t, "")}, skillsearch.ModeHybrid},
		{"user over repo", repo, Options{Getenv: user}, skillsearch.ModeVector},
		{"flag over user", repo, Options{Getenv: user, Mode: skillsearch.ModeLexical}, skillsearch.ModeLexical},
		{"env over user", repo, Options{Getenv: func(n string) string {
			if n == ModeEnv {
				return "lexical"
			}
			return user(n)
		}}, skillsearch.ModeLexical},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := Resolve(tt.cfg, tt.opts)
			require.NoError(t, err)
			assert.Equal(t, tt.want, r.Search.Mode)
		})
	}
}

func TestResolve_RejectsABadTable(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		cfg  *config.Config
		opts Options
	}{
		{"repo mode", projectConfig(&skillsearch.Config{Mode: "semantic"}), Options{Getenv: userConfig(t, "")}},
		{"flag mode", projectConfig(nil), Options{Getenv: userConfig(t, ""), Mode: "nope"}},
		{"env mode", projectConfig(nil), Options{Getenv: func(n string) string {
			if n == ModeEnv {
				return "nope"
			}
			return ""
		}}},
		{"index dir escapes", projectConfig(&skillsearch.Config{IndexDir: "../x"}), Options{Getenv: userConfig(t, "")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Resolve(tt.cfg, tt.opts)
			require.Error(t, err)
			assert.Contains(t, err.Error(), skillsearch.CodeConfigInvalid)
		})
	}
}

func TestResolve_CommandProviderTrust(t *testing.T) {
	t.Parallel()
	repo := projectConfig(&skillsearch.Config{Embeddings: &skillsearch.EmbeddingsConfig{Command: []string{"./embed.sh"}, PassEnv: []string{"TOKEN"}}})
	userCmd := userConfig(t, "[search.embeddings]\ncommand = [\"/usr/local/bin/embed\"]\n")
	tests := []struct {
		name    string
		opts    Options
		want    []string
		wantNot bool
	}{
		{"repository command is ignored", Options{Getenv: userConfig(t, "")}, nil, true},
		{"repository command with --allow-exec", Options{Getenv: userConfig(t, ""), AllowExec: true}, []string{"./embed.sh"}, false},
		{"user command wins and needs no flag", Options{Getenv: userCmd}, []string{"/usr/local/bin/embed"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := Resolve(repo, tt.opts)
			require.NoError(t, err)
			assert.Equal(t, tt.want, r.Command)
			if tt.wantNot {
				require.Len(t, r.Notes, 1)
				assert.Contains(t, r.Notes[0], "cannot run a program")
			}
		})
	}
}

func TestResolve_ModelAndProvider(t *testing.T) {
	t.Parallel()
	get := userConfig(t, "[llm]\nembedding_model = \"text-embedding-3-small\"\nbase_url = \"https://gw.example/v1\"\nprovider = \"openai\"\n")
	r, err := Resolve(projectConfig(nil), Options{Getenv: get})
	require.NoError(t, err)
	assert.Equal(t, "text-embedding-3-small", r.Model)
	p := r.Describe()
	assert.Equal(t, "openai@gw.example", p.Fingerprint)
	assert.True(t, p.Network)

	over, err := Resolve(projectConfig(&skillsearch.Config{Embeddings: &skillsearch.EmbeddingsConfig{Model: "other"}}), Options{Getenv: get})
	require.NoError(t, err)
	assert.Equal(t, "other", over.Model, "[search.embeddings] model overrides [llm] embedding_model")
}

func TestEmbedder_NoModelIsAnError(t *testing.T) {
	t.Parallel()
	r, err := Resolve(projectConfig(nil), Options{Getenv: userConfig(t, "")})
	require.NoError(t, err)
	_, release, err := r.Embedder()
	defer release()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no embedding model")
}

func TestEmbedder_CommandNeedsNoNetwork(t *testing.T) {
	t.Parallel()
	r, err := Resolve(projectConfig(nil), Options{Getenv: userConfig(t, "[search.embeddings]\ncommand = [\"/bin/embed\"]\nmodel = \"local\"\n")})
	require.NoError(t, err)
	emb, release, err := r.Embedder()
	defer release()
	require.NoError(t, err)
	assert.Equal(t, "command:/bin/embed", emb.Fingerprint())
	assert.Equal(t, "local", emb.Model())
	assert.False(t, r.Describe().Network)
}

// withEnv layers extra variables over a getenv.
func withEnv(base func(string) string, extra map[string]string) func(string) string {
	return func(name string) string {
		if v, ok := extra[name]; ok {
			return v
		}
		return base(name)
	}
}

func TestQueryLog_IsHonouredOnlyFromUserScope(t *testing.T) {
	t.Parallel()
	repoOn := projectConfig(&skillsearch.Config{LogQueries: true})
	tests := []struct {
		name     string
		cfg      *config.Config
		getenv   func(string) string
		want     bool
		wantNote bool
	}{
		{"repository alone cannot enable it", repoOn, userConfig(t, ""), false, true},
		{"user config enables it", projectConfig(nil), userConfig(t, "[search]\nlog_queries = true\n"), true, false},
		{"user config enables it beside the repository", repoOn, userConfig(t, "[search]\nlog_queries = true\n"), true, false},
		{"environment enables it", projectConfig(nil), withEnv(userConfig(t, ""), map[string]string{LogQueriesEnv: "1"}), true, false},
		{"environment 0 is off", projectConfig(nil), withEnv(userConfig(t, ""), map[string]string{LogQueriesEnv: "0"}), false, false},
		{"off by default", projectConfig(nil), userConfig(t, ""), false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange and Act
			r, err := Resolve(tt.cfg, Options{Getenv: tt.getenv})

			// Assert
			require.NoError(t, err)
			assert.Equal(t, tt.want, r.QueryLog(nil) != nil)
			noted := false
			for _, n := range r.Notes {
				noted = noted || strings.Contains(n, "log_queries")
			}
			assert.Equal(t, tt.wantNote, noted)
		})
	}
}

func TestQueryLogAndIndexPath(t *testing.T) {
	t.Parallel()
	r, err := Resolve(projectConfig(&skillsearch.Config{IndexDir: "search-index"}), Options{Getenv: withEnv(userConfig(t, ""), map[string]string{LogQueriesEnv: "true"})})
	require.NoError(t, err)
	log := r.QueryLog(nil)
	require.NotNil(t, log)
	assert.Equal(t, filepath.Join("/proj/.ai-rulez", "local", skillsearch.QueryLogFile), log.Path)
	assert.Same(t, log, r.QueryLog(nil), "one log per setup, so its full-log warning is given once, not per call")
	p, err := r.IndexPath()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join("/proj/.ai-rulez", "search-index"), p)

	off, err := Resolve(projectConfig(nil), Options{Getenv: userConfig(t, "")})
	require.NoError(t, err)
	assert.Nil(t, off.QueryLog(nil))
}

func TestRequestModel(t *testing.T) {
	t.Parallel()
	get := userConfig(t, "[llm]\nembedding_model = \"base\"\nprovider = \"gemini\"\nbackend = \"openaicompat\"\n")
	plain, err := Resolve(projectConfig(nil), Options{Getenv: get})
	require.NoError(t, err)
	assert.Empty(t, plain.requestModel(), "no override: the client routes on embedding_model itself")

	over, err := Resolve(projectConfig(&skillsearch.Config{Embeddings: &skillsearch.EmbeddingsConfig{Model: "other"}}), Options{Getenv: get})
	require.NoError(t, err)
	assert.Equal(t, "other", over.requestModel(), "openaicompat sends the override verbatim")

	lit := userConfig(t, "[llm]\nembedding_model = \"base\"\nprovider = \"gemini\"\nbackend = \"literllm\"\n")
	routed, err := Resolve(projectConfig(&skillsearch.Config{Embeddings: &skillsearch.EmbeddingsConfig{Model: "other"}}), Options{Getenv: lit})
	require.NoError(t, err)
	assert.Equal(t, "gemini/other", routed.requestModel(), "literllm routes on provider/model")
}

func TestResolve_UserScopeOwnsTheAbstentionThreshold(t *testing.T) {
	t.Parallel()
	repo := projectConfig(&skillsearch.Config{VectorMinSim: 1})
	tests := []struct {
		name string
		user string
		want float64
	}{
		{"repo value without a user value", "", 1},
		{"user value over the repo's", "[search]\nvector_min_sim = 0.4\n", 0.4},
		{"an explicit user 0 switches it off", "[search]\nvector_min_sim = 0\n", 0},
		{"a user table without the key keeps the repo's", "[search]\nmode = \"lexical\"\n", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange / Act
			r, err := Resolve(repo, Options{Getenv: userConfig(t, tt.user)})

			// Assert
			require.NoError(t, err)
			assert.InDelta(t, tt.want, r.Search.VectorMinSim, 1e-9)
		})
	}
}
