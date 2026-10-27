package generator

import (
	"context"
	"path/filepath"
	"strings"
	"sync"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/jsonmerge"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/registry"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/userscope"
	"github.com/Goldziher/ai-rulez/v5/internal/gitignore"
	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/Goldziher/ai-rulez/v5/schema"
)

const defaultProfileName = "default"
const generatedManifestName = ".generated-manifest.json"

// generatedLocalManifestName is the gitignored manifest of machine-local
// outputs. They stay out of the committed manifest so a teammate's run never
// treats them as stale.
const generatedLocalManifestName = ".generated-manifest.local.json"

// localSourceDirName is the subdirectory of the config dir (.ai-rulez/local)
// holding machine-local override content. It is always gitignored.
const localSourceDirName = "local"

// Generator handles configuration generation
type Generator struct {
	config *config.Config
	// renderedContent is the content tree of the latest collectOutputs pass; the
	// baz budget warning reads it to name the rules the root file inlines.
	renderedContent *config.ContentTree
	// previousFiles is the prior generated manifest as a set, read once per
	// writeOutputs pass for the rules-folder overwrite guard.
	previousFiles map[string]bool
	// skippedPaths holds the relative paths the overwrite guard left alone in
	// the latest writeOutputs pass. They are hand-written, so they stay out of
	// the generated manifest and the managed .gitignore block.
	skippedPaths map[string]bool
	// overwriteUnowned lets generate replace an existing file it cannot prove it
	// wrote (SetOverwriteUnowned, `generate --force`).
	overwriteUnowned bool
	// refusedOutputs and linkedOutputs are what the latest run's output-safety
	// pass found (see output_safety.go): files it will not write, and outputs
	// that are links onto another generated path.
	refusedOutputs []outputRefusal
	linkedOutputs  map[string]bool

	// Machine-local overlay handling (see local_drift.go).
	ctx             context.Context // caller's context (SetContext); NewGenerator starts it at Background
	allowLocalDrift bool            // write merged output even when it drifts from the shared baseline
	lenientMCP      bool            // tolerate unresolved MCP placeholders (baseline renders)
	plan            *localPlan      // baseline comparison for this run; nil without local inputs
	// memberRuntimes, when not empty, limits every [marketplace] member's bundle to these runtimes
	// (WithMemberRuntimes).
	memberRuntimes []string
	// pluginNames and marketplaceNames limit a plugin run to some plugins or
	// marketplaces (WithPluginNames, WithMarketplaceNames).
	pluginNames      []string
	marketplaceNames []string
	// ownerDirs records, for the latest plugin-set collection, which plugin and
	// marketplace each output directory belongs to (VerifyPlugin names them).
	ownerDirs []ownerDir
	// lockRender renders for ai-rulez.lock: MCP placeholders stay as written, so a
	// pinned output carries no secret and no checkout path.
	lockRender bool
	// localManifestPending is set once the run knows it will write the local manifest.
	localManifestPending bool
	renderedMerged       map[string]bool // merged documents this run's outputs write, by manifest path
	localSkipped         bool            // local files exist on disk but were not loaded (--no-local)
	dropExcludes         bool            // clean: .git/info/exclude keeps nothing of this project

	manifests map[string]generatedManifest // manifests read this run, by path

	// pluginOwned caches, per directory, the bundle-relative paths a plugin
	// provenance sidecar declares, so the orphan walk leaves a committed bundle
	// alone (see drift.go pluginOwnedPath).
	pluginOwned map[string]map[string]bool

	// userMode renders for the person rather than a project (see user.go);
	// projectDir is the project the user command runs from, for overlap warnings.
	userMode   bool
	projectDir string
	// userGetenv looks up the layouts' home variables (nil: os.Getenv). userLayouts
	// are the resolved layouts of this run and userHomes the tool homes those
	// variables relocated outside the home directory.
	userGetenv  func(string) string
	userLayouts map[string]*userscope.Layout
	userHomes   []string
	warned      map[string]bool // merged-document warnings already issued by this Generator
	// heldClaims are, by manifest path, the claims of the last render that found their content already in the
	// document and left them to the user (dropUserHeldClaims): still rendered, so never taken back.
	heldClaims map[string][]jsonmerge.Claim
	noticed    []string // skipped files this run reported as no longer managed; recorded by the manifest write
	// userDirs are the directories below the home directory that ai-rulez created
	// (absolute); userDirsRecorded is false for a manifest that predates them.
	userDirs         map[string]bool
	userDirsRecorded bool

	// role is the flattened role a `generate --role` run renders; nil otherwise.
	// It replaces the profile selection (see roles.go).
	role *config.RoleConfig
	// roleOverrides is the skillOverrides the rendered role(s) set, captured at
	// SetRoles so a composed role (which has no single declared name) works too.
	roleOverrides map[string]string

	// mu serializes the operations of this Generator: a watcher asks
	// GeneratedPaths while a run is writing. Generators of different projects
	// share nothing, so they run concurrently.
	mu sync.Mutex

	// hostOverride replaces config.Host when hostSet (see SetHost).
	hostOverride ambient.Host
	hostSet      bool

	// gitignoreSink, when set, collects this run's managed .gitignore patterns
	// into one block at a shared repository toplevel instead of writing a
	// .gitignore beside the config (recursive runs; see gitignore_recursive.go).
	// gitignorePrefix is this root's path relative to the sink's toplevel.
	// renderedServers is what the committed manifest records as Servers on its next
	// write: the MCP server entries this run renders, taken before claims on values
	// the document already holds are dropped (see sharedServerPaths).
	renderedServers map[string][][]string

	gitignoreSink   *GitignoreSink
	gitignorePrefix string
	// excludeLocks, when set, serializes .git/info/exclude rewrites with the
	// other generators of a recursive run.
	excludeLocks *ExcludeLocks
}

type generatedManifest struct {
	Version string   `json:"version"`
	Files   []string `json:"files"`
	// Merged records what ai-rulez wrote into each merged JSON document (see
	// merged_claims.go): in the committed manifest for a document it wrote whole,
	// in the machine-local manifest for one shared with the user.
	Merged map[string][]jsonmerge.Claim `json:"merged,omitempty"`
	// Servers lists, per shared merged document, the member entries ai-rulez wrote
	// from the shared configuration ([[mcp_servers]]), as key paths. It is only a
	// hint for a warning when one of them is still in the document after its source
	// went away: it never licenses removing anything, because this file is
	// committed and anyone can edit it.
	Servers map[string][][]string `json:"servers,omitempty"`
	// Digests holds the SHA-256 of each listed file that has no Content-Hash of
	// its own (JSON, TOML, raw files). Deletion of such a file needs a digest
	// match, so a forged entry cannot delete a file ai-rulez did not write.
	Digests map[string]string `json:"digests,omitempty"`
	// Assets records, for each bundled asset this run wrote, the SHA-256 of its
	// bytes. A bundled asset (a skill's scripts/, references/ or assets/ file) is
	// copied verbatim, so it carries no Content-Hash: the digest is its only
	// proof. Stale cleanup removes a recorded asset only when it still matches
	// its digest and its path is under a generated skills directory (#328); with
	// no manifest a header-less asset is left alone.
	Assets map[string]string `json:"assets,omitempty"`
	// Dirs lists, in the user manifest only, the directories ai-rulez created
	// below the home directory (slash paths relative to it), so clean removes
	// those once empty and never one that existed before. Nil: not recorded.
	Dirs *[]string `json:"dirs,omitempty"`
	// Unmanaged lists, in the machine-local manifest, the generated files that
	// skip_outputs now excludes and that a generate has already reported as no
	// longer managed, so the notice is given once per file and not on every run.
	Unmanaged []string `json:"unmanaged,omitempty"`
}

// SetHost makes the generator use host's environment, clock, process runner and
// logger instead of the ones its config was loaded with (the real process by
// default).
func (g *Generator) SetHost(h ambient.Host) {
	g.hostSet, g.hostOverride = true, h
}

// host is the generator's ambient facilities: SetHost's, else the config's.
func (g *Generator) host() ambient.Host {
	if g.hostSet {
		return g.hostOverride
	}
	if g.config != nil {
		return g.config.Host
	}
	return ambient.Host{}
}

// git answers repository questions through the host's runner.
func (g *Generator) git() gitutil.Git { return gitutil.New(g.host().Runner).WithLog(g.log()) }

// log is the host's logger (the CLI's when unset).
func (g *Generator) log() logger.Logger { return g.host().Logger() }

// NewGenerator creates a new generator
func NewGenerator(cfg *config.Config) *Generator {
	if cfg != nil {
		// [guard] generated = true adds the built-in PreToolUse hook to Hooks, so
		// every hook renderer treats it as any other group.
		cfg.EnableGuardHooks(schema.Version)
		cfg.EnableSessionStartHooks()
	}
	if cfg != nil && cfg.RulesDirs == nil {
		// Created here, before any copy of the config is made or any goroutine adds
		// a folder: AddRulesDir's lazy creation is not synchronized.
		cfg.RulesDirs = &config.RulesDirSet{}
	}
	if cfg != nil && cfg.Registry == nil {
		cfg.Registry = registry.Default()
	}
	return &Generator{
		config: cfg,
		ctx:    context.Background(),
	}
}

// Generate generates all outputs for the specified profile.
func (g *Generator) Generate(profile string) error {
	_, err := g.GenerateFiles(profile)
	return err
}

// GenerateFiles generates all outputs for the specified profile and returns the
// number of files it wrote, directories excluded, so a caller reporting a total
// to the user can report a counted one.
func (g *Generator) GenerateFiles(profile string) (int, error) {
	res, err := g.run(profile, DiskApplier)
	if err != nil {
		return 0, err
	}
	return res.Written, nil
}

// ignoreBeforeWriting makes sure machine-local files and MCP configs holding
// secrets are git-ignored before any is written, and refuses the run when git
// still would not ignore them. It reports whether it wrote the ignore entries.
func (g *Generator) ignoreBeforeWriting(outputs []config.OutputFile) (bool, error) {
	if !g.hasLocalOutputs(outputs) && !g.hasGuardedSecretOutputs(outputs) {
		return false, nil
	}
	if err := g.updateGitignore(outputs); err != nil {
		return false, oops.Wrapf(err, "gitignore machine-local outputs before writing them")
	}
	return true, g.verifyGuardedOutputsIgnored(outputs)
}

// finishGitignore updates .gitignore if enabled, or whenever machine-local
// content exists: local ".local" outputs and the .ai-rulez/local/ source subtree
// are gitignored unconditionally, even when config gitignore is disabled. The
// early pass already wrote the same block unless writing skipped a hand-written
// file, which drops that file's entry.
func (g *Generator) finishGitignore(outputs []config.OutputFile, ignoredEarly bool) {
	// A manifest the early pass could not know about (first run, digests only) is
	// on disk by now and still needs its entry.
	if ignoredEarly && len(g.skippedPaths) == 0 && (g.localManifestPending || !g.localManifestIgnorable()) {
		return
	}
	if g.config.ShouldUpdateGitignore() || g.hasLocalGitignoreTargets() {
		if err := g.updateGitignore(outputs); err != nil {
			g.log().Warn("Failed to update .gitignore", "error", err)
		}
		return
	}
	g.removeStaleGitignoreBlock()
}

// removeStaleGitignoreBlock takes out the managed block an earlier run (gitignore
// was on, or a 4.x default) left behind. With gitignore off the generated files
// are meant to be committed, and the block would keep ignoring them.
func (g *Generator) removeStaleGitignoreBlock() {
	// The toplevel root's .gitignore is the shared recursive-run block: removing
	// it here would race the roots still adding their patterns to it, and the
	// sink rewrites it once at the end (see GitignoreSink.Write). A nested root
	// owns its own file, so it still cleans up a block an earlier run left there.
	if g.gitignoreSink != nil && g.gitignorePrefix == "" {
		return
	}
	gitignorePath := filepath.Join(g.config.BaseDir, ".gitignore")
	if gitignore.IsSymlink(g.config.BaseDir) {
		return
	}
	data, err := gitutil.ReadIgnoreFileOrEmpty(g.log(), gitignorePath)
	if err != nil {
		return
	}
	content := string(data)
	if !strings.Contains(content, gitignore.BeginMarker) && !strings.Contains(content, gitignore.OldHeader) {
		return
	}
	if err := g.dropGitignoreBlock(gitignorePath, content); err != nil {
		g.log().Warn("Failed to remove the stale ai-rulez block from .gitignore", "error", err)
		return
	}
	g.log().Info("Removed the ai-rulez managed block from .gitignore: gitignore is off, so generated files are not ignored", "hint", "set gitignore = true to have ai-rulez ignore them again")
}
