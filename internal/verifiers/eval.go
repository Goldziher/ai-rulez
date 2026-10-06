package verifiers

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	"github.com/Goldziher/ai-rulez/v5/internal/llm"
	"github.com/Goldziher/ai-rulez/v5/internal/verifiers/vspec"
	"github.com/samber/oops"
)

const (
	// maxExcerpt bounds the matched text a finding quotes, in runes.
	maxExcerpt = 120
	// maxFindings bounds the findings one verifier reports.
	maxFindings = 50
)

// Finding is one place a verifier's predicate did not hold.
type Finding struct {
	File    string `json:"file,omitempty"`
	Line    int    `json:"line,omitempty"`
	Message string `json:"message"`
	// Match is the offending text, at most 120 characters, secrets masked.
	Match string `json:"match,omitempty"`
}

type evalOut struct {
	pass     bool
	findings []Finding
}

// evalCtx is the state of evaluating one spec.
type evalCtx struct {
	env    *Env
	spec   *Spec
	scoped []string
	notes  []string
}

func (c *evalCtx) note(format string, args ...any) {
	n := fmt.Sprintf(format, args...)
	for _, have := range c.notes {
		if have == n {
			return
		}
	}
	c.notes = append(c.notes, n)
}

func (c *evalCtx) eval(ctx context.Context, r *Require) (evalOut, error) {
	if err := ctx.Err(); err != nil {
		return evalOut{}, oops.Wrapf(err, "verifier canceled")
	}
	switch {
	case r.Regex != nil:
		return c.evalRegex(ctx, r.Regex, false)
	case r.Forbid != nil:
		return c.evalRegex(ctx, r.Forbid, true)
	case r.FileExists != nil:
		return c.evalFileExists(ctx, r.FileExists)
	case r.Paired != nil:
		return c.evalPaired(ctx, r.Paired)
	case r.GlobCount != nil:
		return c.evalGlobCount(r.GlobCount)
	case len(r.All) > 0:
		return c.evalAll(ctx, r.All)
	case len(r.Any) > 0:
		return c.evalAny(ctx, r.Any)
	case r.Not != nil:
		inner, err := c.eval(ctx, r.Not)
		if err != nil {
			return evalOut{}, err
		}
		if inner.pass {
			return evalOut{findings: []Finding{{Message: "a condition that must not hold is true"}}}, nil
		}
		return evalOut{pass: true}, nil
	}
	return evalOut{}, oops.Errorf("empty predicate")
}

func (c *evalCtx) evalAll(ctx context.Context, kids []Require) (evalOut, error) {
	out := evalOut{pass: true}
	for i := range kids {
		res, err := c.eval(ctx, &kids[i])
		if err != nil {
			return evalOut{}, err
		}
		if !res.pass {
			out.pass = false
			out.findings = append(out.findings, res.findings...)
		}
	}
	return out, nil
}

func (c *evalCtx) evalAny(ctx context.Context, kids []Require) (evalOut, error) {
	var all []Finding
	for i := range kids {
		res, err := c.eval(ctx, &kids[i])
		if err != nil {
			return evalOut{}, err
		}
		if res.pass {
			return evalOut{pass: true}, nil
		}
		all = append(all, res.findings...)
	}
	return evalOut{findings: append([]Finding{{Message: "none of the alternatives holds"}}, all...)}, nil
}

// content reads a scoped file for matching: LF-normalized text, or ok=false
// (with a note) for a binary or oversized file.
func (c *evalCtx) content(ctx context.Context, rel string) (data []byte, ok bool, err error) {
	data, truncated, err := c.env.readFile(ctx, rel)
	if err != nil {
		return nil, false, err
	}
	switch {
	case isBinary(data):
		c.note("skipped %s: binary file", rel)
		return nil, false, nil
	case truncated:
		c.note("skipped %s: larger than %d MiB", rel, maxFileBytes>>20)
		return nil, false, nil
	}
	return bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n")), true, nil
}

// regexFiles are the files a regex predicate examines.
func (c *evalCtx) regexFiles(p *RegexPred) ([]string, error) {
	if p.In != inAnyFile || p.Files == "" {
		return c.scoped, nil
	}
	g, err := vspec.CompileGlob(p.Files)
	if err != nil {
		return nil, oops.Wrapf(err, "invalid files glob")
	}
	var out []string
	for _, f := range c.env.scope.tree {
		if g.Match(f) && !matchesExcluded(c.spec, f) {
			out = append(out, f)
		}
	}
	return out, nil
}

func matchesExcluded(sp *Spec, f string) bool {
	excl, err := compileGlobs(sp.Exclude)
	return err == nil && matchesAny(excl, f)
}

func (c *evalCtx) evalRegex(ctx context.Context, p *RegexPred, forbid bool) (evalOut, error) {
	re, err := regexp.Compile(p.Regex)
	if err != nil {
		return evalOut{}, oops.Wrapf(err, "compile regex")
	}
	in := p.In
	if in == "" {
		in = inSameFile
	}
	files, err := c.regexFiles(p)
	if err != nil {
		return evalOut{}, err
	}
	var findings []Finding
	matchedAny := false
	for _, f := range files {
		data, ok, err := c.content(ctx, f)
		if err != nil {
			return evalOut{}, err
		}
		if !ok {
			continue
		}
		ch := c.env.scope.changed[f]
		hits := matchLines(re, data, in == inDiffAdded, ch)
		if in == inDiffAdded && !ch.AllAdded && len(ch.Added) == 0 {
			continue // nothing was added to this file
		}
		switch {
		case forbid:
			for _, h := range hits {
				findings = append(findings, Finding{File: f, Line: h.line, Message: fmt.Sprintf("forbidden pattern `%s` matched", p.Regex), Match: h.text})
			}
		case len(hits) > 0:
			matchedAny = true
		case in != inAnyFile:
			msg := fmt.Sprintf("pattern `%s` not found", p.Regex)
			if in == inDiffAdded {
				msg = fmt.Sprintf("pattern `%s` not found in the added lines", p.Regex)
			}
			findings = append(findings, Finding{File: f, Message: msg})
		}
		if len(findings) >= maxFindings {
			break
		}
	}
	if !forbid && in == inAnyFile && !matchedAny {
		findings = append(findings, Finding{Message: fmt.Sprintf("pattern `%s` not found in any of %d file(s)", p.Regex, len(files))})
	}
	return evalOut{pass: len(findings) == 0, findings: findings}, nil
}

type lineHit struct {
	line int
	text string
}

// matchLines returns the matches of re with the line they start on, only on
// added lines when added is set.
func matchLines(re *regexp.Regexp, data []byte, addedOnly bool, ch gitutil.Change) []lineHit {
	var hits []lineHit
	line, prev := 1, 0
	for _, loc := range re.FindAllIndex(data, -1) {
		line += bytes.Count(data[prev:loc[0]], []byte{'\n'})
		prev = loc[0]
		if addedOnly && !ch.AllAdded && !inRanges(ch.Added, line) {
			continue
		}
		hits = append(hits, lineHit{line: line, text: excerpt(data, loc[0], loc[1])})
		if len(hits) >= maxFindings {
			break
		}
	}
	return hits
}

func inRanges(ranges []gitutil.LineRange, line int) bool {
	for _, r := range ranges {
		if line >= r.Start && line <= r.End {
			return true
		}
	}
	return false
}

// excerpt is the matched text, cut at the line end and 120 runes, with
// credential-looking substrings masked.
func excerpt(data []byte, start, end int) string {
	text := string(data[start:end])
	if i := strings.IndexByte(text, '\n'); i >= 0 {
		text = text[:i]
	}
	text = llm.RedactSecrets(strings.TrimSpace(text))
	if utf8.RuneCountInString(text) > maxExcerpt {
		text = string([]rune(text)[:maxExcerpt]) + "..."
	}
	return sanitize(text)
}

func (c *evalCtx) evalFileExists(ctx context.Context, p *FileExistsPred) (evalOut, error) {
	want := p.Exists == nil || *p.Exists
	paths := []string{p.Path}
	subjects := []string{""}
	if hasTemplate(p.Path) {
		paths, subjects = nil, nil
		for _, f := range c.scoped {
			derived, err := expandTemplate(p.Path, f, f)
			if err != nil {
				return evalOut{}, err
			}
			paths, subjects = append(paths, derived), append(subjects, f)
		}
	}
	var findings []Finding
	for i, rel := range paths {
		_, exists, err := c.env.stat(ctx, rel)
		if err != nil {
			return evalOut{}, err
		}
		if exists == want {
			continue
		}
		msg := rel + " does not exist"
		if !want {
			msg = rel + " exists"
		}
		findings = append(findings, Finding{File: subjects[i], Message: msg})
	}
	return evalOut{pass: len(findings) == 0, findings: findings}, nil
}

func (c *evalCtx) evalGlobCount(p *GlobCountPred) (evalOut, error) {
	inc, err := vspec.CompileGlob(p.Files)
	if err != nil {
		return evalOut{}, oops.Wrapf(err, "invalid glob")
	}
	excl, err := compileGlobs(p.Exclude)
	if err != nil {
		return evalOut{}, err
	}
	var matched []string
	for _, f := range c.env.scope.tree {
		if inc.Match(f) && !matchesAny(excl, f) {
			matched = append(matched, f)
		}
	}
	n := len(matched)
	switch {
	case p.Min != nil && n < *p.Min:
		return evalOut{findings: []Finding{{Message: fmt.Sprintf("%d file(s) match %s, expected at least %d", n, p.Files, *p.Min)}}}, nil
	case p.Max != nil && n > *p.Max:
		return evalOut{findings: []Finding{{Message: fmt.Sprintf("%d file(s) match %s, expected at most %d: %s", n, p.Files, *p.Max, listFirst(matched))}}}, nil
	}
	return evalOut{pass: true}, nil
}

func (c *evalCtx) evalPaired(ctx context.Context, p *PairedPred) (evalOut, error) {
	match, err := forEachMatcherFor(p.ForEach)
	if err != nil {
		return evalOut{}, err
	}
	tmpl, byChange := p.RequiresExists, false
	if p.RequiresChanged != "" {
		tmpl, byChange = p.RequiresChanged, true
	}
	var findings []Finding
	for _, f := range c.scoped {
		rel, ok := match(f)
		if !ok {
			continue
		}
		derived, err := expandTemplate(tmpl, f, rel)
		if err != nil {
			return evalOut{}, err
		}
		var satisfied bool
		if byChange {
			ch, changed := c.env.scope.changed[derived]
			satisfied = changed && ch.Status != 'D'
		} else if _, satisfied, err = c.env.stat(ctx, derived); err != nil {
			return evalOut{}, err
		}
		if !satisfied {
			verb := "to exist"
			if byChange {
				verb = "to be changed too"
			}
			findings = append(findings, Finding{File: f, Message: fmt.Sprintf("expected %s %s", derived, verb)})
		}
	}
	return evalOut{pass: len(findings) == 0, findings: findings}, nil
}

// forEachMatcherFor builds the matcher of a for_each pattern: with {rel} a
// literal path whose {rel} captures the rest, otherwise a glob (rel is the path).
func forEachMatcherFor(pattern string) (func(string) (string, bool), error) {
	if !strings.Contains(pattern, "{rel}") {
		g, err := vspec.CompileGlob(pattern)
		if err != nil {
			return nil, oops.Wrapf(err, "invalid for_each glob")
		}
		return func(f string) (string, bool) { return f, g.Match(f) }, nil
	}
	raw := strings.Split(pattern, "{rel}")
	var sb strings.Builder
	sb.WriteString("^")
	for i, part := range raw {
		sb.WriteString(regexp.QuoteMeta(part))
		if i < len(raw)-1 {
			sb.WriteString("(.+)")
		}
	}
	sb.WriteString("$")
	re, err := regexp.Compile(sb.String())
	if err != nil {
		return nil, oops.Wrapf(err, "invalid for_each pattern")
	}
	return func(f string) (string, bool) {
		m := re.FindStringSubmatch(f)
		if m == nil {
			return "", false
		}
		return strings.Join(m[1:], "/"), true
	}, nil
}

// sortFindings orders findings by file, line and message for stable output.
func sortFindings(f []Finding) {
	sort.SliceStable(f, func(i, j int) bool {
		if f[i].File != f[j].File {
			return f[i].File < f[j].File
		}
		if f[i].Line != f[j].Line {
			return f[i].Line < f[j].Line
		}
		return f[i].Message < f[j].Message
	})
}
