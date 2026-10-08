package emit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const skillMD = "---\nname: deploy\ndescription: Deploy the service safely.\n---\n\n# Deploy\n\nRun the checks first.\n"

func fixture() Input {
	return Input{
		Version: "1.4.0", Repo: "acme/skills", Commit: "0f3e0f3e0f3e0f3e0f3e0f3e0f3e0f3e0f3e0f3e", Tag: "v1.4.0",
		LockTree: "sha256:6cd1000000000000000000000000000000000000000000000000000000000000",
		Market:   Market{Name: "acme-skills", Description: "Acme conventions", OwnerName: "Acme", OwnerEmail: "plugins@acme.test"},
		Plugins: []Plugin{{
			Name: "acme-conventions", Description: "Conventions", Version: "1.4.0", Category: "development",
			Keywords: []string{"conventions"}, Runtimes: []string{"claude", "cursor"},
			BundleFile: "acme-conventions-1.4.0.tar.gz", BundleDigest: "sha256:" + strings.Repeat("a", 64),
			Files: []File{
				{Path: ".claude-plugin/plugin.json", Data: []byte("{}\n")},
				{Path: ".cursor-plugin/plugin.json", Data: []byte("{\"name\":\"acme-conventions\"}\n")},
				{Path: "skills/deploy/SKILL.md", Data: []byte(skillMD)},
			},
		}},
		Rules: []Doc{
			{Name: "always-tests", Mode: ModeAlways, Body: "# Tests\n\nWrite tests.\n"},
			{Name: "go-style", Mode: ModeGlob, Globs: []string{"**/*.go"}, Body: "---\npriority: high\n---\n# Go\n\nGofmt.\n"},
			{Name: "review", Mode: ModeAuto, Description: "Review checklist", Body: "# Review\n"},
		},
	}
}

// fixtureFor is the shared fixture, plus the Agent Plugins package the
// agent-plugins emitter needs (the other emitters would copy it into their
// output).
func fixtureFor(name string) Input {
	in := fixture()
	switch name {
	case "ard":
		in.ARD = ardFixture()
		in.Plugins[0].Files = append(in.Plugins[0].Files, agentPluginFixtureFiles()...)
	case "agent-plugins":
		in.Plugins[0].Files = append(in.Plugins[0].Files, agentPluginFixtureFiles()...)
	}
	return in
}

func readTree(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, rerr := os.ReadFile(p) //nolint:gosec // test fixture
		if rerr != nil {
			return rerr
		}
		rel, _ := filepath.Rel(root, p)
		out[filepath.ToSlash(rel)] = string(data)
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		require.NoError(t, err)
	}
	return out
}

func TestEmitters_MatchTheGoldenFiles(t *testing.T) {
	for _, name := range Names() {
		t.Run(name, func(t *testing.T) {
			e, _ := Lookup(name)

			files, _, err := e.Emit(fixtureFor(name))

			require.NoError(t, err)
			got := map[string]string{}
			for _, f := range files {
				got[f.Path] = string(f.Data)
			}
			dir := filepath.Join("testdata", "golden", name)
			if os.Getenv("UPDATE_GOLDEN") == "1" {
				require.NoError(t, os.RemoveAll(dir))
				for p, data := range got {
					require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(dir, p)), 0o750))
					require.NoError(t, os.WriteFile(filepath.Join(dir, p), []byte(data), 0o600))
				}
			}
			assert.Equal(t, readTree(t, dir), got, "run UPDATE_GOLDEN=1 go test ./internal/publish/emit and review the diff")
		})
	}
}

func TestEmitters_AreDeterministicAndSorted(t *testing.T) {
	for _, name := range Names() {
		t.Run(name, func(t *testing.T) {
			e, _ := Lookup(name)

			a, _, err := e.Emit(fixtureFor(name))
			require.NoError(t, err)
			b, _, err := e.Emit(fixtureFor(name))
			require.NoError(t, err)

			assert.Equal(t, a, b)
			paths := make([]string, len(a))
			for i, f := range a {
				paths[i] = f.Path
				assert.True(t, validRel(f.Path), f.Path)
			}
			assert.True(t, sort.StringsAreSorted(paths))
		})
	}
}

func TestStatuses(t *testing.T) {
	want := map[string]string{
		"cursor-team-marketplace": StatusVerified, "agent-plugins": StatusVerified, "ard": StatusVerified, "port": StatusExperimental,
		"aws-agent-registry": StatusExperimental, "kiro-steering": StatusExperimental,
	}
	for name, status := range want {
		e, ok := Lookup(name)
		require.True(t, ok, name)
		assert.Equal(t, status, e.Status(), name)
	}
	assert.Len(t, Names(), len(want))
}

// TestCursor_MatchesTheDocumentedManifest checks the index against the schema
// https://cursor.com/docs/reference/plugins documents: name (kebab-case),
// owner.name, plugins[].name and a relative source, at most 10 MB.
func TestCursor_MatchesTheDocumentedManifest(t *testing.T) {
	files, _, err := cursorTeam{}.Emit(fixture())
	require.NoError(t, err)
	var index []byte
	for _, f := range files {
		if f.Path == ".cursor-plugin/marketplace.json" {
			index = f.Data
		}
	}
	require.NotNil(t, index)

	var doc struct {
		Name  string `json:"name"`
		Owner struct {
			Name  string `json:"name"`
			Email string `json:"email"`
		} `json:"owner"`
		Metadata map[string]string `json:"metadata"`
		Plugins  []struct {
			Name   string `json:"name"`
			Source string `json:"source"`
		} `json:"plugins"`
	}
	require.NoError(t, json.Unmarshal(index, &doc))
	assert.Regexp(t, `^[a-z0-9][a-z0-9.-]*$`, doc.Name)
	assert.NotEmpty(t, doc.Owner.Name)
	assert.Less(t, len(index), 10<<20)
	require.Len(t, doc.Plugins, 1)
	assert.Equal(t, "plugins/acme-conventions", doc.Plugins[0].Source)
	for _, f := range files {
		if strings.HasPrefix(f.Path, doc.Plugins[0].Source+"/") {
			return
		}
	}
	t.Fatal("the plugin directory the source names is not emitted")
}

func TestCursor_Errors(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Input)
		want   string
	}{
		{"no plugin", func(in *Input) { in.Plugins = nil }, "no plugin"},
		{"upper-case marketplace", func(in *Input) { in.Market.Name = "Acme" }, "not kebab-case"},
		{"upper-case plugin", func(in *Input) { in.Plugins[0].Name = "Acme_Conventions" }, "not kebab-case"},
		{"no cursor manifest", func(in *Input) { in.Plugins[0].Files = in.Plugins[0].Files[:1] }, ".cursor-plugin/plugin.json"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := fixture()
			tt.mutate(&in)

			_, _, err := cursorTeam{}.Emit(in)

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestCursor_WithoutAnOwnerFallsBackToTheMarketplaceName(t *testing.T) {
	in := fixture()
	in.Market.OwnerName, in.Market.OwnerEmail = "", ""

	files, findings, err := cursorTeam{}.Emit(in)

	require.NoError(t, err)
	require.Len(t, findings, 1)
	for _, f := range files {
		if f.Path == ".cursor-plugin/marketplace.json" {
			assert.Contains(t, string(f.Data), `"name": "acme-skills"`)
			assert.NotContains(t, string(f.Data), "plugins@acme.test")
		}
	}
}

// TestAWS_RecordsMatchTheDocumentedConstraints applies the CreateRegistryRecord
// request constraints to every emitted record.
func TestAWS_RecordsMatchTheDocumentedConstraints(t *testing.T) {
	files, _, err := awsRegistry{}.Emit(fixture())
	require.NoError(t, err)

	records := 0
	for _, f := range files {
		if !strings.HasPrefix(f.Path, "records/") {
			continue
		}
		records++
		var rec awsRecord
		require.NoError(t, json.Unmarshal(f.Data, &rec))
		require.NoError(t, checkAWSRecord(rec), f.Path)
		assert.Contains(t, []string{"SKILL", "CUSTOM"}, rec.RecordType)
		if rec.RecordType == "SKILL" {
			def := rec.Descriptors.AgentSkillsDefinition
			require.NotNil(t, def)
			assert.Equal(t, "0.1.0", def.DataSchemaVersion)
			assert.Equal(t, skillMD, def.AdditionalData.SkillMd.Data)
			assert.True(t, json.Valid([]byte(def.Data)), "data is a JSON document in a string")
			assert.Nil(t, rec.Descriptors.Custom, "exactly one primary descriptor")
		}
	}
	assert.Equal(t, 2, records)
}

func TestAWS_RejectsValuesTheServiceRefuses(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Input)
		want   string
	}{
		{"version with a plus", func(in *Input) { in.Plugins[0].Version = "1.0.0+build" }, "valid record version"},
		{"name with a space", func(in *Input) { in.Plugins[0].Name = "my plugin" }, "valid record name"},
		{"long description", func(in *Input) { in.Plugins[0].Description = strings.Repeat("x", 5000) }, "exceeds 4096"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := fixture()
			tt.mutate(&in)

			_, _, err := awsRegistry{}.Emit(in)

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestPort_EntitiesCarryTheDocumentedFields(t *testing.T) {
	in := fixture()
	in.Options = map[string]string{"blueprint": "ai_skill"}

	files, findings, err := port{}.Emit(in)

	require.NoError(t, err)
	assert.NotEmpty(t, findings)
	identifier := regexp.MustCompile(`^[A-Za-z0-9_.@+:=/-]{1,100}$`)
	entities := 0
	for _, f := range files {
		if !strings.HasPrefix(f.Path, "entities/") {
			continue
		}
		entities++
		var e map[string]any
		require.NoError(t, json.Unmarshal(f.Data, &e))
		assert.Regexp(t, identifier, e["identifier"])
		assert.NotEmpty(t, e["title"])
		assert.Contains(t, e, "properties")
		assert.Contains(t, e, "relations")
	}
	assert.Equal(t, 2, entities)
	for _, f := range files {
		if f.Path == "index.json" {
			assert.Contains(t, string(f.Data), "/v1/blueprints/ai_skill/entities?upsert=true")
		}
	}
}

func TestPort_RejectsABadBlueprint(t *testing.T) {
	in := fixture()
	in.Options = map[string]string{"blueprint": "../x"}

	_, _, err := port{}.Emit(in)

	require.Error(t, err)
}

func TestPortIdentifier(t *testing.T) {
	tests := []struct{ in, want string }{
		{"acme-conventions", "acme-conventions"},
		{"my skill!", "my-skill"},
		{strings.Repeat("a", 120), strings.Repeat("a", 100)},
		{"???", ""},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, portIdentifier(tt.in), tt.in)
	}
}

func TestKiro_FrontMatterComesFirstAndModesMap(t *testing.T) {
	files, _, err := kiroSteering{}.Emit(fixture())

	require.NoError(t, err)
	got := map[string]string{}
	for _, f := range files {
		got[f.Path] = string(f.Data)
	}
	for p, body := range got {
		if strings.HasSuffix(p, ".md") {
			assert.True(t, strings.HasPrefix(body, "---\n"), p)
		}
	}
	assert.Contains(t, got[".kiro/steering/always-tests.md"], "inclusion: always")
	assert.Contains(t, got[".kiro/steering/go-style.md"], "inclusion: fileMatch\nfileMatchPattern: '**/*.go'")
	assert.NotContains(t, got[".kiro/steering/go-style.md"], "priority: high", "source front matter is replaced")
	assert.Contains(t, got[".kiro/steering/review.md"], "inclusion: auto")
	assert.Contains(t, got[".kiro/steering/skill-deploy.md"], "description: Deploy the service safely.")

	var dist kiroDistribution
	require.NoError(t, json.Unmarshal([]byte(got["distribution.json"]), &dist))
	assert.Equal(t, ".kiro/steering", dist.Target)
	assert.Len(t, dist.Files, 4)
}

func TestKiro_NothingToEmit(t *testing.T) {
	in := fixture()
	in.Rules = nil
	in.Plugins[0].Files = nil

	_, _, err := kiroSteering{}.Emit(in)

	require.Error(t, err)
}

func TestSkills_ReadsTheFrontMatterAndFallsBackToTheDirectory(t *testing.T) {
	files := []File{
		{Path: "skills/deploy/SKILL.md", Data: []byte(skillMD)},
		{Path: "skills/plain/SKILL.md", Data: []byte("# no front matter\n")},
		{Path: "skills/deploy/references/a.md", Data: []byte("x")},
		{Path: "skills/a/b/SKILL.md", Data: []byte("nested")},
	}

	got := Skills(files)

	require.Len(t, got, 2)
	assert.Equal(t, "deploy", got[0].Name)
	assert.Equal(t, "Deploy the service safely.", got[0].Description)
	assert.Equal(t, "plain", got[1].Name)
}

func TestValidRel(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"a.md", true}, {"skills/a/SKILL.md", true}, {".claude-plugin/plugin.json", true},
		{"", false}, {"/etc/passwd", false}, {"..", false}, {"../a", false}, {"a/../b", false},
		{"a/./b", false}, {"a//b", false}, {"a/", false}, {`a\b`, false}, {"..\\a", false},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			// Act
			got := validRel(tt.path)

			// Assert
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestFinishRefusesUnsafeAndDuplicatePaths(t *testing.T) {
	tests := []struct {
		name  string
		files []File
		want  string
	}{
		{"escape", []File{{Path: "../x"}}, "unsafe path"},
		{"absolute", []File{{Path: "/x"}}, "unsafe path"},
		{"duplicate", []File{{Path: "a"}, {Path: "a"}}, "twice"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			_, err := finish(tt.files)

			// Assert
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}
