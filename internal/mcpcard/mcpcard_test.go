package mcpcard

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func remote(name, url string) Server {
	return Server{Name: name, Description: "Search the docs.", Transport: "http", URL: url}
}

func TestBuild_RemoteServersBecomeCards(t *testing.T) {
	tests := []struct {
		name   string
		server Server
		opts   Options
		want   Card
	}{
		{
			name:   "streamable http",
			server: remote("docs", "https://docs.acme.test/mcp"),
			opts:   Options{Namespace: "test.acme", Version: "1.4.0"},
			want: Card{
				Schema: SchemaURL, Name: "test.acme/docs", Description: "Search the docs.", Version: "1.4.0",
				Remotes: []Remote{{Type: "streamable-http", URL: "https://docs.acme.test/mcp"}},
			},
		},
		{
			name:   "sse",
			server: Server{Name: "feed", Description: "Feed.", Transport: "sse", URL: "https://feed.acme.test/sse"},
			opts:   Options{Namespace: "test.acme", Version: "1.0.0"},
			want: Card{
				Schema: SchemaURL, Name: "test.acme/feed", Description: "Feed.", Version: "1.0.0",
				Remotes: []Remote{{Type: "sse", URL: "https://feed.acme.test/sse"}},
			},
		},
		{
			name:   "a url with no transport is streamable http, a missing version is 0.0.0",
			server: Server{Name: "api", Description: "API.", URL: "http://localhost:8080/mcp"},
			opts:   Options{Namespace: "test.acme"},
			want: Card{
				Schema: SchemaURL, Name: "test.acme/api", Description: "API.", Version: DefaultVersion,
				Remotes: []Remote{{Type: "streamable-http", URL: "http://localhost:8080/mcp"}},
			},
		},
		{
			name:   "an unusable name is made safe and an empty description is filled",
			server: Server{Name: "my server/v2", Transport: "http", URL: "https://a.test/mcp"},
			opts:   Options{Namespace: "test.acme", Version: "1.0.0"},
			want: Card{
				Schema: SchemaURL, Name: "test.acme/my-server-v2", Description: "MCP server my server/v2", Version: "1.0.0",
				Remotes: []Remote{{Type: "streamable-http", URL: "https://a.test/mcp"}},
			},
		},
		{
			name:   "a long description is cut to the limit",
			server: Server{Name: "long", Description: strings.Repeat("é", 150), Transport: "http", URL: "https://a.test/mcp"},
			opts:   Options{Namespace: "test.acme", Version: "1.0.0"},
			want: Card{
				Schema: SchemaURL, Name: "test.acme/long", Description: strings.Repeat("é", MaxDescription), Version: "1.0.0",
				Remotes: []Remote{{Type: "streamable-http", URL: "https://a.test/mcp"}},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			cards, _, err := Build([]Server{tt.server}, tt.opts)

			// Assert
			require.NoError(t, err)
			require.Len(t, cards, 1)
			assert.Equal(t, tt.want, cards[0])
		})
	}
}

func TestBuild_StdioServersAreSkippedWithANote(t *testing.T) {
	servers := []Server{
		{Name: "local", Description: "Local.", Transport: "stdio", Command: "npx", Args: []string{"-y", "x"}},
		{Name: "bare", Description: "No transport, no url.", Command: "tool"},
		remote("docs", "https://docs.acme.test/mcp"),
	}

	cards, notes, err := Build(servers, Options{Namespace: "test.acme", Version: "1.0.0"})

	require.NoError(t, err)
	require.Len(t, cards, 1)
	assert.Equal(t, "test.acme/docs", cards[0].Name)
	require.Len(t, notes, 2)
	assert.Contains(t, notes[0], `"local"`)
	assert.Contains(t, notes[0], "stdio")
	assert.Contains(t, notes[1], `"bare"`)
}

func TestBuild_URLWithQueryOrFragmentIsSkippedWithANote(t *testing.T) {
	tests := []struct{ name, url string }{
		{"api key in the query", "https://a.test/mcp?api_key=SECRETVALUE"},
		{"placeholder in the query", "https://a.test/mcp?token=${TOKEN}"},
		{"fragment", "https://a.test/mcp#SECRETVALUE"},
		{"empty query marker", "https://a.test/mcp?"},
		{"placeholder in the path", "https://a.test/${TENANT}/mcp"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			servers := []Server{remote("leaky", tt.url), remote("docs", "https://docs.test/mcp")}

			cards, notes, err := Build(servers, Options{Namespace: "test.acme", Version: "1.0.0"})

			require.NoError(t, err)
			require.Len(t, cards, 1)
			assert.Equal(t, "test.acme/docs", cards[0].Name)
			require.Len(t, notes, 1)
			assert.Contains(t, notes[0], `"leaky"`)
			assert.NotContains(t, notes[0], "SECRETVALUE")
			assert.NotContains(t, notes[0], "TOKEN")
		})
	}
}

func TestBuild_HeadersCarryNamesAndNeverValues(t *testing.T) {
	s := remote("docs", "https://docs.acme.test/mcp")
	s.Headers = []Header{{Name: "X-Region"}, {Name: "Authorization", Secret: true}}

	cards, _, err := Build([]Server{s}, Options{Namespace: "test.acme", Version: "1.0.0"})
	require.NoError(t, err)
	data, err := cards[0].JSON()
	require.NoError(t, err)

	assert.Equal(t, []Input{
		{Name: "Authorization", IsRequired: true, IsSecret: true},
		{Name: "X-Region", IsRequired: true},
	}, cards[0].Remotes[0].Headers)
	assert.NotContains(t, string(data), "Bearer")
	assert.NotContains(t, string(data), `"value"`)
}

func TestBuild_CardsAreSortedByName(t *testing.T) {
	servers := []Server{remote("zeta", "https://z.test/mcp"), remote("alpha", "https://a.test/mcp")}

	cards, _, err := Build(servers, Options{Namespace: "test.acme", Version: "1.0.0"})

	require.NoError(t, err)
	require.Len(t, cards, 2)
	assert.Equal(t, "test.acme/alpha", cards[0].Name)
	assert.Equal(t, "test.acme/zeta", cards[1].Name)
}

func TestBuild_Refusals(t *testing.T) {
	tests := []struct {
		name    string
		servers []Server
		opts    Options
		want    string
	}{
		{"no namespace", []Server{remote("docs", "https://a.test/mcp")}, Options{}, "namespace"},
		{"bad namespace", []Server{remote("docs", "https://a.test/mcp")}, Options{Namespace: "a/b"}, "namespace"},
		{"credentials in the url", []Server{remote("docs", "https://u:p@a.test/mcp")}, Options{Namespace: "x.y"}, "credentials"},
		{"relative url", []Server{remote("docs", "/mcp")}, Options{Namespace: "x.y"}, "absolute"},
		{"ftp url", []Server{remote("docs", "ftp://a.test/mcp")}, Options{Namespace: "x.y"}, "http"},
		{"unknown transport", []Server{{Name: "docs", Transport: "carrier-pigeon", URL: "https://a.test"}}, Options{Namespace: "x.y"}, "carrier-pigeon"},
		{"https only", []Server{remote("docs", "http://a.test/mcp")}, Options{Namespace: "x.y", HTTPSOnly: true}, "https"},
		{"duplicate names", []Server{remote("docs", "https://a.test/mcp"), remote("docs", "https://b.test/mcp")}, Options{Namespace: "x.y"}, "twice"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cards, _, err := Build(tt.servers, tt.opts)

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
			assert.Empty(t, cards)
		})
	}
}

func TestCard_JSONIsIndentedAndEndsWithANewline(t *testing.T) {
	cards, _, err := Build([]Server{remote("docs", "https://a.test/mcp")}, Options{Namespace: "test.acme", Version: "1.0.0"})
	require.NoError(t, err)

	data, err := cards[0].JSON()

	require.NoError(t, err)
	assert.True(t, strings.HasSuffix(string(data), "}\n"))
	var back map[string]any
	require.NoError(t, json.Unmarshal(data, &back))
	assert.Equal(t, SchemaURL, back["$schema"])
	m, err := cards[0].Map()
	require.NoError(t, err)
	assert.Equal(t, back, m)
}

func TestNamespaces(t *testing.T) {
	tests := []struct{ in, want string }{
		{"example.com", "com.example"},
		{"docs.acme.test", "test.acme.docs"},
		{"Example.COM.", "com.example"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, NamespaceFromDomain(tt.in), tt.in)
	}
	repo := []struct{ in, want string }{
		{"acme/skills", "io.github.acme"},
		{"git.acme.test/acme/skills", "test.acme.git"},
		{"", ""},
		{"skills", ""},
	}
	for _, tt := range repo {
		assert.Equal(t, tt.want, NamespaceFromRepo(tt.in), tt.in)
	}
}
