package generator

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// ConvertRecordName is the file `convert --write` leaves in the config
// directory: the digest of each native file it imported, so the first generate
// may replace those originals (their content now lives under .ai-rulez) while
// still refusing a file nobody imported.
const ConvertRecordName = ".converted.json"

// outputRefusal is a file generate will not write.
type outputRefusal struct {
	rel    string
	reason string
}

const (
	reasonUnowned = "an existing file ai-rulez did not write"
	// reasonLinkFmt takes the link target.
	reasonLinkFmt = "a symlink to %s, which is not a generated path"
)

// SetOverwriteUnowned lets generate replace an existing file it cannot prove it
// wrote (`generate --force`). A symlinked output stays refused.
func (g *Generator) SetOverwriteUnowned(overwrite bool) { g.overwriteUnowned = overwrite }

type convertRecord struct {
	Version string            `json:"version"`
	Files   map[string]string `json:"files"`
}

// WriteConvertRecord records the files `convert` imported (slash paths relative
// to the project, with the bytes they held), merging into an earlier record.
func WriteConvertRecord(configDir string, files map[string][]byte) error {
	if len(files) == 0 {
		return nil
	}
	recordPath := filepath.Join(configDir, ConvertRecordName)
	rec := convertRecord{Version: "1", Files: map[string]string{}}
	if data, err := os.ReadFile(recordPath); err == nil {
		var old convertRecord
		if json.Unmarshal(data, &old) == nil {
			for rel, sum := range old.Files {
				rec.Files[rel] = sum
			}
		}
	}
	for rel, data := range files {
		rec.Files[filepath.ToSlash(rel)] = fileDigest(data)
	}
	out, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return oops.Wrapf(err, "marshal convert record")
	}
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return oops.With("dir", configDir).Wrapf(err, "create config directory")
	}
	return config.WriteFileAtomic(recordPath, append(out, '\n'), 0o644)
}

// convertRecordFiles reads the record (nil when absent or unreadable).
func (g *Generator) convertRecordFiles() map[string]string {
	data, err := g.config.ReadExisting(filepath.Join(g.manifestDir(), ConvertRecordName))
	if err != nil {
		return nil
	}
	var rec convertRecord
	if json.Unmarshal(data, &rec) != nil {
		return nil
	}
	return rec.Files
}

// convertedOriginal reports whether rel is a file `convert` imported: it is in
// the record, whatever it holds now (clean keeps the generated file that
// replaced it).
func (g *Generator) convertedOriginal(rel string) bool {
	_, ok := g.convertRecordFiles()[rel]
	return ok
}

// adoptable reports whether data is still exactly what convert imported at rel.
func (g *Generator) adoptable(rel string, data []byte) bool {
	want, ok := g.convertRecordFiles()[rel]
	return ok && want == fileDigest(data)
}

// retiredFiles is what a generate removes: the stale manifest entries, plus the
// command files `convert` imported that a generated skill now replaces. clean
// does not use it: it keeps every file convert imported.
func (g *Generator) retiredFiles(outputs []config.OutputFile) []string {
	files := g.staleManifestFiles(outputs)
	for _, abs := range g.supersededCommands(outputs) {
		if !slices.Contains(files, abs) {
			files = append(files, abs)
		}
	}
	sort.Strings(files)
	return files
}

// supersededCommands lists the imported `<dir>/commands/<name>.md` files that are
// unchanged since convert and whose `<dir>/skills/<name>/SKILL.md` this run
// generates. Their content lives under the config directory, and a harness that
// loads both would show the command twice. An edited file stays, and so does one
// clean could not put back (restorableCommand): retiring it would leave clean
// with neither the original nor the skill.
func (g *Generator) supersededCommands(outputs []config.OutputFile) []string {
	if g.userMode {
		return nil
	}
	record := g.convertRecordFiles()
	if len(record) == 0 {
		return nil
	}
	written := map[string]bool{}
	for _, output := range outputs {
		if !output.IsDir {
			written[g.relSlash(g.absOutputPath(output.Path))] = true
		}
	}
	var out []string
	for rel := range record {
		dir, file := path.Split(rel)
		if !strings.HasSuffix(dir, "/commands/") || !strings.HasSuffix(file, ".md") || written[rel] {
			continue
		}
		skill := path.Join(path.Dir(path.Dir(dir)), skillsDirName, strings.TrimSuffix(file, ".md"), skillEntryFile)
		if !written[skill] {
			continue
		}
		abs := filepath.Join(g.config.BaseDir, filepath.FromSlash(rel))
		data, err := g.config.ReadExisting(abs)
		if err != nil || !g.adoptable(rel, data) || isSymlink(abs) || !g.removalConfined(abs) ||
			!g.restorableCommand(rel) {
			continue
		}
		out = append(out, abs)
	}
	sort.Strings(out)
	return out
}

// commandSource is the copy `convert` wrote under the config directory of the
// command it imported at rel (`<dir>/commands/<name>.md`).
func (g *Generator) commandSource(rel string) string {
	return filepath.Join(g.manifestDir(), "commands", path.Base(rel))
}

// restorableCommand reports whether the command `convert` imported at rel can be
// put back byte for byte: its copy under the config directory still holds exactly
// the bytes the convert record vouches for.
func (g *Generator) restorableCommand(rel string) bool {
	want, ok := g.convertRecordFiles()[rel]
	if !ok {
		return false
	}
	data, err := g.config.ReadExisting(g.commandSource(rel))
	return err == nil && fileDigest(data) == want
}

// retiredCommand is the imported command a generated skill at abs replaced: the
// `<dir>/commands/<name>.md` the convert record lists for `<dir>/skills/<name>/SKILL.md`,
// now missing from disk because generate retired it (supersededCommands).
func (g *Generator) retiredCommand(abs string) (string, bool) {
	if g.userMode {
		return "", false
	}
	rel := g.relSlash(abs)
	skillDir := path.Dir(rel)
	if path.Base(rel) != skillEntryFile || path.Base(path.Dir(skillDir)) != skillsDirName {
		return "", false
	}
	cmd := path.Join(path.Dir(path.Dir(skillDir)), "commands", path.Base(skillDir)+".md")
	if _, ok := g.convertRecordFiles()[cmd]; !ok {
		return "", false
	}
	if _, err := os.Lstat(filepath.Join(g.config.BaseDir, filepath.FromSlash(cmd))); !os.IsNotExist(err) {
		return "", false
	}
	return cmd, true
}

// restoreRetiredCommands writes back the imported commands clean is about to
// leave without their replacing skill, from the copies under the config directory.
func (g *Generator) restoreRetiredCommands(restored []string) error {
	for _, abs := range restored {
		rel := g.relSlash(abs)
		data, err := g.config.ReadExisting(g.commandSource(rel))
		if err != nil {
			return oops.With("path", rel).Wrapf(err, "read the imported copy of a retired command")
		}
		target, _, err := g.guardWrite(abs)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return oops.With("path", rel).Wrapf(err, "create the directory of a retired command")
		}
		if err := config.WriteFileAtomic(target, data, 0o644); err != nil {
			return oops.With("path", rel).Wrapf(err, "restore a retired command")
		}
	}
	return nil
}

// outputSafety classifies the outputs a project-scope run is about to write.
// refused are files generate will not touch; linked are outputs that are
// symlinks onto another path this run generates (the target gets its own
// content, the link stays as the user made it and is not recorded as ours).
func (g *Generator) outputSafety(outputs []config.OutputFile) (refused []outputRefusal, linked map[string]bool) {
	guarded := g.guardRefusals(outputs)
	if g.userMode {
		refused = g.userOutputSafety(outputs)
		for rel, reason := range guarded {
			if !slices.ContainsFunc(refused, func(r outputRefusal) bool { return r.rel == rel }) {
				refused = append(refused, outputRefusal{rel, reason})
			}
		}
		sort.Slice(refused, func(i, j int) bool { return refused[i].rel < refused[j].rel })
		return refused, nil
	}
	for rel, reason := range guarded {
		refused = append(refused, outputRefusal{rel, reason})
	}
	linked = map[string]bool{}
	written := g.writtenTargets(outputs)
	for _, output := range outputs {
		if output.IsDir {
			continue
		}
		abs := g.absOutputPath(output.Path)
		rel := g.relSlash(abs)
		if _, ok := guarded[rel]; ok {
			continue
		}
		info, err := os.Lstat(abs)
		if err != nil {
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 {
			if reason, isLinked := g.symlinkedOutput(abs, written); isLinked {
				linked[rel] = true
			} else {
				refused = append(refused, outputRefusal{rel, reason})
			}
			continue
		}
		if g.overwriteUnowned || !g.unowned(abs, rel, output, info) {
			continue
		}
		refused = append(refused, outputRefusal{rel, reasonUnowned})
	}
	sort.Slice(refused, func(i, j int) bool { return refused[i].rel < refused[j].rel })
	return refused, linked
}

// writtenTargets is the set of paths the non-link file outputs really land on.
func (g *Generator) writtenTargets(outputs []config.OutputFile) map[string]bool {
	written := map[string]bool{}
	for _, output := range outputs {
		if output.IsDir {
			continue
		}
		abs := g.absOutputPath(output.Path)
		if info, err := os.Lstat(abs); err == nil && info.Mode()&os.ModeSymlink != 0 {
			continue
		}
		if resolved, _, err := resolveWriteTarget(abs, new(int)); err == nil {
			written[resolved] = true
		}
	}
	return written
}

// symlinkedOutput judges an output whose path abs is a symlink. It reports
// linked when the link points onto another path this run writes (written), and
// otherwise the reason generate refuses to write through it.
func (g *Generator) symlinkedOutput(abs string, written map[string]bool) (reason string, linked bool) {
	resolved, _, rerr := resolveWriteTarget(abs, new(int))
	if rerr == nil && written[resolved] {
		return "", true
	}
	shown := g.displayTarget(resolved)
	if filepath.IsAbs(shown) {
		return fmt.Sprintf("a symlink to %s, outside the project", shown), false
	}
	return fmt.Sprintf(reasonLinkFmt, shown), false
}

// guardRefusals runs the write guard over every output, directories included, so
// a symlink anywhere on an output's path that leads out of the places the run may
// write refuses the run before anything is removed or written; the write itself
// would fail half way through. It maps the output's project path to the reason.
// A path the guard rejects for any other reason (outside the project, inside
// .git) is left to the write, which names it.
func (g *Generator) guardRefusals(outputs []config.OutputFile) map[string]string {
	refused := map[string]string{}
	for _, output := range outputs {
		abs := g.absOutputPath(output.Path)
		if !g.withinScope(abs) || g.inGitDir(abs) {
			continue
		}
		if _, _, err := g.guardWrite(abs); err == nil {
			continue
		}
		rel := g.relSlash(abs)
		resolved, _, err := resolveWriteTarget(abs, new(int))
		switch {
		case err != nil:
			refused[rel] = "a symlink chain on its path that cannot be resolved"
		case isSymlink(abs):
			refused[rel] = fmt.Sprintf("a symlink to %s, outside the project", g.displayTarget(resolved))
		default:
			refused[rel] = fmt.Sprintf("a symlinked parent directory leads outside the project, to %s",
				g.displayTarget(resolved))
		}
	}
	return refused
}

// displayTarget shows a link target relative to the project when it is inside it.
func (g *Generator) displayTarget(resolved string) string {
	if rel, err := filepath.Rel(g.config.BaseDir, resolved); err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(rel)
	}
	return resolved
}

// userOutputSafety applies the same proof in user scope, reusing clean's rule for
// what ai-rulez wrote there. Links are left to guardWrite, which keeps them inside the home.
func (g *Generator) userOutputSafety(outputs []config.OutputFile) []outputRefusal {
	if g.overwriteUnowned {
		return nil
	}
	var refused []outputRefusal
	for _, output := range outputs {
		if output.IsDir || output.PartiallyOwned || len(output.MergeClaims) > 0 {
			continue
		}
		abs := g.absOutputPath(output.Path)
		info, err := os.Lstat(abs)
		if err != nil || info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Size() == 0 {
			continue
		}
		if g.isMergedDocument(abs) || g.userManaged(abs, output) {
			continue
		}
		if data, rerr := g.config.ReadExisting(abs); rerr == nil && g.sameAsRendering(data, output) {
			continue
		}
		refused = append(refused, outputRefusal{g.relSlash(abs), reasonUnowned})
	}
	return refused
}

func (g *Generator) sameAsRendering(data []byte, output config.OutputFile) bool {
	if output.RawContent != nil {
		return bytes.Equal(data, output.RawContent)
	}
	return string(data) == g.finalContent(output)
}

// unowned reports whether the existing file at abs is one ai-rulez cannot prove
// it wrote, so writing it would destroy someone's work. Rules folders keep their
// own guard (isUnmanagedRuleFile), merged documents are merged into rather than
// replaced, and an empty file holds nothing to lose.
func (g *Generator) unowned(abs, rel string, output config.OutputFile, info os.FileInfo) bool {
	if info.IsDir() || info.Size() == 0 || output.PartiallyOwned || len(output.MergeClaims) > 0 {
		return false
	}
	if g.isMergedDocument(abs) {
		return false
	}
	if output.RawContent == nil && (g.config.InRulesDir(rel) || isNestedAgentsMD(rel)) {
		return false
	}
	data, err := g.config.ReadExisting(abs)
	if err != nil {
		return false
	}
	if g.sameAsRendering(data, output) || len(bytes.TrimSpace(data)) == 0 {
		return false
	}
	if g.adoptable(rel, data) {
		return false
	}
	switch g.outputProvenance(abs, rel, data, g.headerlessOutput(abs, output)) {
	case provenLegacy:
		g.warnOnce("Overwriting "+rel+": it has no generated header and no recorded digest, and the generated manifest lists it",
			"hint", "an older ai-rulez wrote it; this run records its digest")
		return false
	case provenStrong, provenWeak:
		return false
	}
	return true
}

// headerlessOutput reports whether the rendering of output carries no generated
// banner: raw outputs, and rendered formats with no header (the Codex agent TOML).
// For such a file a missing banner proves nothing, so an older manifest that
// lists it without a digest still vouches for it.
func (g *Generator) headerlessOutput(abs string, output config.OutputFile) bool {
	if output.RawContent != nil {
		return true
	}
	return !hasGeneratedBanner(abs, []byte(g.finalContent(output)))
}

// provenance is how well the bytes at a path show ai-rulez wrote them. generate
// and clean share outputProvenance and differ only in how much they accept.
type provenance int

const (
	// provenNone: nothing shows ai-rulez wrote the file.
	provenNone provenance = iota
	// provenLegacy: the manifest lists a header-less format and recorded no digest
	// (an older ai-rulez wrote it). generate rewrites it with a warning; clean keeps it.
	provenLegacy
	// provenWeak: a generated banner no manifest vouches for. generate rewrites it; clean keeps it.
	provenWeak
	// provenStrong: a Content-Hash, a banner the manifest lists, or the digest recorded for it.
	provenStrong
)

// outputProvenance classifies existing bytes at abs (manifest path rel). headerless
// says the output's rendering carries no header (headerlessOutput), so a missing banner
// proves nothing and only a recorded digest can vouch for an unchanged file.
func (g *Generator) outputProvenance(abs, rel string, data []byte, headerless bool) provenance {
	if stored, _, _ := g.scanHashes(abs); stored != "" {
		return provenStrong
	}
	banner := hasGeneratedBanner(abs, data)
	if !slices.Contains(g.previousManifestFiles(), rel) {
		if banner {
			return provenWeak
		}
		return provenNone
	}
	if banner {
		return provenStrong
	}
	if want, ok := g.manifestDigestSet()[rel]; ok {
		if want == fileDigest(data) {
			return provenStrong
		}
		return provenNone
	}
	if headerless {
		return provenLegacy
	}
	return provenNone
}

// refusalError names every refused file and the way out.
func refusalError(refused []outputRefusal) error {
	lines := make([]string, len(refused))
	links := false
	for i, r := range refused {
		lines[i] = fmt.Sprintf("%s (%s)", r.rel, r.reason)
		links = links || strings.HasPrefix(r.reason, "a symlink")
	}
	remedy := "Import the file with `ai-rulez convert --write`, move or delete it, or pass --force to overwrite it"
	if links {
		remedy = "generate never writes through a symlink: remove the link (or point it at a path ai-rulez generates). " +
			"For a plain file, import it with `ai-rulez convert --write`, move or delete it, or pass --force to overwrite it"
	}
	return oops.With("files", lines).
		Errorf("refusing to overwrite %d existing file(s) ai-rulez cannot prove it wrote: %s. %s",
			len(refused), strings.Join(lines, "; "), remedy)
}

// refusalReason is why generate would not write the file at relPath, when it would not.
func (g *Generator) refusalReason(relPath string) (string, bool) {
	rel := filepath.ToSlash(relPath)
	for _, r := range g.refusedOutputs {
		if r.rel == rel {
			return r.reason, true
		}
	}
	return "", false
}

// isSymlink reports whether path is itself a symbolic link.
func isSymlink(p string) bool {
	info, err := os.Lstat(p)
	return err == nil && info.Mode()&os.ModeSymlink != 0
}

// keepReason says why clean cannot show ai-rulez wrote the file at abs.
func (g *Generator) keepReason(abs string) string {
	rel := g.relSlash(abs)
	if slices.Contains(g.previousManifestFiles(), rel) {
		if _, ok := g.manifestDigestSet()[rel]; !ok {
			return "the generated manifest lists it, but it has no generated banner and no digest was recorded for it"
		}
		return "the generated manifest lists it, but it has no generated banner and matches no digest recorded for it"
	}
	return "it has no Content-Hash and is not in the generated manifest"
}

// checkOutputSafety fails when any output is refused, and remembers the links
// this run leaves alone.
func (g *Generator) checkOutputSafety(outputs []config.OutputFile) error {
	refused, linked := g.outputSafety(outputs)
	if len(refused) > 0 {
		return refusalError(refused)
	}
	g.linkedOutputs = linked
	return nil
}
