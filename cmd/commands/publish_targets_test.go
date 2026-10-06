package commands

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator"
	"github.com/Goldziher/ai-rulez/v5/internal/publish"
	"github.com/Goldziher/ai-rulez/v5/internal/runner"
	"github.com/Goldziher/ai-rulez/v5/internal/signing"
	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

// reconfigure replaces the project's configuration, regenerates the plugin
// bundle, relocks and commits, so the next publish starts from a clean tree.
func reconfigure(t *testing.T, root, configToml string) {
	t.Helper()
	writeFile(t, filepath.Join(root, ".ai-rulez", "config.toml"), configToml)
	cfg, err := config.LoadConfig(context.Background(), ".", config.WithoutLocal())
	require.NoError(t, err)
	require.NoError(t, generator.NewGenerator(cfg).GeneratePlugin(""))
	require.Equal(t, 0, writeLockAt("", "", nil), "lock")
	publishGit(t, root, "add", "-A")
	publishGit(t, root, "commit", "-q", "-m", "reconfigure")
}

func manifestOf(t *testing.T, dist map[string]string) publish.Manifest {
	t.Helper()
	var m publish.Manifest
	require.NoError(t, json.Unmarshal([]byte(dist["acme-1.4.0.manifest.json"]), &m))
	return m
}

func archiveNames(t *testing.T, dist map[string]string) []string {
	t.Helper()
	var names []string
	for name := range archiveModes(t, dist) {
		names = append(names, name)
	}
	return names
}

func TestPublish_RuntimeFilterShipsOnlyTheRequestedRuntimes(t *testing.T) {
	root := publishProject(t)
	reconfigure(t, root, strings.Replace(publishProjectConfig, `runtimes = ["claude"]`, `runtimes = ["claude", "cursor"]`, 1))
	run := func(runtimes ...string) map[string]string {
		publishRuntimes = runtimes
		publishDist = filepath.Join(t.TempDir(), "dist")
		_, err := runPublishCapture(t)
		require.NoError(t, err)
		return readDist(t, publishDist)
	}

	all := run()
	only := run("claude")

	assert.Equal(t, []string{"claude", "cursor"}, manifestOf(t, all).Runtimes)
	assert.Contains(t, archiveNames(t, all), ".cursor-plugin/plugin.json")
	assert.Equal(t, []string{"claude"}, manifestOf(t, only).Runtimes)
	assert.NotContains(t, archiveNames(t, only), ".cursor-plugin/plugin.json")
	assert.Contains(t, archiveNames(t, only), ".claude-plugin/plugin.json")
	assert.NotEqual(t, manifestOf(t, all).Bundle.Digest, manifestOf(t, only).Bundle.Digest)
}

func TestPublish_RuntimeFilterErrors(t *testing.T) {
	publishProject(t)
	tests := []struct {
		name     string
		runtimes []string
		want     string
	}{
		{"unknown runtime", []string{"vim"}, "unknown runtime"},
		{"not configured", []string{"codex"}, "not one of the [plugin] runtimes"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			publishRuntimes = tt.runtimes

			_, err := runPublishCapture(t)

			requirePublishError(t, err, publish.CodeConfig, publish.ExitFailed)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestPublish_RuntimesFromTheConfigTable(t *testing.T) {
	root := publishProject(t)
	reconfigure(t, root, strings.Replace(publishProjectConfig, `runtimes = ["claude"]`, `runtimes = ["claude", "cursor"]`, 1)+"\n[publish]\nruntimes = [\"cursor\"]\n")

	_, err := runPublishCapture(t)

	require.NoError(t, err)
	assert.Equal(t, []string{"cursor"}, manifestOf(t, readDist(t, filepath.Join(root, "dist"))).Runtimes)
}

func TestPublish_PinnedMarketplacePerChannel(t *testing.T) {
	root := publishProject(t)
	reconfigure(t, root, publishProjectConfig+"\n[publish.marketplace.channels]\ncanary = \"main\"\n")
	run := func(channel string) map[string]string {
		publishMarketplace, publishChannel = true, channel
		publishDist = filepath.Join(t.TempDir(), "dist")
		_, err := runPublishCapture(t)
		require.NoError(t, err)
		return readDist(t, publishDist)
	}

	stable := run("stable")
	canary := run("canary")

	commit := manifestOf(t, stable).Source.Commit
	var doc struct {
		Plugins []struct {
			Source map[string]string `json:"source"`
		} `json:"plugins"`
	}
	require.NoError(t, json.Unmarshal([]byte(stable["marketplace/stable/.claude-plugin/marketplace.json"]), &doc))
	require.Len(t, doc.Plugins, 1)
	assert.Equal(t, map[string]string{"source": "github", "repo": "acme/skills", "ref": "v1.4.0", "sha": commit}, doc.Plugins[0].Source)
	require.NoError(t, json.Unmarshal([]byte(canary["marketplace/canary/.claude-plugin/marketplace.json"]), &doc))
	assert.Equal(t, map[string]string{"source": "github", "repo": "acme/skills", "ref": "main", "sha": commit}, doc.Plugins[0].Source)
}

func TestPublish_PinnedMarketplaceNeedsTheClaudeRuntime(t *testing.T) {
	root := publishProject(t)
	reconfigure(t, root, strings.Replace(publishProjectConfig, `runtimes = ["claude"]`, `runtimes = ["cursor"]`, 1))
	publishMarketplace = true

	_, err := runPublishCapture(t)

	requirePublishError(t, err, publish.CodeConfig, publish.ExitFailed)
	assert.Contains(t, err.Error(), "Claude marketplace index")
}

func TestPublish_EmittersAndTheExperimentalGate(t *testing.T) {
	root := publishProject(t)
	reconfigure(t, root, strings.Replace(publishProjectConfig, `runtimes = ["claude"]`, `runtimes = ["claude", "cursor"]`, 1))

	publishEmit = []string{"cursor-team-marketplace"}
	_, err := runPublishCapture(t)
	require.NoError(t, err)
	dist := readDist(t, filepath.Join(root, "dist"))
	assert.Contains(t, dist, "emit/cursor-team-marketplace/.cursor-plugin/marketplace.json")
	assert.Contains(t, dist, "emit/cursor-team-marketplace/plugins/acme/.cursor-plugin/plugin.json")

	publishEmit = []string{"port"}
	_, err = runPublishCapture(t)
	requirePublishError(t, err, publish.CodeConfig, publish.ExitFailed)

	publishExperimental = true
	_, err = runPublishCapture(t)
	require.NoError(t, err)
	assert.Contains(t, readDist(t, filepath.Join(root, "dist")), "emit/port/index.json")
}

func TestPublish_EmitSubcommandWritesOnlyTheEmitterFiles(t *testing.T) {
	root := publishProject(t)
	publishEmit, publishExperimental = []string{"kiro-steering"}, true
	publishEmitOut = filepath.Join(t.TempDir(), "kiro")
	var out strings.Builder

	var err error
	_, _ = capture(t, func() { err = runPublishEmit(context.Background(), &out, "kiro-steering") })

	require.NoError(t, err)
	files := readDist(t, publishEmitOut)
	assert.Contains(t, files, ".kiro/steering/care.md")
	assert.Contains(t, files, ".kiro/steering/skill-deploy.md")
	assert.Contains(t, files, "distribution.json")
	assert.NoDirExists(t, filepath.Join(root, "dist"), "no release directory is written")
}

func TestPublish_EmittersFromTheConfigTable(t *testing.T) {
	root := publishProject(t)
	writeFile(t, filepath.Join(root, "tools", "entity.json.tmpl"), `{"name": {{json .Name}}}`)
	reconfigure(t, root, publishProjectConfig+`
[[publish.emitters]]
name = "port"
options = { blueprint = "skill" }

[[publish.emitters]]
name = "template"
template = "tools/entity.json.tmpl"
output = "entity.json"
`)
	publishExperimental = true

	_, err := runPublishCapture(t)

	require.NoError(t, err)
	dist := readDist(t, filepath.Join(root, "dist"))
	assert.Contains(t, dist["emit/port/index.json"], "/v1/blueprints/skill/entities")
	assert.JSONEq(t, `{"name":"acme"}`, dist["emit/entity.json"])
}

func TestPublish_TemplateEmitterRefusesAPathOutsideTheProject(t *testing.T) {
	root := publishProject(t)
	outside := filepath.Join(t.TempDir(), "x.tmpl")
	writeFile(t, outside, "x")
	testutil.SymlinkOrSkip(t, outside, filepath.Join(root, "tools-link.tmpl"))
	reconfigure(t, root, publishProjectConfig+"\n[[publish.emitters]]\nname = \"template\"\ntemplate = \"tools-link.tmpl\"\n")

	_, err := runPublishCapture(t)

	requirePublishError(t, err, publish.CodeBundleUnsafe, publish.ExitFailed)
}

func TestPublish_ReleaseNotesDiffAgainstThePreviousTag(t *testing.T) {
	root := publishProject(t)
	publishGit(t, root, "tag", "v1.3.0")
	writeFile(t, filepath.Join(root, ".ai-rulez", "skills", "deploy", "SKILL.md"),
		"---\nname: deploy\ndescription: Use when deploying the service to production; not for local runs.\n---\n\n# Deploy\n\nRun the pipeline twice.\n")
	reconfigure(t, root, publishProjectConfig)

	_, err := runPublishCapture(t)

	require.NoError(t, err)
	notes := readDist(t, filepath.Join(root, "dist"))["RELEASE_NOTES.md"]
	assert.Contains(t, notes, "## Changes since v1.3.0")
	assert.Contains(t, notes, "### Changed")
	assert.Contains(t, notes, "- skill `deploy`")
}

func TestPublish_SinceNeedsATagWithALock(t *testing.T) {
	publishProject(t)
	publishSince = "v0.0.1"

	_, err := runPublishCapture(t)

	requirePublishError(t, err, publish.CodeConfig, publish.ExitFailed)
}

func TestPublish_SBOMIsShippedAndVerified(t *testing.T) {
	root := publishProject(t)
	publishWithSBOM = true

	_, err := runPublishCapture(t)

	require.NoError(t, err)
	dist := readDist(t, filepath.Join(root, "dist"))
	m := manifestOf(t, dist)
	require.NotNil(t, m.SBOM)
	assert.Equal(t, publish.Digest([]byte(dist[m.SBOM.File])), m.SBOM.Digest)
	assert.Contains(t, dist[m.SBOM.File], `"bomFormat": "CycloneDX"`)
	res, verr := publish.Verify(filepath.Join(root, "dist"))
	require.NoError(t, verr)
	assert.True(t, res.OK(), "%v", res.Problems)
}

// ---- npm ----

func TestPublish_NPMDryRunPrintsThePackCommands(t *testing.T) {
	root := publishProject(t)
	publishTo, publishNPMScope, publishDryRun = publish.TargetNPM, "@acme", true

	out, err := runPublishCapture(t)

	require.NoError(t, err)
	assert.Contains(t, out, "npm pack --ignore-scripts --pack-destination npm npm/package")
	assert.Contains(t, out, "npm publish npm/acme-acme-1.4.0.tgz --access restricted --ignore-scripts")
	assert.NoDirExists(t, filepath.Join(root, "dist"))
}

func TestPublish_NPMExecuteUsesAFilteredEnvironment(t *testing.T) {
	publishProject(t)
	t.Setenv("NODE_AUTH_TOKEN", "token-for-npm-only")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "must-not-reach-npm")
	publishTo, publishNPMScope, publishExecute, publishYes, publishChannel = publish.TargetNPM, "@acme", true, true, "canary"
	fake := &runner.Fake{Handle: func(spec runner.Spec) runner.Result {
		if len(spec.Argv) > 1 && spec.Argv[0] == "npm" && spec.Argv[1] == "view" {
			return runner.Result{Status: runner.StatusExit, ExitCode: 1, Stderr: []byte("npm ERR! code E404")}
		}
		return runner.Result{Status: runner.StatusOK}
	}}
	publishRunner = fake

	_, err := runPublishCapture(t)

	require.NoError(t, err)
	var npmCalls [][]string
	for _, c := range fake.Calls() {
		if c.Argv[0] != "npm" {
			continue
		}
		npmCalls = append(npmCalls, c.Argv[:2])
		env := strings.Join(c.Env, "\n")
		assert.Contains(t, env, "NODE_AUTH_TOKEN=token-for-npm-only")
		assert.NotContains(t, env, "AWS_SECRET_ACCESS_KEY")
		assert.True(t, strings.HasSuffix(filepath.ToSlash(c.Dir), "/dist"), c.Dir)
		assert.False(t, c.InheritEnv)
	}
	assert.Equal(t, [][]string{{"npm", "view"}, {"npm", "pack"}, {"npm", "publish"}}, npmCalls)
	for name, content := range readDist(t, publishDist) {
		assert.NotContains(t, content, "token-for-npm-only", name)
	}
	assert.Contains(t, readDist(t, publishDist)["publish-plan.json"], `"tag": "canary"`)
}

func TestPublish_NPMNeedsAScope(t *testing.T) {
	publishProject(t)
	publishTo, publishDryRun = publish.TargetNPM, true

	_, err := runPublishCapture(t)

	requirePublishError(t, err, publish.CodeConfig, publish.ExitFailed)
}

// ---- oci ----

func localRegistryHost(t *testing.T) string {
	t.Helper()
	t.Setenv("DOCKER_CONFIG", t.TempDir()) // never read the developer's registry credentials
	srv := httptest.NewServer(registry.New(registry.Logger(log.New(io.Discard, "", 0))))
	t.Cleanup(srv.Close)
	return strings.TrimPrefix(srv.URL, "http://")
}

func TestPublish_OCIPushAndVerifyAgainstALocalRegistry(t *testing.T) {
	root := publishProject(t)
	host := localRegistryHost(t)
	publishTo, publishOCIRef, publishExecute, publishYes = publish.TargetOCI, host+"/acme/skills/acme", true, true

	out, err := runPublishCapture(t)

	require.NoError(t, err)
	dist := readDist(t, filepath.Join(root, "dist"))
	var plan publish.Plan
	require.NoError(t, json.Unmarshal([]byte(dist["publish-plan.json"]), &plan))
	assert.Contains(t, out, "push        "+host+"/acme/skills/acme:1.4.0")
	assert.Equal(t, publish.Digest([]byte(dist["oci/manifest.json"])), plan.OCIDigest)

	publishExecute, publishTo = false, ""
	var verifyOut strings.Builder
	_, _ = capture(t, func() {
		err = runPublishVerify(context.Background(), &verifyOut, host+"/acme/skills/acme@"+plan.OCIDigest)
	})
	require.NoError(t, err)
	assert.Contains(t, verifyOut.String(), "verified acme 1.4.0")
}

func TestPublish_OCIWithoutARepositoryIsAConfigError(t *testing.T) {
	publishProject(t)
	publishTo, publishDryRun = publish.TargetOCI, true

	_, err := runPublishCapture(t)

	requirePublishError(t, err, publish.CodeConfig, publish.ExitFailed)
}

func TestPublishVerify_RejectsNeitherDirectoryNorReference(t *testing.T) {
	resetPublishFlags(t)

	err := runPublishVerify(context.Background(), &strings.Builder{}, "no-such-thing")

	requirePublishError(t, err, publish.CodeVerify, publish.ExitFailed)
}

// ---- signing ----

func writeSigningKeys(t *testing.T) (priv, pub string) {
	t.Helper()
	privPEM, pubPEM, err := signing.GenerateKeyPair([]byte("pw"))
	require.NoError(t, err)
	dir := t.TempDir()
	priv, pub = filepath.Join(dir, "release.key"), filepath.Join(dir, "release.pub")
	require.NoError(t, os.WriteFile(priv, privPEM, 0o600))
	require.NoError(t, os.WriteFile(pub, pubPEM, 0o600))
	t.Setenv("AI_RULEZ_SIGNING_KEY_PASSWORD", "pw")
	return priv, pub
}

func TestPublish_SignedBundleVerifiesWithTheTrustedKeyOnly(t *testing.T) {
	root := publishProject(t)
	priv, pub := writeSigningKeys(t)
	_, otherPub := writeSigningKeys(t)
	publishSignKey = priv
	_, err := runPublishCapture(t)
	require.NoError(t, err)
	dist := filepath.Join(root, "dist")
	files := readDist(t, dist)
	require.Contains(t, files, "acme-1.4.0.tar.gz.sigstore.json")
	m := manifestOf(t, files)
	require.NotNil(t, m.Signature)

	verify := func(args ...string) (string, error) {
		publishVerifyKeys, publishVerifyRequire = nil, false
		for _, a := range args {
			if a == "require" {
				publishVerifyRequire = true
			} else {
				publishVerifyKeys = append(publishVerifyKeys, a)
			}
		}
		var out strings.Builder
		var verr error
		_, _ = capture(t, func() { verr = runPublishVerify(context.Background(), &out, dist) })
		return out.String(), verr
	}

	out, err := verify(pub, "require")
	require.NoError(t, err)
	assert.Contains(t, out, "signature: verified")
	assert.Contains(t, out, "signer: key sha256:")

	out, err = verify()
	require.NoError(t, err)
	assert.Contains(t, out, "signature: unverified")

	_, err = verify("require")
	requirePublishError(t, err, publish.CodeUnsigned, publish.ExitGate)

	_, err = verify(otherPub)
	requirePublishError(t, err, publish.CodeUnsigned, publish.ExitGate)
}

func TestPublish_RequireSignatureStopsAnUnsignedPublish(t *testing.T) {
	root := publishProject(t)
	reconfigure(t, root, publishProjectConfig+"\n[publish]\nrequire_signature = true\n")

	_, err := runPublishCapture(t)

	requirePublishError(t, err, publish.CodeUnsigned, publish.ExitGate)
	assert.NoDirExists(t, filepath.Join(root, "dist"))
}

func TestPublish_RequireSignatureIsMetBySigning(t *testing.T) {
	root := publishProject(t)
	reconfigure(t, root, publishProjectConfig+"\n[publish]\nrequire_signature = true\n")
	priv, _ := writeSigningKeys(t)
	publishSignKey = priv

	_, err := runPublishCapture(t)

	require.NoError(t, err)
	assert.Contains(t, readDist(t, filepath.Join(root, "dist")), "acme-1.4.0.tar.gz.sigstore.json")
}

func TestPublish_VerifyRequiresAPairOfIdentityFlags(t *testing.T) {
	root := publishProject(t)
	_, err := runPublishCapture(t)
	require.NoError(t, err)
	publishVerifyIdentity = "someone"

	err = runPublishVerify(context.Background(), &strings.Builder{}, filepath.Join(root, "dist"))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "go together")
}

// ---- approval ----

const publishGovernance = "\n[governance]\nrequire_approval = [\"kind:skill\"]\n"

func TestPublish_RequireApprovedFailsWithoutAPolicy(t *testing.T) {
	root := publishProject(t)
	reconfigure(t, root, publishProjectConfig+"\n[publish]\nrequire_approved = true\n")

	_, err := runPublishCapture(t)

	requirePublishError(t, err, publish.CodeUnapproved, publish.ExitGate)
	assert.Contains(t, err.Error(), "selects nothing")
}

func TestPublish_RequireApprovedFailsUntilTheSkillIsApproved(t *testing.T) {
	root := publishProject(t)
	reconfigure(t, root, publishProjectConfig+publishGovernance+"\n[publish]\nrequire_approved = true\n")

	_, err := runPublishCapture(t)
	requirePublishError(t, err, publish.CodeUnapproved, publish.ExitGate)
	assert.Contains(t, err.Error(), "skill deploy (missing)")

	approveYes, approveReviewer = true, "alice@example.org"
	t.Cleanup(resetApproveFlags)
	mustApprove(t, "skill:deploy")
	publishGit(t, root, "add", "-A")
	publishGit(t, root, "commit", "-q", "-m", "approve")
	_, err = runPublishCapture(t)
	require.NoError(t, err)

	files := readDist(t, filepath.Join(root, "dist"))
	m := manifestOf(t, files)
	require.NotNil(t, m.Approval)
	assert.Equal(t, 1, m.Approval.Required)
	assert.Equal(t, 1, m.Approval.Approved)
	assert.NotContains(t, files["ai-rulez.lock"], "alice@example.org", "reviewer emails stay in the repository")
}

func TestPublish_ApprovalStateIsRecordedEvenWhenNotRequired(t *testing.T) {
	root := publishProject(t)
	reconfigure(t, root, publishProjectConfig+publishGovernance)
	approveYes, approveReviewer = true, "alice@example.org"
	t.Cleanup(resetApproveFlags)
	mustApprove(t, "skill:deploy")
	publishGit(t, root, "add", "-A")
	publishGit(t, root, "commit", "-q", "-m", "approve")

	_, err := runPublishCapture(t)

	require.NoError(t, err)
	m := manifestOf(t, readDist(t, filepath.Join(root, "dist")))
	require.NotNil(t, m.Approval)
	assert.Equal(t, 1, m.Approval.Required)
	assert.Equal(t, 1, m.Approval.Approved)
}

// ---- multi-plugin ----

const publishDomainsConfig = `version = "4.0"
name = "acme"
presets = ["claude"]

[plugin]
name = "acme"
description = "Acme skills."
version = "1.4.0"
repository = "https://github.com/acme/skills"
runtimes = ["claude"]

[plugin.author]
name = "Jane"

[marketplace]
name = "acme-market"

[marketplace.from_domains]
name_prefix = "acme-"
`

func multiProject(t *testing.T) string {
	t.Helper()
	root := publishProject(t)
	writeFile(t, filepath.Join(root, ".ai-rulez", "domains", "alpha", "skills", "a-skill", "SKILL.md"),
		"---\nname: a-skill\ndescription: Use when working on alpha things; not otherwise.\n---\n\n# A\n\nDo alpha.\n")
	writeFile(t, filepath.Join(root, ".ai-rulez", "domains", "beta", "skills", "b-skill", "SKILL.md"),
		"---\nname: b-skill\ndescription: Use when working on beta things; not otherwise.\n---\n\n# B\n\nDo beta.\n")
	reconfigure(t, root, publishDomainsConfig)
	return root
}

func TestPublish_MultiPluginWritesOneDistPerPluginAndAnAggregate(t *testing.T) {
	root := multiProject(t)
	publishMarketplace, publishTag = true, "v2.0.0"
	publishTag = ""
	publishChannel = "stable"
	publishEmit = []string{"kiro-steering"}
	publishExperimental = true

	_, err := runPublishCapture(t)

	// A pinned multi-plugin index needs one ref: no --tag and no channel ref is an error.
	requirePublishError(t, err, publish.CodeConfig, publish.ExitFailed)

	publishTag = ""
	reconfigure(t, root, publishDomainsConfig+"\n[publish.marketplace.channels]\nstable = \"main\"\n")
	_, err = runPublishCapture(t)
	require.NoError(t, err)

	dist := filepath.Join(root, "dist")
	files := readDist(t, dist)
	for _, p := range []string{"acme-alpha", "acme-beta"} {
		assert.Contains(t, files, "plugins/"+p+"/"+p+"-1.4.0.tar.gz", p)
		assert.Contains(t, files, "plugins/"+p+"/SHA256SUMS", p)
	}
	assert.Contains(t, files, "aggregate/marketplace/stable/.claude-plugin/marketplace.json")
	assert.Contains(t, files, "aggregate/emit/kiro-steering/distribution.json")
	assert.Contains(t, files["aggregate/marketplace/stable/.claude-plugin/marketplace.json"], `"path": "plugins/acme-alpha"`)

	publishFormat = ""
	var out strings.Builder
	_, _ = capture(t, func() { err = runPublishVerify(context.Background(), &out, dist) })
	require.NoError(t, err)
	assert.Contains(t, out.String(), "plugins/acme-alpha: verified acme-alpha 1.4.0")
	assert.Contains(t, out.String(), "aggregate: verified")

	files = readDist(t, dist)
	require.NoError(t, os.WriteFile(filepath.Join(dist, "plugins", "acme-beta", "ai-rulez.lock"), []byte("tampered"), 0o600))
	_, _ = capture(t, func() { err = runPublishVerify(context.Background(), &out, dist) })
	requirePublishError(t, err, publish.CodeVerify, publish.ExitGate)
}

func TestPublish_MultiPluginOnlySelectsPlugins(t *testing.T) {
	root := multiProject(t)
	publishOnly = []string{"acme-beta"}

	_, err := runPublishCapture(t)

	require.NoError(t, err)
	files := readDist(t, filepath.Join(root, "dist"))
	assert.Contains(t, files, "plugins/acme-beta/acme-beta-1.4.0.tar.gz")
	assert.NotContains(t, files, "plugins/acme-alpha/acme-alpha-1.4.0.tar.gz")

	publishOnly = []string{"nope"}
	_, err = runPublishCapture(t)
	requirePublishError(t, err, publish.CodeConfig, publish.ExitFailed)
}

func TestPublish_OnlyNeedsAMultiPluginProject(t *testing.T) {
	publishProject(t)
	publishOnly = []string{"acme"}

	_, err := runPublishCapture(t)

	requirePublishError(t, err, publish.CodeConfig, publish.ExitFailed)
}

func TestPublish_MultiPluginExecutesEachPluginsTarget(t *testing.T) {
	multiProject(t)
	publishTo, publishNPMScope, publishExecute, publishYes = publish.TargetNPM, "@acme", true, true
	fake := &runner.Fake{Handle: func(spec runner.Spec) runner.Result {
		if spec.Argv[0] == "npm" && spec.Argv[1] == "view" {
			return runner.Result{Status: runner.StatusExit, ExitCode: 1, Stderr: []byte("E404")}
		}
		return runner.Result{Status: runner.StatusOK}
	}}
	publishRunner = fake

	_, err := runPublishCapture(t)

	require.NoError(t, err)
	var published []string
	for _, c := range fake.Calls() {
		if c.Argv[0] == "npm" && c.Argv[1] == "publish" {
			published = append(published, c.Argv[2])
		}
	}
	assert.Equal(t, []string{"npm/acme-acme-alpha-1.4.0.tgz", "npm/acme-acme-beta-1.4.0.tgz"}, published)
}

func TestPublish_MultiPluginRuntimeFilter(t *testing.T) {
	root := multiProject(t)
	publishRuntimes = []string{"claude"}

	_, err := runPublishCapture(t)

	require.NoError(t, err)
	var m publish.Manifest
	files := readDist(t, filepath.Join(root, "dist"))
	require.NoError(t, json.Unmarshal([]byte(files["plugins/acme-alpha/acme-alpha-1.4.0.manifest.json"]), &m))
	assert.Equal(t, []string{"claude"}, m.Runtimes)
}
