package generator

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
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
	path := filepath.Join(configDir, ConvertRecordName)
	rec := convertRecord{Version: "1", Files: map[string]string{}}
	if data, err := os.ReadFile(path); err == nil {
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
	return config.WriteFileAtomic(path, append(out, '\n'), 0o644)
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

// outputSafety classifies the outputs a project-scope run is about to write.
// refused are files generate will not touch; linked are outputs that are
// symlinks onto another path this run generates (the target gets its own
// content, the link stays as the user made it and is not recorded as ours).
func (g *Generator) outputSafety(outputs []config.OutputFile) (refused []outputRefusal, linked map[string]bool) {
	if g.userMode {
		return g.userOutputSafety(outputs), nil
	}
	linked = map[string]bool{}
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
	for _, output := range outputs {
		if output.IsDir {
			continue
		}
		abs := g.absOutputPath(output.Path)
		rel := g.relSlash(abs)
		info, err := os.Lstat(abs)
		if err != nil {
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 {
			resolved, _, rerr := resolveWriteTarget(abs, new(int))
			if rerr == nil && written[resolved] {
				linked[rel] = true
				continue
			}
			shown := g.displayTarget(resolved)
			reason := fmt.Sprintf(reasonLinkFmt, shown)
			if filepath.IsAbs(shown) {
				reason = fmt.Sprintf("a symlink to %s, outside the project", shown)
			}
			refused = append(refused, outputRefusal{rel, reason})
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
	if output.RawContent == nil {
		if stored, _, _ := g.scanHashes(abs); stored != "" || hasGeneratedBanner(abs, data) {
			return false
		}
	}
	return !slices.Contains(g.previousManifestFiles(), rel)
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

// blockedLines is the dry-run form of the refusals.
func blockedLines(refused []outputRefusal) []string {
	lines := make([]string, len(refused))
	for i, r := range refused {
		lines[i] = fmt.Sprintf("blocked: %s (%s)", r.rel, r.reason)
	}
	return lines
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
func isSymlink(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode()&os.ModeSymlink != 0
}

// keepReason says why clean cannot show ai-rulez wrote the file at abs.
func (g *Generator) keepReason(abs string) string {
	if slices.Contains(g.previousManifestFiles(), g.relSlash(abs)) {
		return "the generated manifest lists it, but it has no generated banner and matches no digest recorded for it"
	}
	return "it has no Content-Hash and is not in the generated manifest"
}
