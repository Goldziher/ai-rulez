package dataset

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

// Fetch downloads every pinned file into dir and verifies its digest. A file already present with
// the right digest is kept, so a rerun is offline.
func Fetch(ctx context.Context, p *Protocol, dir string, client *http.Client) error {
	return fetchFrom(ctx, p, dir, client, "https://huggingface.co/datasets/"+p.Dataset.Repo+"/resolve/"+p.Dataset.Revision+"/")
}

func fetchFrom(ctx context.Context, p *Protocol, dir string, client *http.Client, base string) error {
	for _, f := range p.Dataset.Files {
		dst := filepath.Join(dir, filepath.FromSlash(f.Path))
		if digest(dst) == f.SHA256 {
			continue
		}
		if err := download(ctx, client, base+f.Path, dst); err != nil {
			return err
		}
		if got := digest(dst); got != f.SHA256 {
			return errors.Join(fmt.Errorf("%s: sha256 %s, the protocol pins %s", f.Path, got, f.SHA256), os.Remove(dst))
		}
	}
	return nil
}

// Verify checks the pinned files without touching the network.
func Verify(p *Protocol, dir string) error {
	for _, f := range p.Dataset.Files {
		if got := digest(filepath.Join(dir, filepath.FromSlash(f.Path))); got != f.SHA256 {
			return fmt.Errorf("%s: sha256 %q, the protocol pins %s; run fetch", f.Path, got, f.SHA256)
		}
	}
	return nil
}

func download(ctx context.Context, client *http.Client, url, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(dst), err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return fmt.Errorf("request %s: %w", url, err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("download %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: %s", url, resp.Status)
	}
	out, err := os.Create(dst) //nolint:gosec // dst is under the operator's data directory
	if err != nil {
		return fmt.Errorf("create %s: %w", dst, err)
	}
	if _, err := io.Copy(out, resp.Body); err != nil {
		return errors.Join(fmt.Errorf("write %s: %w", dst, err), out.Close())
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("close %s: %w", dst, err)
	}
	return nil
}

func digest(path string) string {
	f, err := os.Open(path) //nolint:gosec // a path under the operator's data directory
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return ""
	}
	return hex.EncodeToString(h.Sum(nil))
}
