package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/Goldziher/ai-rulez/internal/generator"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func servedSkill(id, domain, desc string, keywords []string, extra ...generator.ServedSkillFile) generator.ServedSkill {
	body := fmt.Sprintf("---\nname: %s\ndescription: %s\n# Content-Hash: abc\n---\n\n# %s\n", id, desc, id)
	files := []generator.ServedSkillFile{{RelPath: "SKILL.md", Content: []byte(body)}}
	files = append(files, extra...)
	return generator.ServedSkill{ID: id, Domain: domain, Source: ".ai-rulez/skills/" + id + "/SKILL.md", Keywords: keywords, Files: files}
}

func testServed() []generator.ServedSkill {
	return []generator.ServedSkill{
		servedSkill("git-workflow", "", "Follow the team git conventions for branching and commits", []string{"git", "branch"}),
		servedSkill("pdf-processing", "docs", "Extract and fill PDF documents", []string{"pdf"},
			generator.ServedSkillFile{RelPath: "references/FORMS.md", Content: []byte("forms")},
			generator.ServedSkillFile{RelPath: "scripts/extract.py", Content: []byte("print(1)\n")}),
		servedSkill("refund-policy", "billing", "Process customer refund requests", []string{"refunds", "billing"}),
	}
}

func TestBuildCatalog_Filters(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		filter SkillFilter
		want   []string
	}{
		{"no filter", SkillFilter{}, []string{"git-workflow", "pdf-processing", "refund-policy"}},
		{"domain", SkillFilter{Domains: []string{"docs"}}, []string{"pdf-processing"}},
		{"root domain", SkillFilter{Domains: []string{"root"}}, []string{"git-workflow"}},
		{"allow glob", SkillFilter{Allow: []string{"*-policy", "git-*"}}, []string{"git-workflow", "refund-policy"}},
		{"deny wins", SkillFilter{Allow: []string{"*"}, Deny: []string{"pdf-*"}}, []string{"git-workflow", "refund-policy"}},
		{"deny only", SkillFilter{Deny: []string{"git-workflow"}}, []string{"pdf-processing", "refund-policy"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cat, err := BuildCatalog("p", "claude", testServed(), tt.filter)
			require.NoError(t, err)
			var got []string
			for _, s := range cat.Skills() {
				got = append(got, s.Name)
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestBuildCatalog_SkipsUnrepresentableSkills(t *testing.T) {
	t.Parallel()
	noDescription := generator.ServedSkill{ID: "x", Files: []generator.ServedSkillFile{{RelPath: "SKILL.md", Content: []byte("---\nname: x\n---\nbody")}}}
	noFrontmatter := generator.ServedSkill{ID: "y", Files: []generator.ServedSkillFile{{RelPath: "SKILL.md", Content: []byte("body only")}}}
	good := servedSkill("good", "", "A good skill", nil)
	cat, err := BuildCatalog("p", "claude", []generator.ServedSkill{noDescription, noFrontmatter, good}, SkillFilter{})
	require.NoError(t, err, "one bad skill must not stop the server")
	require.Len(t, cat.Skills(), 1)
	assert.Equal(t, "good", cat.Skills()[0].Name)
}

func TestBuildCatalog_DuplicateNamesAreAnError(t *testing.T) {
	t.Parallel()
	a := servedSkill("same", "", "one", nil)
	b := servedSkill("same", "docs", "two", nil)
	_, err := BuildCatalog("p", "claude", []generator.ServedSkill{a, b}, SkillFilter{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "same")
}

func TestCatalog_DigestsAreStableAndContentAddressed(t *testing.T) {
	t.Parallel()
	a, err := BuildCatalog("p", "claude", testServed(), SkillFilter{})
	require.NoError(t, err)
	b, err := BuildCatalog("p", "claude", testServed(), SkillFilter{})
	require.NoError(t, err)
	pa, _ := a.Lookup("pdf-processing")
	pb, _ := b.Lookup("pdf-processing")
	assert.Equal(t, pa.Digest, pb.Digest)
	assert.Regexp(t, `^sha256:[0-9a-f]{64}$`, pa.Files[0].Digest)

	changed := testServed()
	changed[1].Files[1].Content = []byte("different")
	c, err := BuildCatalog("p", "claude", changed, SkillFilter{})
	require.NoError(t, err)
	pc, _ := c.Lookup("pdf-processing")
	assert.NotEqual(t, pa.Digest, pc.Digest, "a changed supporting file must change the skill digest")
	gw, _ := a.Lookup("git-workflow")
	gc, _ := c.Lookup("git-workflow")
	assert.Equal(t, gw.Digest, gc.Digest)
}

func TestCatalog_Search(t *testing.T) {
	t.Parallel()
	cat, err := BuildCatalog("p", "claude", testServed(), SkillFilter{})
	require.NoError(t, err)
	tests := []struct {
		query string
		limit int
		want  []string
	}{
		{"", 0, []string{"git-workflow", "pdf-processing", "refund-policy"}},
		{"pdf", 0, []string{"pdf-processing"}},
		{"refunds", 0, []string{"refund-policy"}},
		{"git branch", 0, []string{"git-workflow"}},
		{"customer", 0, []string{"refund-policy"}},
		{"nothing-matches", 0, nil},
		{"", 2, []string{"git-workflow", "pdf-processing"}},
	}
	for _, tt := range tests {
		var got []string
		for _, h := range cat.Search(tt.query, tt.limit) {
			got = append(got, h.Skill.Name)
		}
		assert.Equal(t, tt.want, got, "query %q", tt.query)
	}
	hits := cat.Search("pdf", 0)
	require.Len(t, hits, 1)
	assert.Positive(t, hits[0].Score)
}

// rpcPeer drives the skills server over raw newline-delimited JSON-RPC, since
// the SDK client cannot issue the extension's custom methods.
type rpcPeer struct {
	t      *testing.T
	in     io.Writer
	out    *bufio.Scanner
	nextID int
}

func startSkillServer(t *testing.T, cat *Catalog) *rpcPeer {
	t.Helper()
	srv := NewSkillServer("test", cat)
	clientToServer, serverIn := io.Pipe()
	serverOut, clientFromServer := io.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = srv.GetMCPServer().Run(ctx, srv.WrapTransport(&sdkmcp.IOTransport{Reader: clientToServer, Writer: clientFromServer}))
	}()
	t.Cleanup(func() {
		cancel()
		_ = serverIn.Close()
		_ = clientFromServer.Close()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
		}
	})
	scanner := bufio.NewScanner(serverOut)
	scanner.Buffer(make([]byte, 1<<20), 1<<24)
	p := &rpcPeer{t: t, in: serverIn, out: scanner}
	init := p.call("initialize", map[string]any{
		"protocolVersion": "2025-06-18", "capabilities": map[string]any{},
		"clientInfo": map[string]any{"name": "t", "version": "1"},
	})
	require.Nil(t, init["error"], "initialize failed: %v", init)
	p.notify("notifications/initialized")
	return p
}

func (p *rpcPeer) notify(method string) {
	_, err := fmt.Fprintf(p.in, `{"jsonrpc":"2.0","method":%q}`+"\n", method)
	require.NoError(p.t, err)
}

func (p *rpcPeer) call(method string, params any) map[string]any {
	p.t.Helper()
	p.nextID++
	raw, err := json.Marshal(params)
	require.NoError(p.t, err)
	_, err = fmt.Fprintf(p.in, `{"jsonrpc":"2.0","id":%d,"method":%q,"params":%s}`+"\n", p.nextID, method, raw)
	require.NoError(p.t, err)
	for p.out.Scan() {
		var msg map[string]any
		require.NoError(p.t, json.Unmarshal(p.out.Bytes(), &msg))
		if id, ok := msg["id"].(float64); ok && int(id) == p.nextID {
			return msg
		}
	}
	p.t.Fatalf("no response to %s: %v", method, p.out.Err())
	return nil
}

func TestSkillServer_ExtensionMethods(t *testing.T) {
	cat, err := BuildCatalog("p", "claude", testServed(), SkillFilter{})
	require.NoError(t, err)
	p := startSkillServer(t, cat)

	t.Run("skills/list", func(t *testing.T) {
		resp := p.call("skills/list", map[string]any{})
		require.Nil(t, resp["error"])
		result := resp["result"].(map[string]any)
		assert.Equal(t, "complete", result["resultType"])
		skills := result["skills"].([]any)
		require.Len(t, skills, 3)
		pdf := skills[1].(map[string]any)
		assert.Equal(t, "skill://pdf-processing/SKILL.md", pdf["uri"])
		assert.Equal(t, "pdf-processing", pdf["frontmatter"].(map[string]any)["name"])
		res := pdf["resources"].([]any)
		require.Len(t, res, 3)
		first := res[0].(map[string]any)
		assert.Equal(t, "skill://pdf-processing/SKILL.md", first["uri"])
		assert.Regexp(t, `^sha256:[0-9a-f]{64}$`, first["digest"])
		assert.InDelta(t, float64(len(cat.skills[1].Files[0].Content)), first["size"], 0)
	})

	t.Run("skills/get", func(t *testing.T) {
		resp := p.call("skills/get", map[string]any{"uri": "skill://git-workflow/SKILL.md"})
		require.Nil(t, resp["error"])
		skill := resp["result"].(map[string]any)["skill"].(map[string]any)
		assert.Equal(t, "skill://git-workflow/SKILL.md", skill["uri"])
	})

	t.Run("skills/get unknown is invalid params", func(t *testing.T) {
		resp := p.call("skills/get", map[string]any{"uri": "skill://nope/SKILL.md"})
		require.NotNil(t, resp["error"])
		assert.InDelta(t, -32602, resp["error"].(map[string]any)["code"], 0)
	})

	t.Run("capability advertises the extension", func(t *testing.T) {
		resp := p.call("initialize", map[string]any{
			"protocolVersion": "2025-06-18", "capabilities": map[string]any{},
			"clientInfo": map[string]any{"name": "t", "version": "1"},
		})
		caps := resp["result"].(map[string]any)["capabilities"].(map[string]any)
		assert.Contains(t, caps["extensions"], SkillsExtensionID)
		assert.Contains(t, caps, "resources")
	})

	t.Run("resources/read matches the served bytes", func(t *testing.T) {
		resp := p.call("resources/read", map[string]any{"uri": "skill://pdf-processing/references/FORMS.md"})
		require.Nil(t, resp["error"])
		contents := resp["result"].(map[string]any)["contents"].([]any)[0].(map[string]any)
		assert.Equal(t, "forms", contents["text"])
		skillMD := p.call("resources/read", map[string]any{"uri": "skill://git-workflow/SKILL.md"})
		text := skillMD["result"].(map[string]any)["contents"].([]any)[0].(map[string]any)["text"]
		assert.Equal(t, string(cat.skills[0].Files[0].Content), text)
	})

	t.Run("resources/list lists every file", func(t *testing.T) {
		resp := p.call("resources/list", map[string]any{})
		assert.Len(t, resp["result"].(map[string]any)["resources"], 5)
	})

	t.Run("tools are read-only serving tools only", func(t *testing.T) {
		resp := p.call("tools/list", map[string]any{})
		var names []string
		for _, tool := range resp["result"].(map[string]any)["tools"].([]any) {
			m := tool.(map[string]any)
			names = append(names, m["name"].(string))
			ann := m["annotations"].(map[string]any)
			assert.Equal(t, true, ann["readOnlyHint"], m["name"])
		}
		assert.ElementsMatch(t, []string{"search_skills", "get_skill", "read_skill_file"}, names)
		for _, n := range names {
			assert.False(t, strings.HasPrefix(n, "create_") || strings.HasPrefix(n, "update_") || strings.HasPrefix(n, "delete_"))
		}
	})

	t.Run("search_skills ranks and respects the domain filter", func(t *testing.T) {
		resp := p.call("tools/call", map[string]any{"name": "search_skills", "arguments": map[string]any{"query": "pdf refunds", "domain": "billing"}})
		text := resp["result"].(map[string]any)["content"].([]any)[0].(map[string]any)["text"].(string)
		var out struct {
			Count   int `json:"count"`
			Results []struct {
				Name   string `json:"name"`
				Digest string `json:"digest"`
				Source string `json:"source"`
			} `json:"results"`
		}
		require.NoError(t, json.Unmarshal([]byte(text), &out))
		require.Equal(t, 1, out.Count)
		assert.Equal(t, "refund-policy", out.Results[0].Name)
		assert.NotEmpty(t, out.Results[0].Digest)
		assert.NotEmpty(t, out.Results[0].Source)
	})

	t.Run("authoring tools do not exist", func(t *testing.T) {
		resp := p.call("tools/call", map[string]any{"name": "create_skill", "arguments": map[string]any{"name": "x"}})
		assert.NotNil(t, resp["error"])
	})
}

func TestNewServer_AuthoringToolsUnchanged(t *testing.T) {
	t.Parallel()
	srv := NewServer("test")
	assert.Nil(t, srv.Catalog())
	// The default server must not claim the Skills extension.
	peer := startAuthoringPeer(t, srv)
	assert.NotContains(t, peer, SkillsExtensionID)
}

func startAuthoringPeer(t *testing.T, srv *Server) string {
	t.Helper()
	ctxt, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	serverT, clientT := sdkmcp.NewInMemoryTransports()
	go func() { _ = srv.GetMCPServer().Run(ctxt, serverT) }()
	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "c", Version: "1"}, nil)
	session, err := client.Connect(ctxt, clientT, nil)
	require.NoError(t, err)
	defer session.Close()
	tools, err := session.ListTools(ctxt, nil)
	require.NoError(t, err)
	var names []string
	for _, tool := range tools.Tools {
		names = append(names, tool.Name)
	}
	require.Contains(t, names, "create_skill")
	require.Contains(t, names, "generate_outputs")
	init := session.InitializeResult()
	raw, err := json.Marshal(init.Capabilities)
	require.NoError(t, err)
	return string(raw)
}
