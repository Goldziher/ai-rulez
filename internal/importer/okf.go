package importer

import (
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/okf"
	"github.com/Goldziher/ai-rulez/v5/internal/okfbridge"
	"github.com/samber/oops"
)

// The okf importer is `convert --from okf`: an OKF bundle (docs/okf.md) becomes
// an .ai-rulez/ tree through the same mapping as `ai-rulez import okf`, but its
// result goes through convert's plan, report, scan and write, so there is one
// report format and one set of safety checks.

const (
	okfName = "okf"
	// okfDefaultDir is where `export okf` puts a bundle ([okf] dir).
	okfDefaultDir = "docs/okf"
)

type okfImporter struct{}

func (okfImporter) Name() string { return okfName }

func (okfImporter) Description() string {
	return "OKF bundle (Open Knowledge Format): an index.md with okf_version at the source root or in docs/okf"
}

// bundleRoot returns the directory of the bundle inside the source, "" when there
// is none. A bundle is recognised by its root index.md naming okf_version.
func bundleRoot(r *reader) string {
	for _, dir := range []string{".", okfDefaultDir} {
		index := path.Join(dir, okf.IndexFile)
		if _, ok := r.exists(index); !ok {
			continue
		}
		data, err := r.read(index)
		if err == nil && strings.Contains(string(data), "okf_version") {
			return dir
		}
	}
	return ""
}

func (okfImporter) Detect(fsys fs.FS) []string {
	r := newReader(fsys)
	if dir := bundleRoot(r); dir != "" {
		return []string{path.Join(dir, okf.IndexFile)}
	}
	return nil
}

func (okfImporter) Plan(fsys fs.FS, opt Options) (*Plan, error) {
	r := newReader(fsys)
	dir := bundleRoot(r)
	if dir == "" {
		return nil, oops.Hint("Pass the directory that holds the bundle's index.md").
			Errorf("no OKF bundle found: expected an index.md naming okf_version at the source root or in %s", okfDefaultDir)
	}
	root := fsys
	if dir != "." {
		sub, err := fs.Sub(fsys, dir)
		if err != nil {
			return nil, oops.With("path", dir).Wrapf(err, "open the bundle %s", dir)
		}
		root = sub
	}
	bundle, err := okf.Load(root)
	if err != nil {
		return nil, oops.Wrapf(err, "read the OKF bundle (%s)", CodeInvalid)
	}

	// The bridge writes the planned tree to disk; give it a scratch directory and
	// read the result back, so convert decides what is written where.
	scratch, err := os.MkdirTemp("", "ai-rulez-okf-*")
	if err != nil {
		return nil, oops.Wrapf(err, "create scratch directory")
	}
	defer removeScratch(scratch)
	res, err := okfbridge.Import(bundle, okfbridge.ImportOptions{ConfigDir: scratch, Domain: opt.Domain})
	if err != nil {
		return nil, oops.Wrapf(err, "import the OKF bundle")
	}
	return planFromBridge(dir, scratch, res)
}

// planFromBridge reads the tree the bridge wrote and the findings it produced.
func planFromBridge(bundleDir, scratch string, res *okfbridge.ImportResult) (*Plan, error) {
	p := &Plan{}
	sources := map[string]string{} // planned path -> bundle path (relative to the source)
	for _, a := range res.Actions {
		sources[a.Path] = path.Join(bundleDir, a.Source)
	}
	var rels []string
	err := filepath.WalkDir(scratch, func(file string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, rerr := filepath.Rel(scratch, file)
		if rerr != nil {
			return rerr
		}
		rels = append(rels, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, oops.Wrapf(err, "read the planned tree")
	}
	sort.Strings(rels)
	for _, rel := range rels {
		data, rerr := os.ReadFile(filepath.Join(scratch, filepath.FromSlash(rel)))
		if rerr != nil {
			return nil, oops.With("path", rel).Wrapf(rerr, "read the planned file")
		}
		src := sources[rel]
		if src == "" {
			src = bundleDir
		}
		exec := false
		// Lstat, regular files only: a link or a device is never an executable script.
		if info, serr := os.Lstat(filepath.Join(scratch, filepath.FromSlash(rel))); serr == nil {
			exec = info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0
		}
		p.Raw = append(p.Raw, RawFile{Path: rel, Data: data, Sources: []string{src}, Exec: exec})
		p.add(newFinding(StatusMapped, src, "concept", rel, ""))
	}
	for _, f := range res.Findings {
		status := StatusApproximated
		if f.Severity == okf.SeverityError {
			status = StatusUnsupported
		}
		p.add(newFinding(status, path.Join(bundleDir, f.Path), f.Code, "", f.Message))
	}
	for _, s := range res.Skipped {
		file, why, _ := strings.Cut(s, ": ")
		p.add(newFinding(StatusDropped, path.Join(bundleDir, file), "", "", why))
	}
	return p, nil
}
