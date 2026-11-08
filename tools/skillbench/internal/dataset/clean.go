package dataset

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// maxCleanFile bounds one file of a real skill; larger ones are data, not instructions.
const maxCleanFile = 4 << 20

var unsafeIDChars = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// FetchClean downloads each pinned repository once. GitHub does not promise stable archive bytes, so
// integrity comes from the population digest in the protocol, checked after extraction.
func FetchClean(ctx context.Context, p *Protocol, dir string, client *http.Client) error {
	return fetchCleanFrom(ctx, p, dir, client, "https://codeload.github.com/")
}

func fetchCleanFrom(ctx context.Context, p *Protocol, dir string, client *http.Client, base string) error {
	for _, r := range p.Clean.Repos {
		dst := cleanArchive(dir, r)
		if _, err := os.Stat(dst); err == nil {
			continue
		}
		if err := download(ctx, client, base+r.Repo+"/tar.gz/"+r.Commit, dst); err != nil {
			return err
		}
	}
	return nil
}

func cleanArchive(dir string, r CleanRepo) string {
	return filepath.Join(dir, "clean", strings.ReplaceAll(r.Repo, "/", "__")+"-"+r.Commit+".tar.gz")
}

// MaterializeClean writes every skill of the pinned repositories under dir, one directory per
// skill, with its regular files. Links are skipped and nothing is executable.
func MaterializeClean(p *Protocol, dataDir, track, dir string, every int) (*Materialized, error) {
	if every != 1 {
		return nil, fmt.Errorf("track %s cannot be thinned", track)
	}
	if err := refuseRepository(dir); err != nil {
		return nil, err
	}
	out := &Materialized{Track: track}
	var lines []string
	for _, r := range p.Clean.Repos {
		files, err := readArchive(cleanArchive(dataDir, r))
		if err != nil {
			return nil, err
		}
		for _, root := range skillRoots(files) {
			id := cleanID(r.Repo, root)
			for name, data := range filesUnder(files, root) {
				if err := writeCleanFile(dir, id, name, data); err != nil {
					return nil, err
				}
				sum := sha256.Sum256(data)
				lines = append(lines, id+" "+name+" "+hex.EncodeToString(sum[:]))
			}
			out.Samples = append(out.Samples, Sample{ID: id, Source: r.Repo, Evidence: "vendor", Cluster: id})
		}
	}
	slices.SortFunc(out.Samples, func(a, b Sample) int { return strings.Compare(a.ID, b.ID) })
	slices.Sort(lines)
	sum := sha256.Sum256([]byte(strings.Join(lines, "\n")))
	out.Digest = hex.EncodeToString(sum[:])
	if p.Clean.Population != "" && out.Digest != p.Clean.Population {
		return nil, fmt.Errorf("clean population is %s, the protocol pins %s", out.Digest, p.Clean.Population)
	}
	return out, nil
}

// readArchive returns the regular files of a repository archive by path, without its top directory.
func readArchive(file string) (map[string][]byte, error) {
	f, err := os.Open(file) //nolint:gosec // a path under the operator's data directory
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", file, err)
	}
	defer func() { _ = f.Close() }() //nolint:errcheck // read-only
	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", file, err)
	}
	files := map[string][]byte{}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return files, nil
		}
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", file, err)
		}
		_, name, ok := strings.Cut(h.Name, "/")
		if !ok || h.Typeflag != tar.TypeReg || h.Size > maxCleanFile || !safeRelative(name) {
			continue
		}
		data, err := io.ReadAll(tr)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", file, err)
		}
		files[name] = data
	}
}

func safeRelative(name string) bool {
	return name != "" && filepath.IsLocal(filepath.FromSlash(name))
}

// skillRoots lists the directories holding a SKILL.md, sorted.
func skillRoots(files map[string][]byte) []string {
	var roots []string
	for name := range files {
		if path.Base(name) == "SKILL.md" {
			roots = append(roots, path.Dir(name))
		}
	}
	slices.Sort(roots)
	return roots
}

// filesUnder returns the files of a skill relative to its root, leaving out nested skills.
func filesUnder(files map[string][]byte, root string) map[string][]byte {
	out := map[string][]byte{}
	prefix := root + "/"
	if root == "." {
		prefix = ""
	}
	for name, data := range files {
		if rel, ok := strings.CutPrefix(name, prefix); ok && (!strings.Contains(rel, "/SKILL.md") || rel == "SKILL.md") {
			out[rel] = data
		}
	}
	return out
}

func cleanID(repo, root string) string {
	return unsafeIDChars.ReplaceAllString(strings.ReplaceAll(repo+"/"+root, "/", "__"), "_")
}

func writeCleanFile(dir, id, name string, data []byte) error {
	dst := filepath.Join(dir, id, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(dst), err)
	}
	if err := os.WriteFile(dst, data, 0o440); err != nil { //nolint:gosec // non-executable by design
		return fmt.Errorf("write %s: %w", dst, err)
	}
	return nil
}
