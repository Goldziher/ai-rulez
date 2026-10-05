package commands

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/kaptinlin/jsonschema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/generator"
	"github.com/Goldziher/ai-rulez/v5/internal/progress"
	"github.com/Goldziher/ai-rulez/v5/internal/roles"
	"github.com/Goldziher/ai-rulez/v5/internal/tokens"
)

const rolesTestConfig = lockProjectConfig + `
[[roles]]
name = "base"
description = "Everyone"
domains = ["backend"]
[roles.skills]
exclude = ["deploy"]

[[roles]]
name = "dev"
extends = "base"
domains = ["frontend"]
[roles.skill_mode]
migrate = "name-only"
[roles.match]
groups = ["okta:dev"]
`

func rolesCmdProject(t *testing.T) string {
	t.Helper()
	root := lockProject(t, "")
	writeFile(t, filepath.Join(root, ".ai-rulez", "config.toml"), rolesTestConfig)
	for _, p := range []string{"domains/backend/skills/migrate", "domains/backend/skills/deploy", "domains/frontend/skills/ui"} {
		id := filepath.Base(p)
		writeFile(t, filepath.Join(root, ".ai-rulez", p, "SKILL.md"),
			"---\nname: "+id+"\ndescription: Use when you need "+id+".\n---\nBody of "+id+".\n")
	}
	t.Cleanup(func() { rolesFormat, tokensRole, tokensByRole, generateRole, profile = "", "", false, "", "" })
	return root
}

func validateAgainst(t *testing.T, schemaPath string, doc []byte) {
	t.Helper()
	_, thisFile, _, _ := runtime.Caller(0) //nolint:dogsled // only the file is needed
	schemaBytes, err := os.ReadFile(filepath.Join(filepath.Dir(thisFile), filepath.FromSlash(schemaPath)))
	require.NoError(t, err)
	compiled, err := jsonschema.NewCompiler().Compile(schemaBytes)
	require.NoError(t, err)
	var v any
	require.NoError(t, json.NewDecoder(bytes.NewReader(doc)).Decode(&v))
	res := compiled.Validate(v)
	assert.True(t, res.IsValid(), "document violates %s: %v", schemaPath, res.Errors)
}

func TestRolesListShowResolve(t *testing.T) {
	rolesCmdProject(t)

	var out bytes.Buffer
	require.NoError(t, runRolesList(&out))
	assert.Contains(t, out.String(), "dev")
	assert.Contains(t, out.String(), "Everyone")

	rolesFormat = formatJSON
	out.Reset()
	require.NoError(t, runRolesList(&out))
	validateAgainst(t, "../../schema/roles-manifest.schema.json", out.Bytes())
	var manifest roles.Manifest
	require.NoError(t, json.Unmarshal(out.Bytes(), &manifest))
	require.Len(t, manifest.Roles, 2)
	dev := manifest.Roles[1]
	assert.Equal(t, "dev", dev.Name)
	assert.Equal(t, []string{"backend", "frontend"}, dev.Domains)
	assert.Equal(t, map[string]string{"migrate": "name-only"}, dev.SkillModes)
	assert.Equal(t, []string{"okta:dev"}, dev.Match.Groups)

	out.Reset()
	require.NoError(t, runRolesShow(&out, "dev"))
	var shown roleShow
	require.NoError(t, json.Unmarshal(out.Bytes(), &shown))
	assert.Equal(t, []string{"dev", "base"}, shown.Chain)
	assert.Equal(t, []string{"frontend"}, shown.Declared.Domains)
	assert.Equal(t, []string{"backend", "frontend"}, shown.Effective.Domains)
	assert.Equal(t, []string{"okta:dev"}, shown.Effective.Match.Groups, "a role keeps its own match hints")

	out.Reset()
	require.NoError(t, runRolesResolve(&out, "base"))
	var resolved struct {
		Role roles.Role `json:"role"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &resolved))
	var ids []string
	for _, it := range resolved.Role.Items {
		ids = append(ids, it.ID)
	}
	assert.NotContains(t, ids, "deploy")
	assert.Contains(t, ids, "migrate")

	assert.Error(t, runRolesShow(&out, "ghost"))
	assert.Error(t, runRolesResolve(&out, "ghost"))
}

func TestTokensByRole(t *testing.T) {
	rolesCmdProject(t)
	progress.SetQuiet(true)

	tokensByRole, tokensJSON = true, true
	defer func() { tokensJSON = false }()
	var out bytes.Buffer
	_, err := runTokens(&out, nil)
	require.NoError(t, err)
	var reports []struct {
		Profile string `json:"profile"`
		Role    string `json:"role"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &reports))
	require.Len(t, reports, 2)
	assert.Equal(t, "role:base", reports[0].Profile)
	assert.Equal(t, "dev", reports[1].Role)

	tokensByRole, tokensRole = false, "base"
	out.Reset()
	_, err = runTokens(&out, nil)
	require.NoError(t, err)
	assert.Contains(t, out.String(), `"role": "base"`)

	profile = "x"
	_, err = runTokens(&out, nil)
	require.Error(t, err, "--role cannot be combined with --profile")
}

func TestGenerateRoleFlagsAreMutuallyExclusive(t *testing.T) {
	rolesCmdProject(t)
	generateRole, profile = "dev", "backend"
	assert.Error(t, checkRoleFlags())
	profile = ""
	assert.NoError(t, checkRoleFlags())
}

func TestCatalog(t *testing.T) {
	root := rolesCmdProject(t)
	require.Equal(t, 0, writeLockAt("", "", nil))

	cfg, err := loadConfigForCommand(t.Context(), nil)
	require.NoError(t, err)
	counter := mustCounter(t)
	doc, err := buildCatalog(cfg, counter)
	require.NoError(t, err)
	assert.Equal(t, catalogSchemaVersion, doc.SchemaVersion)
	assert.True(t, doc.Lock.Present)
	require.NotNil(t, doc.Lock.SourcesInSync)
	assert.True(t, *doc.Lock.SourcesInSync)
	var migrate *catalogItem
	for i := range doc.Items {
		if doc.Items[i].ID == "migrate" {
			migrate = &doc.Items[i]
		}
	}
	require.NotNil(t, migrate)
	assert.ElementsMatch(t, []string{"base", "dev"}, migrate.Roles)
	assert.Contains(t, migrate.Digest, "sha256:")

	writeFile(t, filepath.Join(root, ".ai-rulez", "rules", "style.md"), "# Style\nchanged\n")
	cfg, err = loadConfigForCommand(t.Context(), nil)
	require.NoError(t, err)
	doc, err = buildCatalog(cfg, counter)
	require.NoError(t, err)
	assert.False(t, *doc.Lock.SourcesInSync)
	assert.Equal(t, 1, doc.Lock.Changed)

	var out bytes.Buffer
	catalogFormat = formatJSON
	defer func() { catalogFormat = "" }()
	require.NoError(t, runCatalog(&out))
	validateAgainst(t, "../../schema/catalog.schema.json", out.Bytes())
}

func mustCounter(t *testing.T) tokens.Counter {
	t.Helper()
	c, err := tokens.New("")
	require.NoError(t, err)
	return c
}

// generate --check must agree with generate: right after `generate --role dev`
// the role check is clean, and checking without the role sees that a generate
// would take the role's skillOverrides back out of .claude/settings.json.
func TestGenerateCheckRoleSkillModeRoundTrip(t *testing.T) {
	root := rolesCmdProject(t)
	cfg, err := loadForLock("")
	require.NoError(t, err)

	gen := generator.NewGenerator(cfg)
	require.NoError(t, gen.SetRole("dev"))
	require.NoError(t, gen.Generate(""))
	settings, err := os.ReadFile(filepath.Join(root, ".claude", "settings.json"))
	require.NoError(t, err)
	require.Contains(t, string(settings), "skillOverrides")

	cfg, err = loadForLock("")
	require.NoError(t, err)
	gen = generator.NewGenerator(cfg)
	require.NoError(t, gen.SetRole("dev"))
	drift, err := gen.CheckDrift("")
	require.NoError(t, err)
	assert.Empty(t, drift, "a role check right after generating that role is clean")

	cfg, err = loadForLock("")
	require.NoError(t, err)
	drift, err = generator.NewGenerator(cfg).CheckDrift("")
	require.NoError(t, err)
	var paths []string
	for _, d := range drift {
		paths = append(paths, d.Path)
	}
	assert.Contains(t, paths, ".claude/settings.json", "generating without the role would take its skillOverrides back")
}
