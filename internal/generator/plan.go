package generator

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/rulefiles"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/Goldziher/ai-rulez/v5/internal/templates"
)

// PlanSchema is the version of the Plan document (schema/plan.schema.json). A new
// field is additive; a removed or re-typed one raises it.
const PlanSchema = 1

// Plan actions of a PlanFile.
const (
	// PlanWrite is a file ai-rulez writes whole.
	PlanWrite = "write"
	// PlanMerge is a document shared with the consumer: ai-rulez writes only the
	// keys it owns and preserves the rest.
	PlanMerge = "merge"
	// PlanMkdir is an empty directory ai-rulez creates.
	PlanMkdir = "mkdir"
)

// Plan removal reasons.
const (
	// RemoveStale is a file the previous run recorded and this run no longer renders.
	RemoveStale = "stale"
	// RemoveUnmerge is a document ai-rulez takes its earlier entries out of.
	RemoveUnmerge = "unmerge"
	// RemoveDelete is a merged document nothing user-authored remains in.
	RemoveDelete = "delete"
)

// PlanOptions selects what PlanOutputs renders.
type PlanOptions struct {
	// Profile is the profile to render ("" is the configured default).
	Profile string
	// Role renders a role's slice instead of a profile (see roles.go); it excludes Profile.
	Role string
}

// Plan is everything a generate run would write, merge into or remove, computed
// without touching disk. It is deterministic: the same sources give the same bytes
// (paths sorted, no timestamp, no resolved environment value).
type Plan struct {
	// Schema is PlanSchema.
	Schema int `json:"schema"`
	// Renderer is the generator's render-schema version; a renderer change changes digests.
	Renderer string `json:"renderer"`
	// Profile is the profile that was rendered, or the role when Role is set.
	Profile string `json:"profile,omitempty"`
	Role    string `json:"role,omitempty"`
	// Files are the outputs, sorted by path.
	Files []PlanFile `json:"files"`
	// Removals are the files and document entries a run would take back, sorted by path.
	Removals []PlanRemoval `json:"removals"`
}

// PlanFile is one output.
type PlanFile struct {
	// Path is slash-separated and relative to the project root.
	Path string `json:"path"`
	// Action is PlanWrite, PlanMerge or PlanMkdir.
	Action string `json:"action"`
	// Mode is the permission bits (octal string) of a file; empty for a directory.
	Mode string `json:"mode,omitempty"`
	// Size and SHA256 describe the content ai-rulez renders for the path, as
	// the presets produce it: the Generated stamp and the Content-Hash and
	// Source-Hash lines are not part of it. They are omitted for a sensitive
	// output, whose content may carry a secret, and for an output whose content
	// holds a credential the security scan detects.
	Size   int    `json:"size,omitempty"`
	SHA256 string `json:"sha256,omitempty"`
	// LocalOnly marks a machine-local output (gitignored, never committed).
	LocalOnly bool `json:"local_only,omitempty"`
	// Sensitive marks an output that may carry a secret; it is git-ignored and
	// written owner-only.
	Sensitive bool `json:"sensitive,omitempty"`
	// Raw marks a verbatim payload (skill resources) without a header.
	Raw bool `json:"raw,omitempty"`
}

// PlanRemoval is something a run takes back.
type PlanRemoval struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

// PlanOutputs renders every output for opts in memory and returns the plan, without
// writing, deleting or ignoring anything. It reads the project the way a run does
// (the previous manifest, merged documents) and honours cfg.Host.
//
// MCP placeholders stay as written (${VAR}), so the plan depends on neither the
// environment nor the checkout and never holds a resolved secret. generate itself
// is not yet built on the plan: both share the rendering code (collectOutputs)
// and differ only in what follows it, which a later step (S4) unifies.
func PlanOutputs(ctx context.Context, cfg *config.Config, opts PlanOptions) (*Plan, error) {
	if cfg == nil {
		return nil, oops.Errorf("plan: no configuration")
	}
	if err := ctx.Err(); err != nil {
		return nil, oops.Wrap(err)
	}
	g := NewGenerator(cfg)
	g.SetContext(ctx)
	if opts.Role != "" {
		if opts.Profile != "" {
			return nil, oops.Errorf("plan: a role and a profile cannot be combined")
		}
		if err := g.SetRole(opts.Role); err != nil {
			return nil, oops.Wrapf(err, "resolve role %q", opts.Role)
		}
	}

	generateMu.Lock()
	defer generateMu.Unlock()
	g.beginRun()
	defer g.resetRunState()
	rulefiles.ResetDowngrades()
	defer rulefiles.FlushDowngrades()
	g.lockRender = true // leave ${VAR} and ${PROJECT_ROOT} as written

	outputs, active, err := g.collectOutputs(opts.Profile)
	if err != nil {
		return nil, err
	}
	g.markSensitiveOutputs(outputs)
	g.markPlannedSecretDocuments(outputs)

	plan := &Plan{Schema: PlanSchema, Renderer: templates.GeneratorSchemaVersion, Files: []PlanFile{}, Removals: []PlanRemoval{}}
	if role := g.Role(); role != "" {
		plan.Role = role
	} else {
		plan.Profile = active
	}
	for i := range outputs {
		plan.Files = append(plan.Files, g.planFile(&outputs[i]))
	}
	sort.Slice(plan.Files, func(i, j int) bool { return plan.Files[i].Path < plan.Files[j].Path })

	for _, stale := range g.staleManifestFiles(outputs) {
		plan.Removals = append(plan.Removals, PlanRemoval{Path: g.relSlash(stale), Reason: RemoveStale})
	}
	for _, edit := range g.planUnmerge(outputs, false) {
		reason := RemoveUnmerge
		if edit.delete {
			reason = RemoveDelete
		}
		plan.Removals = append(plan.Removals, PlanRemoval{Path: edit.rel, Reason: reason})
	}
	sort.Slice(plan.Removals, func(i, j int) bool {
		if plan.Removals[i].Path != plan.Removals[j].Path {
			return plan.Removals[i].Path < plan.Removals[j].Path
		}
		return plan.Removals[i].Reason < plan.Removals[j].Reason
	})
	return plan, nil
}

// markPlannedSecretDocuments flags every MCP config document as sensitive when a
// server carries a secret. A real run decides from the resolved content, which a
// plan never has (it keeps ${VAR} as written), so the plan is conservative: it may
// call a document sensitive that a run finds clean, never the reverse.
func (g *Generator) markPlannedSecretDocuments(outputs []config.OutputFile) {
	if !g.mcpMayCarrySecret() {
		return
	}
	scopes := g.scopeDirs()
	for i := range outputs {
		o := &outputs[i]
		if !o.IsDir && isMCPConfigOutputIn(g.relSlash(g.absOutputPath(o.Path)), scopes) {
			o.Sensitive = true
		}
	}
}

// mcpMayCarrySecret reports whether a server takes an environment value or header
// from a placeholder, or carries a literal secret.
func (g *Generator) mcpMayCarrySecret() bool {
	for _, server := range g.config.MCPServers {
		if server == nil {
			continue
		}
		if len(literalSecrets(server)) > 0 {
			return true
		}
		for _, values := range []map[string]string{server.Env, server.Headers} {
			for _, v := range values {
				if mcpEnvPlaceholderPattern.MatchString(v) {
					return true
				}
			}
		}
	}
	return false
}

func (g *Generator) planFile(o *config.OutputFile) PlanFile {
	f := PlanFile{Path: g.relSlash(g.absOutputPath(o.Path)), LocalOnly: o.LocalOnly, Sensitive: o.Sensitive, Raw: o.RawContent != nil}
	if o.IsDir {
		f.Action = PlanMkdir
		return f
	}
	f.Action = PlanWrite
	if o.PartiallyOwned {
		f.Action = PlanMerge
	}
	mode := o.Mode.Perm()
	if mode == 0 {
		mode = 0o644
	}
	if o.Sensitive {
		mode = sensitiveFileMode
	}
	f.Mode = fmt.Sprintf("%04o", uint32(mode))
	if o.Sensitive {
		return f
	}
	data := o.RawContent
	if data == nil {
		data = []byte(normalizeTrailingNewline(stripGeneratedStamp(o.Content, o.Path))) // as finalContent writes it
	}
	if _, found := lint.DetectSecret(string(data)); found {
		return f // a credential the output carries must not be confirmable from its digest
	}
	sum := sha256.Sum256(data)
	f.Size, f.SHA256 = len(data), hex.EncodeToString(sum[:])
	return f
}

// MarshalPlan encodes p as indented JSON with a trailing newline. The encoding is
// deterministic.
func MarshalPlan(p *Plan) ([]byte, error) {
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return nil, oops.Wrapf(err, "encode plan")
	}
	return append(data, '\n'), nil
}
