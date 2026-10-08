package mcp

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/contentlock"
	"github.com/Goldziher/ai-rulez/v5/internal/generator"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/signing"
	"github.com/Goldziher/ai-rulez/v5/internal/signing/sigstore"
)

func isolateUserDirs(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, "state"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "cache"))
}

// signer is a publisher key pair whose public half is written into the project.
type signer struct {
	ks     *sigstore.KeySigner
	pubPEM []byte
}

func newSigner(t *testing.T) signer {
	t.Helper()
	priv, pub, err := sigstore.GenerateKeyPair(nil)
	require.NoError(t, err)
	ks, err := sigstore.LoadKeySigner(priv, nil)
	require.NoError(t, err)
	return signer{ks: ks, pubPEM: pub}
}

func (s signer) signSkill(t *testing.T, dir string) {
	t.Helper()
	st, _, err := signing.TreeStatement(signing.SubjectSkill, dir, signing.ArtifactMeta{Version: "5.0.0", Now: time.Now()})
	require.NoError(t, err)
	data, err := signing.SignStatement(context.Background(), s.ks, st)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, signing.SidecarName), data, 0o644))
}

const signingSkillTable = "\n[signing]\nrequire = [\"skill\"]\n[[signing.trust]]\nsubject = \"skill\"\nsource = \"vendor\"\nkey_file = \"keys/publisher.pub\"\n"

// publisherProject is a project that serves the skills of one local skill source
// named "vendor" and requires them to carry a publisher signature.
func publisherProject(t *testing.T, pub signer, table string) string {
	t.Helper()
	isolateUserDirs(t)
	cfg := baseConfig + "\n[[skill_sources]]\nname = \"vendor\"\nurl = \"vendor-skills\"\n" + table
	root := project(t, cfg, map[string]string{"skills/local/SKILL.md": skillFile("local", "Authored here", "delivery: served\n")})
	writeFile(t, root, "keys/publisher.pub", string(pub.pubPEM))
	writeFile(t, root, "vendor-skills/shipit/SKILL.md", skillFile("shipit", "Ship it", ""))
	writeFile(t, root, "vendor-skills/shipit/scripts/run.sh", "#!/bin/sh\necho ship\n")
	return root
}

func serve(t *testing.T, root string) *Catalog {
	t.Helper()
	return newServerFor(t, &ServeSetup{WorkDir: root}).Catalog()
}

func TestServe_RequireSkillRefusesRemoteSkillsWithoutATrustedPublisher(t *testing.T) {
	good, other := newSigner(t), newSigner(t)

	tests := []struct {
		name     string
		table    string
		prepare  func(t *testing.T, root string)
		wantCode string
	}{
		{"signed by the trusted publisher", signingSkillTable, func(t *testing.T, root string) {
			good.signSkill(t, filepath.Join(root, "vendor-skills", "shipit"))
		}, ""},
		{"unsigned", signingSkillTable, func(*testing.T, string) {}, signing.CodeMissing},
		{"file changed after signing", signingSkillTable, func(t *testing.T, root string) {
			dir := filepath.Join(root, "vendor-skills", "shipit")
			good.signSkill(t, dir)
			writeFile(t, dir, "scripts/run.sh", "#!/bin/sh\necho changed\n")
		}, signing.CodeSubjectMismatch},
		{"file added after signing", signingSkillTable, func(t *testing.T, root string) {
			dir := filepath.Join(root, "vendor-skills", "shipit")
			good.signSkill(t, dir)
			writeFile(t, dir, "references/extra.md", "unreviewed\n")
		}, signing.CodeSubjectMismatch},
		{"signed by someone else", signingSkillTable, func(t *testing.T, root string) {
			other.signSkill(t, filepath.Join(root, "vendor-skills", "shipit"))
		}, signing.CodeSignerNotTrusted},
		{"a publisher trusted for another source", strings.Replace(signingSkillTable, `source = "vendor"`, `source = "elsewhere"`, 1), func(t *testing.T, root string) {
			good.signSkill(t, filepath.Join(root, "vendor-skills", "shipit"))
		}, signing.CodeSignerNotTrusted},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := publisherProject(t, good, tt.table)
			tt.prepare(t, root)

			cat := serve(t, root)

			assert.Contains(t, catalogNames(cat), "local", "a skill authored in the project is not remote: the publisher rule does not apply")
			r, refused := cat.Refusal("shipit")
			if tt.wantCode == "" {
				assert.False(t, refused, "refused: %+v", r)
				assert.Contains(t, catalogNames(cat), "shipit")
				return
			}
			require.True(t, refused)
			assert.Equal(t, tt.wantCode, r.Code, r.Reason)
			assert.NotContains(t, catalogNames(cat), "shipit")
			assert.Contains(t, r.Reason, "[signing] require")
		})
	}
}

func TestServe_RequireIsOffByDefault(t *testing.T) {
	good := newSigner(t)
	root := publisherProject(t, good, strings.Replace(signingSkillTable, `require = ["skill"]`, "", 1))

	cat := serve(t, root)

	assert.Contains(t, catalogNames(cat), "shipit", "an unsigned skill is served until the policy requires a signature")
}

func TestServe_RequireSkillAllowsTheRewrittenNameOfASource(t *testing.T) {
	// A skill source with name_prefix is served with a rewritten SKILL.md name;
	// the signed file need not carry it.
	good := newSigner(t)
	root := publisherProject(t, good, signingSkillTable)
	writeFile(t, root, ".ai-rulez/config.toml", baseConfig+"\n[[skill_sources]]\nname = \"vendor\"\nurl = \"vendor-skills\"\nname_prefix = \"v-\"\n"+signingSkillTable)
	good.signSkill(t, filepath.Join(root, "vendor-skills", "shipit"))

	cat := serve(t, root)

	assert.Contains(t, catalogNames(cat), "v-shipit")
}

func TestServedMatchesSigned(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "SKILL.md", "---\nname: a\n---\nbody\n")
	writeFile(t, dir, "scripts/run.sh", "echo 1\n")
	tree, err := signing.ReadTreeSubject(signing.KindSkillTree, dir)
	require.NoError(t, err)
	file := func(rel, content string) CatalogFile { return CatalogFile{RelPath: rel, Content: []byte(content)} }

	tests := []struct {
		name     string
		files    []CatalogFile
		wantCode string
	}{
		{"identical", []CatalogFile{file("SKILL.md", "---\nname: a\n---\nbody\n"), file("scripts/run.sh", "echo 1\n")}, ""},
		{"a file differs", []CatalogFile{file("SKILL.md", "---\nname: a\n---\nbody\n"), file("scripts/run.sh", "echo 2\n")}, signing.CodeSubjectMismatch},
		{"SKILL.md differs", []CatalogFile{file("SKILL.md", "other")}, signing.CodeSubjectMismatch},
		{"SKILL.md body differs", []CatalogFile{file("SKILL.md", "---\nname: a\n---\nevil body\n")}, signing.CodeSubjectMismatch},
		{"SKILL.md frontmatter differs beyond the name", []CatalogFile{file("SKILL.md", "---\nname: a\nallowed-tools: all\n---\nbody\n")}, signing.CodeSubjectMismatch},
		{"SKILL.md name rewritten by the source", []CatalogFile{file("SKILL.md", "---\nname: v-a\n---\nbody\n")}, ""},
		{"SKILL.md name added by the source", []CatalogFile{file("SKILL.md", "---\nname: v-a\n---\nbody\n")}, ""},
		{"a served file the signature does not cover", []CatalogFile{file("extra.md", "x")}, signing.CodeSubjectMismatch},
		{"signature files are not content", []CatalogFile{file(signing.SidecarName, "{}"), file(".ai-rulez.2.sigstore.json", "{}")}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := servedMatchesSigned(&CatalogSkill{Files: tt.files}, tree.Tree)

			if tt.wantCode == "" {
				assert.Nil(t, err)
				return
			}
			require.NotNil(t, err)
			assert.Equal(t, tt.wantCode, err.Code)
		})
	}
}

// signedLockProject is a project with a signed lock that pins one authored item
// and trusts one key for the lock.
func signedLockProject(t *testing.T, sign bool) (*config.Config, *lockfile.File) {
	t.Helper()
	isolateUserDirs(t)
	root := t.TempDir()
	cfgDir := filepath.Join(root, ".ai-rulez")
	require.NoError(t, os.MkdirAll(cfgDir, 0o755))
	lock := &lockfile.File{
		Version: lockfile.Version,
		Item:    []lockfile.Item{{Kind: "rule", ID: "style", Path: "rules/style.md", Digest: "sha256:" + strings.Repeat("ab", 32)}},
	}
	lock.Tree = contentlock.TreeOf(lock)
	require.NoError(t, lockfile.Save(cfgDir, lock))
	pub := newSigner(t)
	writeFile(t, root, "keys/release.pub", string(pub.pubPEM))
	if sign {
		data, err := signing.SignLock(context.Background(), pub.ks, lock, signing.LockMeta{Version: "5.0.0", Now: time.Now()})
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(cfgDir, config.SigningAttestationFile), data, 0o644))
	}
	cfg := &config.Config{
		BaseDir: root, ConfigDir: cfgDir,
		Signing: &config.SigningConfig{Require: []string{"served"}, KeyFile: "keys/release.pub"},
	}
	return cfg, lock
}

func scanCatalogForSigning(t *testing.T) *Catalog {
	t.Helper()
	served := []generator.ServedSkill{
		servedSkill("clean", "", "A clean skill", nil),
		servedSkill("second", "", "Another clean skill", nil),
	}
	cat, err := BuildCatalog("p", "claude", served, SkillFilter{})
	require.NoError(t, err)
	return cat
}

func TestAdmit_RequireServedGatesOnTheSignedLock(t *testing.T) {
	cfg, lock := signedLockProject(t, true)
	cat := scanCatalogForSigning(t)
	pinned := mustSkill(t, cat, "clean")
	lock.Set(lockfile.KindServed, lockfile.Entry{Name: "clean", Digest: pinned.LockDigest})
	// The lock changed after it was signed: its pins no longer match the signature.
	require.NoError(t, lockfile.Save(cfg.ConfigDir, lock))
	gate := newSignatureGate(cfg, nil, time.Now())

	admitted := cat.Admit(Admission{Config: cfg, Lock: lock, Enforce: true, Signatures: gate})

	r, refused := admitted.Refusal("clean")
	require.True(t, refused, "an edited lock does not match its signature")
	assert.Equal(t, signing.CodeSubjectMismatch, r.Code, r.Reason)
	assert.Contains(t, r.Reason, `require includes "served"`)
}

func TestAdmit_RequireServedWithAValidSignatureStillNeedsThePin(t *testing.T) {
	cfg, lock := signedLockProject(t, true)
	cat := scanCatalogForSigning(t)
	gate := newSignatureGate(cfg, nil, time.Now())
	require.Nil(t, gate.lockProblem, "the signed lock verifies")

	admitted := cat.Admit(Admission{Config: cfg, Lock: lock, Enforce: true, Signatures: gate})

	r, refused := admitted.Refusal("clean")
	require.True(t, refused, "the verified lock does not pin this skill")
	assert.Equal(t, CodeServedLockMismatch, r.Code, "AR995: the existing served lock check runs under require = served")
}

func TestAdmit_RequireServedRefusesEverySkillWithoutASignature(t *testing.T) {
	cfg, lock := signedLockProject(t, false)
	cat := scanCatalogForSigning(t)
	gate := newSignatureGate(cfg, nil, time.Now())

	admitted := cat.Admit(Admission{Config: cfg, Lock: lock, Enforce: true, Signatures: gate})

	require.NotEmpty(t, cat.Skills())
	for _, s := range cat.Skills() {
		r, refused := admitted.Refusal(s.Name)
		require.True(t, refused, s.Name)
		assert.Equal(t, signing.CodeMissing, r.Code)
	}
	assert.Empty(t, admitted.Skills())
}

func TestServe_RequireServedMakesTheLockPinEverySkill(t *testing.T) {
	isolateUserDirs(t)
	pub := newSigner(t)
	root := project(t, baseConfig+"\n[signing]\nrequire = [\"served\"]\nkey_file = \"keys/release.pub\"\n",
		map[string]string{"skills/heavy/SKILL.md": skillFile("heavy", "Heavy served skill", "delivery: served\n")})
	writeFile(t, root, "keys/release.pub", string(pub.pubPEM))
	cfgDir := filepath.Join(root, ".ai-rulez")
	lock := &lockfile.File{Version: lockfile.Version, Item: []lockfile.Item{{Kind: "rule", ID: "style", Path: "rules/style.md", Digest: "sha256:" + strings.Repeat("ab", 32)}}}
	lock.Tree = contentlock.TreeOf(lock)
	require.NoError(t, lockfile.Save(cfgDir, lock))
	data, err := signing.SignLock(context.Background(), pub.ks, lock, signing.LockMeta{Version: "5.0.0", Now: time.Now()})
	require.NoError(t, err)
	writeFile(t, cfgDir, config.SigningAttestationFile, string(data))

	cat := serve(t, root)

	r, refused := cat.Refusal("heavy")
	require.True(t, refused, "the signed lock does not pin the served skill")
	assert.Equal(t, CodeServedLockMismatch, r.Code, "[lock] enforce is off, and require = served enforces the pins anyway: %s", r.Reason)
	assert.Empty(t, catalogNames(cat))
}

func TestSignatureGate_InstalledSkillsCarryTheirPublisherAttestation(t *testing.T) {
	isolateUserDirs(t)
	pub := newSigner(t)
	root := t.TempDir()
	writeFile(t, root, "keys/publisher.pub", string(pub.pubPEM))
	dir := filepath.Join(root, "cache", "deploy")
	writeFile(t, dir, "SKILL.md", skillFile("deploy", "Deploy", ""))
	cfg := &config.Config{
		BaseDir: root, ConfigDir: filepath.Join(root, ".ai-rulez"),
		InstalledSkills: []config.InstalledSkillConfig{{Name: "deploy", Source: "https://example.org/skills.git"}},
		Content:         &config.ContentTree{Skills: []config.ContentFile{{Name: "deploy", Path: filepath.Join(dir, "SKILL.md")}}},
		Signing: &config.SigningConfig{
			Require: []string{"skill"},
			Trust:   []config.SigningTrust{{Subject: "skill", Source: "deploy", KeyFile: "keys/publisher.pub"}},
		},
	}
	origins := skillOrigins(cfg, nil)
	require.Contains(t, origins, "deploy")
	assert.Equal(t, dir, origins["deploy"].Dir)
	assert.Equal(t, "deploy", origins["deploy"].Source, "an installed skill's trust scope is its configured name")
	gate := newSignatureGate(cfg, origins, time.Now())
	installed := &CatalogSkill{Name: "deploy", Ref: "v1.2.0", Files: []CatalogFile{{RelPath: "SKILL.md"}}}

	r := gate.Check(installed)
	require.NotNil(t, r, "an installed skill without an attestation is refused")
	assert.Equal(t, signing.CodeMissing, r.Code)

	pub.signSkill(t, dir)
	assert.Nil(t, gate.Check(installed), "rendered installed skills are checked against the directory, not byte-compared")
	extra := &CatalogSkill{Name: "deploy", Ref: "v1.2.0", Files: []CatalogFile{{RelPath: "SKILL.md"}, {RelPath: "scripts/unsigned.sh"}}}
	r = gate.Check(extra)
	require.NotNil(t, r, "a served file the publisher never signed is refused for an installed skill too")
	assert.Equal(t, signing.CodeSubjectMismatch, r.Code)

	remote := &CatalogSkill{Name: "stranger", Imported: true}
	r = gate.Check(remote)
	require.NotNil(t, r, "a remote skill whose origin carries no attestation cannot satisfy require = skill")
	assert.Equal(t, signing.CodeMissing, r.Code)

	local := &CatalogSkill{Name: "mine"}
	assert.Nil(t, gate.Check(local))
}
