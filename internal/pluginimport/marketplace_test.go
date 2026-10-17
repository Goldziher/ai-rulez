package pluginimport_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/pluginimport"
	"github.com/Goldziher/ai-rulez/v5/internal/pluginimport/plugintest"
	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

const sha40 = "0123456789abcdef0123456789abcdef01234567"

func TestParseIndex_Sources(t *testing.T) {
	tests := []struct {
		name    string
		format  string
		doc     string
		want    pluginimport.Source
		problem string
	}{
		{name: "claude relative path", format: pluginimport.IndexClaude,
			doc:  `{"name":"m","plugins":[{"name":"p","source":"./plugins/p"}]}`,
			want: pluginimport.Source{Kind: pluginimport.SourcePath, Path: "plugins/p"}},
		{name: "plugin root is prepended to a bare name", format: pluginimport.IndexClaude,
			doc:  `{"name":"m","metadata":{"pluginRoot":"./plugins"},"plugins":[{"name":"p","source":"p"}]}`,
			want: pluginimport.Source{Kind: pluginimport.SourcePath, Path: "plugins/p"}},
		{name: "plugin root does not double an explicit path", format: pluginimport.IndexClaude,
			doc:  `{"name":"m","metadata":{"pluginRoot":"./plugins"},"plugins":[{"name":"p","source":"./plugins/p"}]}`,
			want: pluginimport.Source{Kind: pluginimport.SourcePath, Path: "plugins/p"}},
		{name: "github object with ref and sha", format: pluginimport.IndexClaude,
			doc:  `{"name":"m","plugins":[{"name":"p","source":{"source":"github","repo":"acme/tools","ref":"v1","sha":"` + sha40 + `"}}]}`,
			want: pluginimport.Source{Kind: pluginimport.SourceRepo, URL: "https://github.com/acme/tools.git", Ref: "v1", SHA: sha40}},
		{name: "url object", format: pluginimport.IndexClaude,
			doc:  `{"name":"m","plugins":[{"name":"p","source":{"source":"url","url":"https://example.com/x.git","ref":"main"}}]}`,
			want: pluginimport.Source{Kind: pluginimport.SourceRepo, URL: "https://example.com/x.git", Ref: "main"}},
		{name: "git object", format: pluginimport.IndexClaude,
			doc:  `{"name":"m","plugins":[{"name":"p","source":{"source":"git","url":"git@example.com:acme/x.git"}}]}`,
			want: pluginimport.Source{Kind: pluginimport.SourceRepo, URL: "git@example.com:acme/x.git"}},
		{name: "git-subdir object", format: pluginimport.IndexCodex,
			doc:  `{"name":"m","plugins":[{"name":"p","source":{"source":"git-subdir","url":"https://example.com/x.git","path":"sub/dir","sha":"` + sha40 + `"}}]}`,
			want: pluginimport.Source{Kind: pluginimport.SourceRepo, URL: "https://example.com/x.git", Subdir: "sub/dir", SHA: sha40}},
		{name: "codex local object", format: pluginimport.IndexCodex,
			doc:  `{"name":"m","plugins":[{"name":"p","source":{"source":"local","path":"./plugins/p"}}]}`,
			want: pluginimport.Source{Kind: pluginimport.SourcePath, Path: "plugins/p"}},
		{name: "cursor relative path", format: pluginimport.IndexCursor,
			doc:  `{"name":"m","plugins":[{"name":"p","source":"plugins/p"}]}`,
			want: pluginimport.Source{Kind: pluginimport.SourcePath, Path: "plugins/p"}},
		{name: "parent traversal is refused", doc: `{"plugins":[{"name":"p","source":"../outside"}]}`, problem: "leaves the directory"},
		{name: "nested traversal is refused", doc: `{"plugins":[{"name":"p","source":"./a/../../outside"}]}`, problem: "leaves the directory"},
		{name: "absolute path is refused", doc: `{"plugins":[{"name":"p","source":"/etc"}]}`, problem: "relative"},
		{name: "home path is refused", doc: `{"plugins":[{"name":"p","source":"~/x"}]}`, problem: "relative"},
		{name: "windows drive path is refused", doc: `{"plugins":[{"name":"p","source":"C:/x"}]}`, problem: "relative"},
		{name: "backslash is refused", doc: `{"plugins":[{"name":"p","source":".\\x"}]}`, problem: "backslash"},
		{name: "marketplace root as source is refused", doc: `{"plugins":[{"name":"p","source":"./"}]}`, problem: "marketplace root"},
		{name: "local object traversal is refused", doc: `{"plugins":[{"name":"p","source":{"source":"local","path":"../x"}}]}`, problem: "leaves the directory"},
		{name: "subdir traversal is refused", doc: `{"plugins":[{"name":"p","source":{"source":"git-subdir","url":"https://e.com/x.git","path":"../x"}}]}`, problem: "leaves the directory"},
		{name: "credentials in a url are refused", doc: `{"plugins":[{"name":"p","source":{"source":"url","url":"https://user:token@example.com/x.git"}}]}`, problem: "credentials"},
		{name: "token as user is refused", doc: `{"plugins":[{"name":"p","source":{"source":"url","url":"https://ghp_abc@example.com/x.git"}}]}`, problem: "credentials"},
		{name: "plain http is refused", doc: `{"plugins":[{"name":"p","source":{"source":"url","url":"http://example.com/x.git"}}]}`, problem: "http://"},
		{name: "transport helper is refused", doc: `{"plugins":[{"name":"p","source":{"source":"url","url":"ext::sh -c id"}}]}`, problem: "transport helper"},
		{name: "option-looking url is refused", doc: `{"plugins":[{"name":"p","source":{"source":"url","url":"--upload-pack=x"}}]}`, problem: "git option"},
		{name: "option-looking ref is refused", doc: `{"plugins":[{"name":"p","source":{"source":"url","url":"https://e.com/x.git","ref":"--evil"}}]}`, problem: "ref"},
		{name: "short sha is refused", doc: `{"plugins":[{"name":"p","source":{"source":"url","url":"https://e.com/x.git","sha":"abc123"}}]}`, problem: "40-character"},
		{name: "local url is refused as a repository", doc: `{"plugins":[{"name":"p","source":{"source":"url","url":"./local"}}]}`, problem: "git URL"},
		{name: "invalid github repo is refused", doc: `{"plugins":[{"name":"p","source":{"source":"github","repo":"not a repo"}}]}`, problem: "owner/repo"},
		{name: "npm source is not supported", doc: `{"plugins":[{"name":"p","source":{"source":"npm","package":"x"}}]}`, problem: "not supported"},
		{name: "source object without a type", doc: `{"plugins":[{"name":"p","source":{"repo":"a/b"}}]}`, problem: "no type"},
		{name: "source of the wrong type", doc: `{"plugins":[{"name":"p","source":5}]}`, problem: "neither a path nor an object"},
		{name: "missing source", doc: `{"plugins":[{"name":"p"}]}`, problem: "no source"},
		{name: "invalid plugin name", doc: `{"plugins":[{"name":"../p","source":"./p"}]}`, problem: "not a valid plugin name"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange / Act
			ix, err := pluginimport.ParseIndex([]byte(tt.doc), tt.format)

			// Assert
			require.NoError(t, err)
			require.Len(t, ix.Plugins, 1)
			e := ix.Plugins[0]
			if tt.problem != "" {
				assert.Contains(t, e.Problem, tt.problem)
				return
			}
			assert.Empty(t, e.Problem)
			assert.Equal(t, tt.want, e.Source)
		})
	}
}

func TestParseIndex_Document(t *testing.T) {
	tests := []struct {
		name    string
		data    string
		wantErr string
		names   []string
		problem map[string]string
	}{
		{name: "lists plugins in order", data: `{"name":"m","plugins":[{"name":"b","source":"./b"},{"name":"a","source":"./a"}]}`, names: []string{"b", "a"}},
		{name: "duplicate name keeps the first usable", data: `{"plugins":[{"name":"a","source":"./a"},{"name":"a","source":"./b"}]}`,
			names: []string{"a", "a"}, problem: map[string]string{"a": "duplicate"}},
		{name: "one bad entry does not spoil the others", data: `{"plugins":[7,{"name":"ok","source":"./ok"}]}`, names: []string{"(entry 0)", "ok"}},
		{name: "not json", data: `{`, wantErr: "not valid JSON"},
		{name: "array instead of object", data: `[]`, wantErr: "not valid JSON"},
		{name: "plugin root escaping", data: `{"metadata":{"pluginRoot":"../x"},"plugins":[]}`, wantErr: "pluginRoot"},
		{name: "empty marketplace", data: `{}`, names: []string{}},
		{name: "too many plugins", data: `{"plugins":[` + strings.Repeat(`{"name":"a","source":"./a"},`, pluginimport.MaxIndexPlugins+1) + `{"name":"z","source":"./z"}]}`, wantErr: "more than the limit"},
		{name: "oversize document", data: `{"name":"` + strings.Repeat("a", pluginimport.MaxIndexBytes) + `"}`, wantErr: "larger than"},
		{name: "deeply nested hostile json", data: strings.Repeat("[", 100000), wantErr: "not valid JSON"},
		{name: "wrong field types", data: `{"name":5,"plugins":"x"}`, wantErr: "not valid JSON"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange / Act
			ix, err := pluginimport.ParseIndex([]byte(tt.data), pluginimport.IndexClaude)

			// Assert
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			got := ix.Names()
			if len(got) == 0 {
				got = []string{}
			}
			assert.Equal(t, tt.names, got)
			for _, e := range ix.Plugins {
				if want, ok := tt.problem[e.Name]; ok && e.Problem != "" {
					assert.Contains(t, e.Problem, want)
				}
			}
		})
	}
}

func TestParseIndex_InlineComponentsAndStrict(t *testing.T) {
	// Arrange
	doc := `{"plugins":[{"name":"p","source":"./p","strict":false,"description":"d","version":"1.2.3",
		"commands":["./c.md"],"mcpServers":{"s":{"command":"x"}}}]}`

	// Act
	ix, err := pluginimport.ParseIndex([]byte(doc), pluginimport.IndexClaude)

	// Assert
	require.NoError(t, err)
	e := ix.Plugins[0]
	require.NotNil(t, e.Strict)
	assert.False(t, *e.Strict)
	assert.Equal(t, "1.2.3", e.Version)
	assert.Contains(t, e.Inline, "commands")
	assert.Contains(t, e.Inline, "mcpServers")
	assert.NotContains(t, e.Inline, "skills")
}

func TestSource_DescribeIsCredentialFree(t *testing.T) {
	// Arrange
	src := pluginimport.Source{Kind: pluginimport.SourceRepo, URL: "https://tok@example.com/x.git", Subdir: "s", SHA: sha40}

	// Act / Assert
	assert.Equal(t, "https://example.com/x.git#s@"+sha40, src.Describe())
	assert.Equal(t, "./plugins/p", pluginimport.Source{Kind: pluginimport.SourcePath, Path: "plugins/p"}.Describe())
}

func TestFindIndex_Locations(t *testing.T) {
	tests := []struct {
		name   string
		path   string
		format string
	}{
		{"claude", ".claude-plugin/marketplace.json", pluginimport.IndexClaude},
		{"codex", ".agents/plugins/marketplace.json", pluginimport.IndexCodex},
		{"cursor", ".cursor-plugin/marketplace.json", pluginimport.IndexCursor},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir := t.TempDir()
			testutil.WriteTree(t, dir, map[string]string{tt.path: `{"name":"m","plugins":[{"name":"p","source":"./p"}]}`})

			// Act
			ix, err := pluginimport.ReadIndex(dir)

			// Assert
			require.NoError(t, err)
			assert.Equal(t, tt.format, ix.Format)
			assert.Equal(t, []string{"p"}, ix.Names())
		})
	}
}

func TestReadIndex_Failures(t *testing.T) {
	t.Run("no index", func(t *testing.T) {
		_, err := pluginimport.ReadIndex(t.TempDir())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no marketplace index")
	})
	t.Run("symlinked index is not followed", func(t *testing.T) {
		dir, outside := t.TempDir(), t.TempDir()
		testutil.WriteTree(t, outside, map[string]string{"m.json": `{"plugins":[]}`})
		testutil.WriteTree(t, dir, map[string]string{".claude-plugin/.keep": ""})
		testutil.SymlinkOrSkip(t, outside+"/m.json", dir+"/.claude-plugin/marketplace.json")
		_, err := pluginimport.ReadIndex(dir)
		require.Error(t, err)
	})
	t.Run("symlinked index directory is not followed", func(t *testing.T) {
		dir, outside := t.TempDir(), t.TempDir()
		testutil.WriteTree(t, outside, map[string]string{"marketplace.json": `{"plugins":[]}`})
		testutil.SymlinkOrSkip(t, outside, dir+"/.claude-plugin")
		_, err := pluginimport.ReadIndex(dir)
		require.Error(t, err)
	})
	t.Run("index with a hostile document", func(t *testing.T) {
		dir := t.TempDir()
		testutil.WriteTree(t, dir, plugintest.ClaudeMarketplace("m"))
		testutil.WriteTree(t, dir, map[string]string{".claude-plugin/marketplace.json": "{"})
		_, err := pluginimport.ReadIndex(dir)
		require.Error(t, err)
	})
}

func TestCleanRel(t *testing.T) {
	tests := []struct {
		in, want string
		wantErr  bool
	}{
		{"./a/b", "a/b", false},
		{"a//b/../c", "a/c", false},
		{".", ".", false},
		{"", "", true},
		{"..", "", true},
		{"../a", "", true},
		{"a/../../b", "", true},
		{"/abs", "", true},
		{"~/x", "", true},
		{"C:\\x", "", true},
		{"a\\b", "", true},
		{"a\x00b", "", true},
		{"a\nb", "", true},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%q", tt.in), func(t *testing.T) {
			got, err := pluginimport.CleanRel(tt.in)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSafeJoin_RefusesSymlinkedComponents(t *testing.T) {
	// Arrange
	root, outside := t.TempDir(), t.TempDir()
	testutil.WriteTree(t, outside, map[string]string{"x/file.md": "x"})
	testutil.SymlinkOrSkip(t, outside, root+"/link")

	// Act
	_, err := pluginimport.SafeJoin(root, "link/x")
	_, errMissing := pluginimport.SafeJoin(root, "not/there")

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "symlink")
	assert.NoError(t, errMissing)
}
