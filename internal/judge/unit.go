package judge

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/Goldziher/ai-rulez/v5/internal/genmark"
	"github.com/Goldziher/ai-rulez/v5/internal/llm"
	"github.com/Goldziher/ai-rulez/v5/internal/secretpat"
)

// Kind is what a unit is.
type Kind string

// The unit kinds.
const (
	KindSkill     Kind = "skill"
	KindAgent     Kind = "agent"
	KindCommand   Kind = "command"
	KindRule      Kind = "rule"
	KindClaudeMD  Kind = "claude-md"
	KindAgentsMD  Kind = "agents-md"
	KindMCPConfig Kind = "mcp-config"
	KindSettings  Kind = "settings"
	KindPlugin    Kind = "plugin"
	KindOther     Kind = "other"
)

// Limits on what one unit sends. A skill with more files or bytes is reviewed in part and reported as
// partial: nothing is silently left out.
const (
	// MaxUnitFiles is the most files one unit reviews.
	MaxUnitFiles = 40
	// MaxUnitBytes is the most text one unit reviews.
	MaxUnitBytes = 512 * 1024
	// MaxFileBytes is the largest single file read at all.
	MaxFileBytes = 2 << 20
)

// Why a file or a unit is not sent.
const (
	SkipSecrets    = "secrets"
	SkipEmpty      = "empty"
	SkipBinary     = "binary"
	SkipUnreadable = "unreadable"
	SkipGenerated  = "generated"
	SkipLimit      = "limit"
)

// SourceFile is one file as the caller read it.
type SourceFile struct {
	// Path is the slash path relative to the project root.
	Path string
	// Data is the file content; empty when Skip is set.
	Data []byte
	// Bytes is the size of the file on disk (for a skipped file the data may be absent).
	Bytes int
	// Skip and SkipReason say why the caller could not read it (a symlink, over the size limit).
	Skip, SkipReason string
}

// UnitFile is a file of a unit, ready to send.
type UnitFile struct {
	Path string
	// Role is skill-md, script, reference, config or text.
	Role string
	// Lines is the text as sent: secret-looking text masked, fence-looking runs defused, split into lines.
	Lines    []string
	Bytes    int
	Redacted bool
	// Omitted is set when the file is not sent (binary, unreadable, over the unit limit) and says why.
	Omitted string
	// RawSHA256 is the hash of the bytes on disk.
	RawSHA256 string
}

// Unit is what a review judges as one piece: a skill directory or a single file.
type Unit struct {
	// ID is "<kind>:<path>".
	ID   string
	Kind Kind
	// Path is the skill directory or the file.
	Path string
	// Primary is the path of the file a human would open first (SKILL.md, the agent file).
	Primary string
	Files   []UnitFile
	// Hash covers the path and the bytes of every file, before any masking.
	Hash string
	// Status is non-empty for a unit that is not sent: StatusWithheld or StatusSkipped.
	Status string
	Reason string
}

// The statuses a unit can have.
const (
	StatusPass      = "pass"
	StatusWarn      = "warn"
	StatusFail      = "fail"
	StatusPartial   = "partial"
	StatusWithheld  = "withheld_secret"
	StatusSkipped   = "skipped"
	StatusError     = "error"
	StatusWouldSend = "would_send"
)

// UnitOptions tunes BuildUnit.
type UnitOptions struct {
	// Redact masks secret-looking text and sends the unit instead of withholding it.
	Redact bool
	// Secrets are values (API keys) that no unit may carry; they are masked like secret-looking text.
	Secrets []string
}

// Ref names a unit before its files are read.
type Ref struct {
	Kind Kind
	// Path is the skill directory or the file.
	Path string
	// Primary is the file a human opens first.
	Primary string
	// Members are the files of the unit, Primary included. A single-file unit has one.
	Members []string
}

// kindOfName is the kind of a file name that is one kind wherever it sits.
func kindOfName(base string) (Kind, bool) {
	switch base {
	case "SKILL.md":
		return KindSkill, true
	case "CLAUDE.md", "CLAUDE.local.md":
		return KindClaudeMD, true
	case "AGENTS.md", "AGENTS.local.md":
		return KindAgentsMD, true
	case ".mcp.json":
		return KindMCPConfig, true
	}
	return "", false
}

// ClassifyFile returns the unit kind of a single file by its name and place, and whether a harness
// reads that place by convention (a file named explicitly is judged either way).
func ClassifyFile(rel string) (kind Kind, conventional bool) {
	if k, ok := kindOfName(path.Base(rel)); ok {
		return k, true
	}
	if k, ok := classifyConfigFile(rel); ok {
		return k, true
	}
	ext := strings.ToLower(path.Ext(rel))
	if ext != ".md" && ext != ".mdc" {
		return KindOther, false
	}
	return classifyContentFile(rel)
}

// classifyConfigFile recognizes the JSON files of a fixed place: MCP files of the editors,
// Claude Code settings and plugin manifests.
func classifyConfigFile(rel string) (Kind, bool) {
	base, holder := path.Base(rel), path.Base(path.Dir(rel))
	switch {
	case base == "mcp.json" && (holder == ".cursor" || holder == ".vscode"):
		return KindMCPConfig, true
	case (base == "settings.json" || base == "settings.local.json") && holder == ".claude":
		return KindSettings, true
	case holder == ".claude-plugin" && (base == "plugin.json" || base == "marketplace.json"):
		return KindPlugin, true
	}
	return "", false
}

// kindOfContentDir is the kind of the Markdown below a directory of this name.
func kindOfContentDir(name string) (Kind, bool) {
	switch name {
	case "agents":
		return KindAgent, true
	case "commands":
		return KindCommand, true
	case "rules":
		return KindRule, true
	case "context", "checks":
		return KindOther, true
	}
	return "", false
}

// isConventionalHolder reports whether a directory is one a harness reads content from.
func isConventionalHolder(name string) bool {
	switch name {
	case ".claude", ".agents", ".ai-rulez", ".cursor", ".github":
		return true
	}
	return false
}

// classifyContentFile classifies Markdown by the nearest agents/commands/rules directory above it.
func classifyContentFile(rel string) (kind Kind, conventional bool) {
	segs := strings.Split(rel, "/")
	for i := len(segs) - 2; i >= 0; i-- {
		if segs[i] == "skills" {
			return KindOther, false // a resource of a skill
		}
		if k, ok := kindOfContentDir(segs[i]); ok {
			return k, i > 0 && isConventionalHolder(segs[i-1])
		}
	}
	return KindOther, false
}

// Discover groups candidate files (slash paths relative to the project root) into units. A skill is
// every file below its SKILL.md directory; any other recognized file is a unit of its own. When
// explicit is true every Markdown or JSON file in the list is a unit, even in a place no harness
// reads; otherwise only the conventional places are.
func Discover(files []string, explicit bool) []Ref {
	sorted := append([]string(nil), files...)
	sort.Strings(sorted)
	skillDirs := map[string]bool{}
	for _, f := range sorted {
		if path.Base(f) == "SKILL.md" {
			skillDirs[path.Dir(f)] = true
		}
	}
	skills := map[string]*Ref{}
	var refs []Ref
	for _, f := range sorted {
		if dir := closestSkillDir(f, skillDirs); dir != "" {
			r := skills[dir]
			if r == nil {
				r = &Ref{Kind: KindSkill, Path: dir, Primary: path.Join(dir, "SKILL.md")}
				skills[dir] = r
			}
			r.Members = append(r.Members, f)
			continue
		}
		if kind, ok := singleFileKind(f, explicit); ok {
			refs = append(refs, Ref{Kind: kind, Path: f, Primary: f, Members: []string{f}})
		}
	}
	for _, r := range skills {
		// SKILL.md first, then the rest in path order (the files arrive sorted).
		sort.SliceStable(r.Members, func(i, j int) bool {
			return r.Members[i] == r.Primary && r.Members[j] != r.Primary
		})
		refs = append(refs, *r)
	}
	sort.Slice(refs, func(i, j int) bool { return refs[i].Path < refs[j].Path })
	return refs
}

// closestSkillDir is the nearest skill directory that holds f, or "".
func closestSkillDir(f string, skillDirs map[string]bool) string {
	for d := path.Dir(f); ; d = path.Dir(d) {
		if skillDirs[d] {
			return d
		}
		if d == "." || d == "/" {
			return ""
		}
	}
}

// singleFileKind is the kind of a file that is a unit of its own: one in a conventional place, or,
// when files were named explicitly, any Markdown or JSON file.
func singleFileKind(f string, explicit bool) (Kind, bool) {
	kind, conventional := ClassifyFile(f)
	if conventional {
		return kind, true
	}
	switch strings.ToLower(path.Ext(f)) {
	case ".md", ".mdc", ".json":
		return kind, explicit
	}
	return kind, false
}

// BuildUnit reads a Ref's files into a Unit: it screens each file (binary, empty, generated, secret-looking
// text), applies the unit limits and prepares the text exactly as it will be sent.
func BuildUnit(ref Ref, read func(rel string) SourceFile, opts UnitOptions) Unit {
	u := Unit{ID: string(ref.Kind) + ":" + ref.Path, Kind: ref.Kind, Path: SafePath(ref.Path, opts.Secrets), Primary: ref.Primary}
	hash := sha256.New()
	total := 0
	secretsIn := ""
	for i, member := range ref.Members {
		src := read(member)
		src.Path = member
		hash.Write([]byte(member + "\x00"))
		hash.Write(src.Data)
		hash.Write([]byte{0})
		raw := sha256.Sum256(src.Data)
		f := UnitFile{Path: SafePath(member, opts.Secrets), Role: fileRole(ref, member), Bytes: max(src.Bytes, len(src.Data)), RawSHA256: hex.EncodeToString(raw[:])}
		if why := omissionReason(i, &src, total); why != "" {
			f.Omitted = why
			if member == ref.Primary && strings.HasPrefix(why, generatedPrefix) {
				u.Status, u.Reason = StatusSkipped, "generated file: judge its sources"
			}
		} else {
			total += len(src.Data)
			var hits []secretpat.Hit
			f.Lines, f.Redacted, hits = prepareText(string(src.Data), opts)
			if len(hits) > 0 && !opts.Redact && secretsIn == "" {
				secretsIn = fmt.Sprintf("%s:%d (%s)", f.Path, hits[0].Line, hits[0].Name)
			}
		}
		u.Files = append(u.Files, f)
	}
	u.Hash = hex.EncodeToString(hash.Sum(nil))
	switch {
	case u.Status != "":
	case secretsIn != "":
		u.Status, u.Reason = StatusWithheld, "secret-looking text at "+secretsIn+"; nothing of this unit was sent (remove it, or pass --redact to send it masked)"
		for i := range u.Files {
			u.Files[i].Lines = nil
		}
	case !hasLines(u.Files, ref.Primary):
		u.Status, u.Reason = StatusSkipped, firstOmitted(u.Files, ref.Primary)
	}
	return u
}

// generatedPrefix starts the reason of a file that carries the ai-rulez generated header.
const generatedPrefix = "not sent: generated file"

// omissionReason says why a file of a unit is not sent, or "" when it is. total is the text sent so far.
func omissionReason(index int, src *SourceFile, total int) string {
	switch {
	case src.Skip != "":
		return src.SkipReason
	case index >= MaxUnitFiles:
		return "not sent: the unit has more than " + itoa(MaxUnitFiles) + " files"
	case bytes.IndexByte(src.Data, 0) >= 0 || !utf8.Valid(src.Data):
		return "not sent: not UTF-8 text"
	case strings.TrimSpace(string(src.Data)) == "":
		return "not sent: the file is empty"
	case total+len(src.Data) > MaxUnitBytes:
		return "not sent: the unit is larger than " + itoa(MaxUnitBytes/1024) + " KiB"
	case genmark.IsGeneratedOutput(src.Path, src.Data):
		return generatedPrefix + " (judge its sources)"
	}
	return ""
}

// prepareText masks secret-looking text, defuses fence-looking runs and splits the text into lines.
// hits are the credentials the security scan (AR001) finds in the text as given; only they withhold a unit.
func prepareText(text string, opts UnitOptions) (lines []string, redacted bool, hits []secretpat.Hit) {
	given := maskSecretValues(text, opts.Secrets)
	hits = secretpat.Scan(given)
	if given != text {
		hits = append([]secretpat.Hit{{Name: "configured API key", Line: firstChangedLine(text, given)}}, hits...)
	}
	masked := llm.RedactForReview(given)
	return splitLines(defuseMarkers(masked)), masked != text, hits
}

// firstChangedLine is the 1-based line of the first difference between a and b.
func firstChangedLine(a, b string) int {
	al, bl := strings.Split(a, "\n"), strings.Split(b, "\n")
	for i := range al {
		if i >= len(bl) || al[i] != bl[i] {
			return i + 1
		}
	}
	return 1
}

func hasLines(files []UnitFile, primary string) bool {
	for _, f := range files {
		if len(f.Lines) > 0 && (f.Path == primary || primary == "") {
			return true
		}
	}
	for _, f := range files {
		if len(f.Lines) > 0 {
			return true
		}
	}
	return false
}

func firstOmitted(files []UnitFile, primary string) string {
	for _, f := range files {
		if f.Path == primary && f.Omitted != "" {
			return f.Omitted
		}
	}
	for _, f := range files {
		if f.Omitted != "" {
			return f.Omitted
		}
	}
	return "nothing to send"
}

func fileRole(ref Ref, member string) string {
	switch {
	case member == ref.Primary && ref.Kind == KindSkill:
		return "skill-md"
	case member == ref.Primary:
		return string(ref.Kind)
	case strings.HasSuffix(member, ".md"):
		return "reference"
	case isScriptName(member):
		return "script"
	case strings.HasSuffix(member, ".json") || strings.HasSuffix(member, ".yaml") || strings.HasSuffix(member, ".yml") || strings.HasSuffix(member, ".toml"):
		return "config"
	}
	return "text"
}

func isScriptName(p string) bool {
	switch strings.ToLower(path.Ext(p)) {
	case ".sh", ".bash", ".zsh", ".py", ".js", ".mjs", ".cjs", ".ts", ".rb", ".pl", ".ps1", ".bat", ".go", ".rs", ".php", ".lua":
		return true
	}
	return false
}

// splitLines splits text into lines without the trailing newline of the last one.
func splitLines(text string) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	text = strings.TrimSuffix(text, "\n")
	return strings.Split(text, "\n")
}

// Sent reports whether any file of the unit has text to send.
func (u Unit) Sent() bool {
	for _, f := range u.Files {
		if len(f.Lines) > 0 {
			return true
		}
	}
	return false
}

// Text returns the sent lines of a file by path.
func (u Unit) fileByPath(p string) (UnitFile, bool) {
	for _, f := range u.Files {
		if f.Path == p {
			return f, true
		}
	}
	return UnitFile{}, false
}

// Description returns the frontmatter description of the primary file when it has one.
func (u Unit) Description() string {
	f, ok := u.fileByPath(u.Primary)
	if !ok {
		return ""
	}
	return frontmatterField(f.Lines, "description")
}

// frontmatterField reads a one-line scalar from the YAML frontmatter at the top of lines.
func frontmatterField(lines []string, key string) string {
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return ""
	}
	for _, l := range lines[1:] {
		if strings.TrimSpace(l) == "---" {
			break
		}
		if k, v, ok := strings.Cut(l, ":"); ok && strings.TrimSpace(k) == key {
			return strings.Trim(strings.TrimSpace(v), `"'`)
		}
	}
	return ""
}
