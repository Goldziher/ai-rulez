package importer

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/samber/oops"
)

// checkStaged writes the planned tree to a scratch project and loads and scans
// it, so a tree that does not validate or carries a secret is never written to
// the real project. installed_skills are validated field by field instead of
// loaded: loading them would fetch from the network. The security scan always
// runs, also when validation fails, and also covers config.toml, which the
// project scan does not read, with inline suppression comments ignored.
func checkStaged(ctx context.Context, report *Report, files map[string][]byte, cfg *config.Config, sc scanContext) error {
	tmp, err := os.MkdirTemp("", "ai-rulez-convert-*")
	if err != nil {
		return oops.Wrapf(err, "create scratch directory")
	}
	defer removeScratch(tmp)

	root := filepath.Join(tmp, DefaultConfigDir)
	for rel, data := range files {
		if err := writeFileAtomic(filepath.Join(root, filepath.FromSlash(rel)), data, 0o644); err != nil {
			return oops.Wrapf(err, "stage %s", rel)
		}
	}
	staged := *cfg
	staged.InstalledSkills = nil
	data, err := config.MarshalTOML(&staged)
	if err != nil {
		return oops.Wrapf(err, "render staged config")
	}
	if err := writeFileAtomic(filepath.Join(root, configTOML), data, 0o644); err != nil {
		return oops.Wrapf(err, "stage %s", configTOML)
	}

	invalid := func(err error) {
		report.Validation.Errors++
		report.Validation.Messages = append(report.Validation.Messages, firstLine(err.Error()))
	}
	if err := config.ValidateInstalledSkills(cfg.InstalledSkills); err != nil {
		invalid(err)
	}
	loaded, err := config.LoadConfigFromDir(ctx, tmp, DefaultConfigDir, config.WithoutLocal(), config.WithoutRemote())
	if err == nil {
		err = loaded.Validate()
	}
	if err != nil {
		invalid(err)
	}

	var found []lint.Finding
	if loaded != nil {
		var loader lint.Loader
		tree, lerr := loader.Load(loaded.BaseDir)
		if lerr != nil {
			return oops.Wrapf(lerr, "index scratch project")
		}
		lr, rerr := lint.RunWith(loaded, tree, lint.Options{SecurityOnly: true})
		if rerr != nil {
			return oops.Wrapf(rerr, "security scan")
		}
		found = append(found, lr.Findings...)
	}
	// The project scan honors inline ignore comments and skips config.toml;
	// converted text is not trusted to silence itself, so scan every staged text again.
	rels := make([]string, 0, len(files)+1)
	for rel := range files {
		if rel != configTOML {
			rels = append(rels, rel)
		}
	}
	sort.Strings(rels)
	for _, rel := range rels {
		if isText(files[rel]) {
			found = append(found, lint.ScanText(DefaultConfigDir+"/"+rel, string(files[rel]))...)
		}
	}
	found = append(found, lint.ScanText(DefaultConfigDir+"/"+configTOML, string(data))...)
	names := make([]string, 0, len(sc.fetched))
	for name := range sc.fetched {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		found = append(found, lint.ScanText(name, sc.fetched[name])...)
	}

	report.Security.Findings = []SecurityFinding{}
	seen := map[string]bool{}
	for _, f := range found {
		sf := SecurityFinding{Code: f.Code, Severity: string(f.Severity), File: stagedRel(f.File), Line: f.Line, Message: f.Message}
		sc.locate(&sf, files)
		key := fmt.Sprintf("%s|%s|%d|%s", sf.Code, sf.File, sf.Line, sf.Message)
		if seen[key] {
			continue
		}
		seen[key] = true
		sf.Allowed = f.Severity == lint.SeverityError && sc.allows(f.Code)
		report.Security.Findings = append(report.Security.Findings, sf)
		if f.Severity == lint.SeverityError && !sf.Allowed {
			report.Security.Blocked = true
		}
	}
	sort.SliceStable(report.Security.Findings, func(i, j int) bool {
		a, b := report.Security.Findings[i], report.Security.Findings[j]
		if a.File != b.File {
			return a.File < b.File
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		return a.Code < b.Code
	})
	if report.Security.Blocked {
		report.Security.Code = CodeBlockedScan
	}
	return nil
}

// scanContext carries what the scan needs to report where a finding came from
// and which codes the caller let through.
type scanContext struct {
	srcDir  string
	origins map[string][]string
	allow   []string
	// fetched is the text of fetched files that are referenced, not copied,
	// keyed by display name; the scan covers it like a planned file.
	fetched map[string]string
}

func (sc scanContext) allows(code string) bool {
	for _, a := range sc.allow {
		if strings.EqualFold(strings.TrimSpace(a), code) {
			return true
		}
	}
	return false
}

// locate rewrites a finding in the planned tree (.ai-rulez/rules/x.md:28) to the
// source file and line the text was read from; the planned location is kept in
// Planned. A finding in a generated file (config.toml) stays as it is.
func (sc scanContext) locate(sf *SecurityFinding, files map[string][]byte) {
	rel := strings.TrimPrefix(sf.File, DefaultConfigDir+"/")
	sources := sc.origins[rel]
	if len(sources) == 0 {
		return
	}
	planned := fmt.Sprintf("%s:%d", sf.File, sf.Line)
	if src, line, ok := sourceLocation(sc.srcDir, sources, files[rel], sf.Line); ok {
		sf.File, sf.Line, sf.Planned = src, line, planned
		return
	}
	sf.File, sf.Line, sf.Planned = sources[0], 1, planned
}

func isText(data []byte) bool {
	return utf8.Valid(data) && !bytes.Contains(data, []byte{0})
}

func stagedRel(file string) string {
	file = filepath.ToSlash(file)
	if i := strings.Index(file, "/"+DefaultConfigDir+"/"); i >= 0 {
		return file[i+1:]
	}
	return file
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
