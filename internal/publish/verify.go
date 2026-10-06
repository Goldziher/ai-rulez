package publish

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/samber/oops"
)

// Problem is one mismatch `publish verify` found.
type Problem struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

// VerifyResult is the outcome of verifying a dist directory.
type VerifyResult struct {
	Name     string    `json:"name,omitempty"`
	Version  string    `json:"version,omitempty"`
	Files    int       `json:"files"`
	Problems []Problem `json:"problems"`
}

// OK reports whether nothing mismatched.
func (r VerifyResult) OK() bool { return len(r.Problems) == 0 }

func (r *VerifyResult) add(path, format string, args ...any) {
	r.Problems = append(r.Problems, Problem{Path: path, Message: sprintf(format, args...)})
}

// Verify recomputes every digest of a dist directory: SHA256SUMS against the
// files, the manifest against the archive's entries, the lock copy against the
// manifest, and the archive's headers against the determinism rules. Nothing
// is written. A dist directory that cannot be read at all is an error; a
// mismatch is a Problem in the result.
func Verify(dir string) (VerifyResult, error) {
	res := VerifyResult{Problems: []Problem{}}
	sumsRaw, err := readRegular(filepath.Join(dir, SumsFile))
	if err != nil {
		return res, newError(CodeVerify, ExitFailed, "run `ai-rulez publish` first", "cannot read %s: %v", SumsFile, err)
	}
	sums, err := ParseSums(sumsRaw)
	if err != nil {
		res.add(SumsFile, "%v", err)
		return res, nil
	}
	recorded := map[string]string{}
	for _, s := range sums {
		recorded[s.Path] = s.Digest
		data, rerr := readRegular(filepath.Join(dir, filepath.FromSlash(s.Path)))
		if rerr != nil {
			res.add(s.Path, "listed in %s but unreadable: %v", SumsFile, rerr)
			continue
		}
		res.Files++
		if got := Digest(data); got != s.Digest {
			res.add(s.Path, "digest is %s, %s records %s", got, SumsFile, s.Digest)
		}
	}
	manifest, ok := loadManifest(dir, &res)
	if !ok {
		return res, nil
	}
	res.Name, res.Version = manifest.Name, manifest.Version
	checkManifest(dir, manifest, recorded, &res)
	checkPlan(dir, recorded, &res)
	sort.SliceStable(res.Problems, func(i, j int) bool { return res.Problems[i].Path < res.Problems[j].Path })
	return res, nil
}

func loadManifest(dir string, res *VerifyResult) (Manifest, bool) {
	matches, err := filepath.Glob(filepath.Join(dir, "*.manifest.json"))
	if err != nil || len(matches) != 1 {
		res.add(".", "expected exactly one *.manifest.json, found %d", len(matches))
		return Manifest{}, false
	}
	raw, err := readRegular(matches[0])
	if err != nil {
		res.add(filepath.Base(matches[0]), "unreadable: %v", err)
		return Manifest{}, false
	}
	var m Manifest
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil {
		res.add(filepath.Base(matches[0]), "invalid manifest: %v", err)
		return Manifest{}, false
	}
	if m.SchemaVersion != SchemaVersion {
		res.add(filepath.Base(matches[0]), "unsupported schema_version %d", m.SchemaVersion)
		return Manifest{}, false
	}
	if err := ValidateName(m.Name, m.Version); err != nil {
		res.add(filepath.Base(matches[0]), "%v", err)
		return Manifest{}, false
	}
	if want := m.Name + "-" + m.Version + ".manifest.json"; filepath.Base(matches[0]) != want {
		res.add(filepath.Base(matches[0]), "named for a different bundle than its content (%s)", want)
	}
	return m, true
}

func checkManifest(dir string, m Manifest, recorded map[string]string, res *VerifyResult) {
	if _, ok := recorded[m.Name+"-"+m.Version+".manifest.json"]; !ok {
		res.add(SumsFile, "does not list the manifest")
	}
	if want := m.Name + "-" + m.Version + ".tar.gz"; m.Bundle.File != want {
		res.add(m.Bundle.File, "bundle.file must be %s", want)
		return
	}
	if d, ok := recorded[m.Bundle.File]; !ok || d != m.Bundle.Digest {
		res.add(m.Bundle.File, "manifest digest %s differs from %s (%s)", m.Bundle.Digest, SumsFile, d)
	}
	if d, ok := recorded[LockFile]; !ok || d != m.Lock.FileDigest {
		res.add(LockFile, "manifest lock.file_digest %s differs from %s (%s)", m.Lock.FileDigest, SumsFile, d)
	}
	archive, err := readRegular(filepath.Join(dir, m.Bundle.File))
	if err != nil {
		return // already reported as unreadable by the SHA256SUMS pass
	}
	if len(archive) != m.Bundle.Size {
		res.add(m.Bundle.File, "size is %d, manifest records %d", len(archive), m.Bundle.Size)
	}
	entries, err := readArchive(archive)
	if err != nil {
		res.add(m.Bundle.File, "%v", err)
		return
	}
	got := map[string]FileEntry{}
	for _, e := range entries {
		got[e.Path] = e
	}
	want := map[string]FileEntry{}
	for _, f := range m.Files {
		want[f.Path] = f
	}
	for p, w := range want {
		g, ok := got[p]
		switch {
		case !ok:
			res.add(p, "in the manifest but not in the archive")
		case g.Digest != w.Digest || g.Size != w.Size:
			res.add(p, "archive holds %s (%d bytes), manifest records %s (%d bytes)", g.Digest, g.Size, w.Digest, w.Size)
		}
	}
	for p := range got {
		if _, ok := want[p]; !ok {
			res.add(p, "in the archive but not in the manifest")
		}
	}
}

func checkPlan(dir string, recorded map[string]string, res *VerifyResult) {
	raw, err := readRegular(filepath.Join(dir, PlanFile))
	if err != nil {
		return // the plan is optional for verification
	}
	var plan Plan
	if err := json.Unmarshal(raw, &plan); err != nil {
		res.add(PlanFile, "invalid plan: %v", err)
		return
	}
	for _, a := range plan.Artifacts {
		if a.Path == SumsFile {
			continue // SHA256SUMS cannot list itself
		}
		if d, ok := recorded[a.Path]; !ok || d != a.Digest {
			res.add(a.Path, "plan digest %s differs from %s (%s)", a.Digest, SumsFile, d)
		}
	}
}

// readArchive reads a bundle archive and checks the determinism rules.
func readArchive(data []byte) ([]FileEntry, error) {
	zr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, oops.Wrapf(err, "not a gzip archive")
	}
	if zr.Name != "" || zr.Comment != "" || !zr.ModTime.IsZero() && zr.ModTime.Unix() != 0 {
		return nil, oops.Errorf("gzip header carries a name, comment or time")
	}
	tr := tar.NewReader(zr)
	var out []FileEntry
	var mtime int64 = -1
	prev := ""
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return nil, oops.Wrapf(err, "read archive")
		}
		if err := checkHeader(hdr, mtime, prev); err != nil {
			return nil, err
		}
		mtime, prev = hdr.ModTime.Unix(), hdr.Name
		body, err := io.ReadAll(io.LimitReader(tr, hdr.Size+1))
		if err != nil || int64(len(body)) != hdr.Size {
			return nil, oops.Errorf("archive entry %q is truncated", hdr.Name)
		}
		out = append(out, FileEntry{Path: hdr.Name, Size: len(body), Digest: Digest(body)})
	}
}

// readRegular reads a regular file and refuses a symlink.
func readRegular(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, oops.Wrap(err)
	}
	if !info.Mode().IsRegular() {
		return nil, oops.Errorf("%s is not a regular file", filepath.Base(path))
	}
	data, err := os.ReadFile(path) //nolint:gosec // an explicit dist directory chosen by the caller
	return data, oops.Wrap(err)
}

func sprintf(format string, args ...any) string {
	if len(args) == 0 {
		return format
	}
	return strings.TrimSpace(fmt.Sprintf(format, args...))
}

// checkHeader applies the determinism rules to one entry; mtime is -1 for the first.
func checkHeader(hdr *tar.Header, mtime int64, prev string) error {
	if hdr.Typeflag != tar.TypeReg || !ValidPath(hdr.Name) {
		return oops.Errorf("archive entry %q is not a regular file with a safe path", hdr.Name)
	}
	if hdr.Uid != 0 || hdr.Gid != 0 || hdr.Uname != "" || hdr.Gname != "" ||
		(hdr.Mode != modeFile && hdr.Mode != modeExecutable) || len(hdr.PAXRecords) > 0 {
		return oops.Errorf("archive entry %q has a non-normalised owner, mode or PAX record", hdr.Name)
	}
	if mtime >= 0 && hdr.ModTime.Unix() != mtime {
		return oops.Errorf("archive entry %q has a different mtime from its siblings", hdr.Name)
	}
	if prev != "" && hdr.Name <= prev {
		return oops.Errorf("archive entry %q is out of order or duplicated", hdr.Name)
	}
	return nil
}
