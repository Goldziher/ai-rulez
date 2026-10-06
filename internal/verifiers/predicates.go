package verifiers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/verifiers/vspec"
	"github.com/pelletier/go-toml/v2"
	"github.com/samber/oops"
	"gopkg.in/yaml.v3"
)

const (
	// maxFileBytes bounds how much of a file a content predicate reads. A file
	// larger than this is only partly checked, which regex and forbid report as
	// an error rather than pass on the strength of a prefix.
	maxFileBytes = 5 << 20
	// maxReported bounds the file:line locations a failure message lists.
	maxReported = 10
)

// skipDirs are never walked: VCS metadata and dependency trees. Use `exclude`
// to drop more.
var skipDirs = map[string]bool{
	".git": true, ".hg": true, ".svn": true, "node_modules": true, ".venv": true, "__pycache__": true,
}

func predFileExists(ctx context.Context, env *Env, v config.VerifierConfig) (Outcome, error) {
	_, exists, err := env.stat(ctx, v.Path)
	if err != nil {
		return Outcome{}, err
	}
	if !exists {
		return Outcome{Message: v.Path + " does not exist", Findings: []Finding{{File: v.Path, Message: "file does not exist"}}}, nil
	}
	return Outcome{Pass: true}, nil
}

func predFileAbsent(ctx context.Context, env *Env, v config.VerifierConfig) (Outcome, error) {
	_, exists, err := env.stat(ctx, v.Path)
	if err != nil {
		return Outcome{}, err
	}
	if exists {
		return Outcome{Message: v.Path + " exists", Findings: []Finding{{File: v.Path, Message: "file exists but must not"}}}, nil
	}
	return Outcome{Pass: true}, nil
}

func predGlobCount(ctx context.Context, env *Env, v config.VerifierConfig) (Outcome, error) {
	matched, incomplete, err := env.match(ctx, v)
	if err != nil {
		return Outcome{}, err
	}
	if incomplete != "" {
		return Outcome{Message: incomplete}, nil
	}
	n := len(matched)
	if v.Min != nil && n < *v.Min {
		return Outcome{Message: fmt.Sprintf("%d file(s) match %s, expected at least %d", n, v.Glob, *v.Min)}, nil
	}
	if v.Max != nil && n > *v.Max {
		return Outcome{Message: fmt.Sprintf("%d file(s) match %s, expected at most %d: %s", n, v.Glob, *v.Max, listFirst(matched)),
			Findings: fileFindings(matched, "matches "+v.Glob+" but more files match than allowed")}, nil
	}
	return Outcome{Pass: true, Message: fmt.Sprintf("%d file(s)", n)}, nil
}

// unchecked describes files a content predicate could not examine fully.
func unchecked(files []string) error {
	return oops.Hint("Narrow `glob` or add the file to `exclude`.").
		Errorf("%d file(s) could not be fully checked (binary or over %d MiB): %s",
			len(files), maxFileBytes>>20, listFirst(files))
}

func predRegex(ctx context.Context, env *Env, v config.VerifierConfig) (Outcome, error) {
	re, err := regexp.Compile(v.Pattern)
	if err != nil {
		return Outcome{}, oops.Wrapf(err, "compile pattern")
	}
	files, incomplete, err := env.match(ctx, v)
	if err != nil {
		return Outcome{}, err
	}
	if incomplete != "" {
		return Outcome{Message: incomplete}, nil
	}
	if len(files) == 0 {
		return Outcome{Message: "no files match " + v.Glob}, nil
	}
	var missing, skipped []string
	checked := 0
	for _, f := range files {
		if err := ctx.Err(); err != nil {
			return Outcome{}, oops.Wrapf(err, "verifier canceled")
		}
		data, truncated, err := env.readFile(ctx, f)
		if err != nil {
			return Outcome{}, err
		}
		switch {
		case isBinary(data):
			skipped = append(skipped, f)
		case re.Match(data):
			checked++
		case truncated:
			// Not found in the prefix, but the pattern could be past it.
			skipped = append(skipped, f)
		default:
			checked++
			missing = append(missing, f)
		}
	}
	if len(missing) > 0 {
		return Outcome{Message: fmt.Sprintf("pattern %q not found in %s", v.Pattern, listFirst(missing)),
			Findings: fileFindings(missing, fmt.Sprintf("pattern %q not found", v.Pattern))}, nil
	}
	if len(skipped) > 0 {
		return Outcome{}, unchecked(skipped)
	}
	return Outcome{Pass: true, Message: fmt.Sprintf("%d file(s)", checked)}, nil
}

func predForbid(ctx context.Context, env *Env, v config.VerifierConfig) (Outcome, error) {
	re, err := regexp.Compile(v.Pattern)
	if err != nil {
		return Outcome{}, oops.Wrapf(err, "compile pattern")
	}
	files, incomplete, err := env.match(ctx, v)
	if err != nil {
		return Outcome{}, err
	}
	if incomplete != "" {
		return Outcome{Message: incomplete}, nil
	}
	// Collect one more hit than is reported so the message can say "and more"
	// without counting every match, and stop as soon as that many are found.
	const hitCap = maxReported + 1
	var hits, skipped []string
	var hitFindings []Finding
	checked := 0
scan:
	for _, f := range files {
		if err := ctx.Err(); err != nil {
			return Outcome{}, oops.Wrapf(err, "verifier canceled")
		}
		data, truncated, err := env.readFile(ctx, f)
		if err != nil {
			return Outcome{}, err
		}
		if isBinary(data) {
			skipped = append(skipped, f)
			continue
		}
		line, prev := 1, 0
		for _, loc := range re.FindAllIndex(data, hitCap-len(hits)) {
			line += bytes.Count(data[prev:loc[0]], []byte{'\n'})
			prev = loc[0]
			hits = append(hits, fmt.Sprintf("%s:%d", f, line))
			hitFindings = append(hitFindings, Finding{File: f, Line: line, Message: fmt.Sprintf("forbidden pattern %q", v.Pattern),
				Match: excerpt(data, loc[0], loc[1])})
		}
		if len(hits) >= hitCap {
			break scan
		}
		if truncated {
			skipped = append(skipped, f)
			continue
		}
		checked++
	}
	if len(hits) > 0 {
		msg := fmt.Sprintf("forbidden pattern %q at %s", v.Pattern, listCapped(hits))
		if len(skipped) > 0 {
			msg += fmt.Sprintf("; %d file(s) were not fully checked (binary or over %d MiB)", len(skipped), maxFileBytes>>20)
		}
		return Outcome{Message: msg, Findings: hitFindings[:min(len(hitFindings), maxReported)]}, nil
	}
	if len(skipped) > 0 {
		return Outcome{}, unchecked(skipped)
	}
	return Outcome{Pass: true, Message: fmt.Sprintf("%d file(s)", checked)}, nil
}

// fileFindings makes one finding per file, up to the reporting cap.
func fileFindings(files []string, msg string) []Finding {
	out := make([]Finding, 0, min(len(files), maxReported))
	for _, f := range files[:min(len(files), maxReported)] {
		out = append(out, Finding{File: f, Message: msg})
	}
	return out
}

func isBinary(data []byte) bool { return bytes.IndexByte(data, 0) >= 0 }

func predKeyEquals(ctx context.Context, env *Env, v config.VerifierConfig) (Outcome, error) {
	var decode func([]byte, any) error
	switch strings.ToLower(filepath.Ext(v.Path)) {
	case ".json":
		decode = decodeJSON
	case ".yaml", ".yml":
		decode = yaml.Unmarshal
	case ".toml":
		decode = toml.Unmarshal
	default:
		return keyFail(v.Path, fmt.Sprintf("%s: unsupported format (use .json, .yaml, .yml or .toml)", v.Path)), nil
	}
	segs, err := vspec.ParseKey(v.Key)
	if err != nil {
		return Outcome{}, oops.Wrapf(err, "invalid key")
	}
	_, exists, err := env.stat(ctx, v.Path)
	if err != nil {
		return Outcome{}, err
	}
	if !exists {
		return Outcome{Message: v.Path + " does not exist"}, nil
	}
	data, truncated, err := env.readFile(ctx, v.Path)
	if err != nil {
		return Outcome{}, err
	}
	if truncated {
		return Outcome{}, oops.Errorf("%s is over %d MiB and cannot be parsed", v.Path, maxFileBytes>>20)
	}
	var doc any
	if err := decode(data, &doc); err != nil {
		return keyFail(v.Path, fmt.Sprintf("%s does not parse: %v", v.Path, err)), nil
	}
	val, found := lookupKey(doc, segs)
	if !found {
		return keyFail(v.Path, fmt.Sprintf("%s has no key %s", v.Path, v.Key)), nil
	}
	if val == nil {
		return keyFail(v.Path, fmt.Sprintf("%s: %s is null, expected %q", v.Path, v.Key, derefString(v.Equals))), nil
	}
	got, scalar := scalarString(val)
	if !scalar {
		return keyFail(v.Path, fmt.Sprintf("%s: %s is not a scalar value", v.Path, v.Key)), nil
	}
	if want := derefString(v.Equals); got != want {
		return keyFail(v.Path, fmt.Sprintf("%s: %s is %q, expected %q", v.Path, v.Key, got, want)), nil
	}
	return Outcome{Pass: true}, nil
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// decodeJSON keeps numbers as written (json.Number) so large integers and
// "1.0" compare exactly instead of through float64.
func decodeJSON(data []byte, out any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err := dec.Decode(out); err != nil {
		return err //nolint:wrapcheck // reported verbatim in the failure message
	}
	return nil
}

// DefaultDrift lists the generated files that differ from a fresh render of
// profile, as "kind: path". It is what generated_in_sync uses unless
// Options.Drift replaces it, and it matches `generate --check`: includes and
// installed skills are resolved as the loaded config has them.
func DefaultDrift(cfg *config.Config, profile string) ([]string, error) {
	return defaultDrift(cfg, profile)
}

func predGeneratedInSync(_ context.Context, env *Env, v config.VerifierConfig) (Outcome, error) {
	drift := env.opts.Drift
	if drift == nil {
		drift = DefaultDrift
	}
	files, err := drift(env.Cfg, v.Profile)
	if err != nil {
		return Outcome{}, oops.Wrapf(err, "render generated outputs")
	}
	if len(files) > 0 {
		findings := make([]Finding, 0, len(files))
		for _, f := range files[:min(len(files), maxReported)] {
			_, p, ok := strings.Cut(f, ": ")
			if !ok {
				p = f
			}
			findings = append(findings, Finding{File: p, Message: "generated file differs from its sources"})
		}
		return Outcome{Message: "generated files differ from their sources: " + listFirst(files) + " (run `ai-rulez generate`)", Findings: findings}, nil
	}
	return Outcome{Pass: true}, nil
}

func listFirst(items []string) string {
	if len(items) <= maxReported {
		return strings.Join(items, ", ")
	}
	return fmt.Sprintf("%s and %d more", strings.Join(items[:maxReported], ", "), len(items)-maxReported)
}

// listCapped lists items collected up to maxReported+1: more than maxReported
// means more may exist, and how many is unknown.
func listCapped(items []string) string {
	if len(items) <= maxReported {
		return strings.Join(items, ", ")
	}
	return strings.Join(items[:maxReported], ", ") + " and more"
}

// keyFail is a failed key_equals outcome located at the file.
func keyFail(path, msg string) Outcome {
	return Outcome{Message: msg, Findings: []Finding{{File: path, Message: "key check failed"}}}
}
