package sbom

import (
	"crypto/sha1" //nolint:gosec // SPDX 2.3 requires a SHA-1 checksum on every file; it is a schema field, not a security hash
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/contentlock"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
)

// contentIndex finds the loaded file behind a lock item.
type contentIndex struct {
	configDir string
	files     map[string]*config.ContentFile // kind + "\x00" + item path
}

func newContentIndex(cfg *config.Config) *contentIndex {
	idx := &contentIndex{configDir: cfg.ConfigDir, files: map[string]*config.ContentFile{}}
	if cfg.Content == nil {
		return idx
	}
	idx.addTree(cfg.Content.Rules, cfg.Content.Context, cfg.Content.Skills, cfg.Content.Agents, cfg.Content.Commands, cfg.Content.Checks)
	for _, d := range cfg.Content.Domains {
		if d == nil || d.FromInclude || d.Builtin {
			continue
		}
		idx.addTree(d.Rules, d.Context, d.Skills, d.Agents, d.Commands, d.Checks)
	}
	return idx
}

func (x *contentIndex) addTree(rules, contexts, skills, agents, commands, checks []config.ContentFile) {
	for _, g := range []struct {
		kind  string
		files []config.ContentFile
	}{{contentlock.KindRule, rules}, {contentlock.KindContext, contexts}, {contentlock.KindSkill, skills},
		{contentlock.KindAgent, agents}, {contentlock.KindCommand, commands}, {contentlock.KindCheck, checks}} {
		for i := range g.files {
			cf := &g.files[i]
			rel, ok := x.rel(cf.Path)
			if !ok {
				continue
			}
			if g.kind == contentlock.KindSkill || len(cf.Resources) > 0 {
				rel = dirOf(rel)
			}
			x.files[g.kind+"\x00"+rel] = cf
		}
	}
}

// rel is p relative to the configuration directory, "/"-separated; ok is false
// outside it.
func (x *contentIndex) rel(p string) (string, bool) {
	if p == "" {
		return "", false
	}
	if x.configDir == "" {
		return filepath.ToSlash(p), true
	}
	r, err := filepath.Rel(x.configDir, p)
	if err != nil || r == ".." || strings.HasPrefix(r, ".."+string(filepath.Separator)) {
		return "", false
	}
	return filepath.ToSlash(r), true
}

func dirOf(p string) string {
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[:i]
	}
	return p
}

func (x *contentIndex) find(it *lockfile.Item) *config.ContentFile {
	return x.files[it.Kind+"\x00"+it.Path]
}

// entries lists the files of cf, sorted by path. With onlyExecutable it keeps
// just the scripts (files under scripts/ and files with an executable bit) and
// leaves them unhashed, so a default document does not change with the line
// endings of a checkout. Otherwise every file is hashed (SHA-256 and the SHA-1
// SPDX requires) from the bytes on disk, the resources as loaded when a file has
// vanished.
func (x *contentIndex) entries(cf *config.ContentFile, onlyExecutable bool) []fileEntry {
	var out []fileEntry
	add := func(abs string, fallback []byte, mode os.FileMode) {
		rel, ok := x.rel(abs)
		if !ok {
			return
		}
		info, statErr := os.Stat(abs)
		if statErr == nil {
			mode = info.Mode()
		} else if fallback == nil {
			return
		}
		e := fileEntry{path: rel, executes: mode.Perm()&0o111 != 0 || isScript(rel)}
		if onlyExecutable {
			if e.executes {
				out = append(out, e)
			}
			return
		}
		data := fallback
		if disk, err := os.ReadFile(abs); err == nil {
			data = disk
		}
		s256 := sha256.Sum256(data)
		s1 := sha1.Sum(data) //nolint:gosec // see import
		e.hashed, e.size = true, int64(len(data))
		e.sha256, e.sha1 = hex.EncodeToString(s256[:]), hex.EncodeToString(s1[:])
		out = append(out, e)
	}
	add(cf.Path, []byte(cf.Content), 0)
	dir := filepath.Dir(cf.Path)
	for _, res := range cf.Resources {
		add(filepath.Join(dir, filepath.FromSlash(res.RelPath)), res.Content, res.Mode)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].path < out[j].path })
	return out
}

func isScript(rel string) bool {
	return strings.HasPrefix(rel, "scripts/") || strings.Contains(rel, "/scripts/")
}

// fileComponents are the CycloneDX file components of entries, nested under the
// item whose ref is parent.
func fileComponents(parent string, entries []fileEntry) []Component {
	if len(entries) == 0 {
		return nil
	}
	out := make([]Component, 0, len(entries))
	for _, e := range entries {
		props := []Property{prop("kind", "file")}
		if e.executes {
			props = append(props, prop("executes", "true"))
		}
		comp := Component{Type: "file", BOMRef: parent + ":file:" + e.path, Name: e.path}
		if e.hashed {
			props = append(props, prop("size", itoa(e.size)))
			comp.Hashes = []Hash{{Alg: "SHA-256", Content: e.sha256}}
		}
		comp.Properties = sortProps(props)
		out = append(out, comp)
	}
	return out
}
