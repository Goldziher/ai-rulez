package commands

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/jsondoc"
	"github.com/Goldziher/ai-rulez/v5/internal/publish"
	"github.com/samber/oops"
)

func runPublishVerify(ctx context.Context, out io.Writer, target string) error {
	if err := checkFormatFlag(publishFormat); err != nil {
		return err
	}
	trust, err := publishVerifyOptions(nil)
	if err != nil {
		return err
	}
	checks := publish.VerifyChecks{Signature: trust, RequireSignature: publishVerifyRequire}
	dir, cleanup, err := resolveVerifyTarget(ctx, target)
	if err != nil {
		return err
	}
	defer cleanup()
	results, err := verifyTree(dir, checks) //nolint:contextcheck // plan verification runs without a context
	if err != nil {
		return err //nolint:wrapcheck // a publish.Error carries the exit status
	}
	return reportVerify(out, results, target)
}

// verifyResult is one verified directory of a (possibly multi-plugin) release.
type verifyResult struct {
	Dir string `json:"dir,omitempty"`
	publish.VerifyResult
}

// verifyTree verifies dir; a multi-plugin directory verifies every plugin and
// the aggregate checksums.
func verifyTree(dir string, checks publish.VerifyChecks) ([]verifyResult, error) {
	plugins, err := os.ReadDir(filepath.Join(dir, "plugins"))
	if err != nil || fileExists(filepath.Join(dir, publish.SumsFile)) {
		res, verr := publish.VerifyWith(dir, checks)
		return []verifyResult{{VerifyResult: res}}, verr
	}
	var out []verifyResult
	for _, e := range plugins {
		if !e.IsDir() {
			continue
		}
		sub := filepath.Join("plugins", e.Name())
		res, err := publish.VerifyWith(filepath.Join(dir, sub), checks)
		if err != nil {
			return nil, err //nolint:wrapcheck // a publish.Error carries the exit status
		}
		out = append(out, verifyResult{Dir: filepath.ToSlash(sub), VerifyResult: res})
	}
	if len(out) == 0 {
		return nil, publish.Errorf(publish.CodeVerify, publish.ExitFailed, "run `ai-rulez publish` first", "%s holds no release to verify", dir)
	}
	agg := verifyResult{Dir: "aggregate"}
	if fileExists(filepath.Join(dir, "aggregate", publish.SumsFile)) {
		res, err := publish.VerifySums(filepath.Join(dir, "aggregate"))
		if err != nil {
			return nil, err //nolint:wrapcheck // a publish.Error carries the exit status
		}
		agg.VerifyResult = res
	} else {
		agg.Problems = []publish.Problem{}
	}
	// The aggregate names the plugins of the release: a plugin directory that is
	// gone (or one that was never part of it) is a mismatch, not a clean verify.
	agg.Problems = append(agg.Problems, publish.VerifyPluginList(dir)...)
	return append(out, agg), nil
}

func fileExists(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode().IsRegular()
}

// printVerifyResults prints the text report of `publish verify`: what verified
// to out, the mismatches to stderr. It prints nothing for --format json.
func printVerifyResults(out io.Writer, results []verifyResult) {
	for _, r := range results {
		label := r.Dir
		if label != "" {
			label += ": "
		}
		if r.OK() && publishFormat != formatJSON {
			if r.Name == "" {
				reportWriter{out}.printf("%sverified: %d files match SHA256SUMS\n", label, r.Files)
				continue
			}
			reportWriter{out}.printf("%sverified %s %s: %d files match SHA256SUMS, the manifest and the archive (signature: %s)\n", label, r.Name, r.Version, r.Files, verifyOrNone(r.Signature))
			if r.Signer != "" {
				reportWriter{out}.printf("%ssigner: %s\n", label, r.Signer)
			}
			continue
		}
		if publishFormat != formatJSON {
			for _, p := range r.Problems {
				fmt.Fprintf(os.Stderr, "%s%s: %s\n", label, p.Path, p.Message)
			}
		}
	}
}

func reportVerify(out io.Writer, results []verifyResult, target string) error {
	problems := 0
	for _, r := range results {
		problems += len(r.Problems)
	}
	if publishFormat == formatJSON {
		if err := jsondoc.Write(out, map[string]any{"results": results}); err != nil {
			return oops.Wrapf(err, "write result")
		}
	}
	printVerifyResults(out, results)
	if problems > 0 {
		code := publish.CodeVerify
		if signatureProblems(results) {
			code = publish.CodeUnsigned
		}
		return publish.Errorf(code, publish.ExitGate, "", "%d mismatch(es) in %s", problems, target)
	}
	return nil
}

// noneText is what the text reports show for an absent value.
const noneText = "none"

func verifyOrNone(s string) string {
	if s == "" {
		return noneText
	}
	return s
}

func signatureProblems(results []verifyResult) bool {
	for _, r := range results {
		for _, p := range r.Problems {
			if strings.Contains(p.Message, publish.CodeUnsigned) {
				return true
			}
		}
	}
	return false
}
