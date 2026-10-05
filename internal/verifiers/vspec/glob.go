// Package vspec holds the pure parsers shared by the verifier config validator
// and the verifier runner: glob patterns and key_equals key paths. It imports
// nothing from the rest of the project so both sides can use it.
package vspec

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// MaxBraceAlternatives caps how many alternatives the {a,b} groups of one glob
// may expand to. Groups multiply (`{a,b}{c,d}` is four), so an unbounded
// expansion is a way to make a config exhaust memory.
const MaxBraceAlternatives = 64

// Glob matches slash-separated repo-relative paths. ** crosses directories, *
// and ? do not, {a,b} alternates, and a pattern without a slash matches at any
// depth (like .gitignore). A pattern ending in "/" matches everything below it.
type Glob struct{ re *regexp.Regexp }

// CompileGlob compiles a glob. It fails when the brace groups expand to more
// than MaxBraceAlternatives alternatives.
func CompileGlob(pattern string) (Glob, error) {
	p := strings.TrimPrefix(strings.TrimSpace(pattern), "./")
	p = strings.TrimPrefix(p, "/")
	if strings.HasSuffix(p, "/") {
		p += "**"
	}
	alts, err := expandBraces(p, MaxBraceAlternatives)
	if err != nil {
		return Glob{}, err
	}
	for i, a := range alts {
		alts[i] = globBody(a)
	}
	re, err := regexp.Compile("^(?:" + strings.Join(alts, "|") + ")$")
	if err != nil {
		return Glob{}, fmt.Errorf("compile glob: %w", err)
	}
	return Glob{re: re}, nil
}

// Match reports whether the slash-separated path matches.
func (g Glob) Match(p string) bool { return g.re.MatchString(p) }

func globBody(p string) string {
	var sb strings.Builder
	if !strings.Contains(p, "/") {
		sb.WriteString("(?:.*/)?")
	}
	for i := 0; i < len(p); i++ {
		switch c := p[i]; c {
		case '*':
			switch {
			case i+2 < len(p) && p[i+1] == '*' && p[i+2] == '/':
				i += 2
				sb.WriteString("(?:.*/)?")
			case i+1 < len(p) && p[i+1] == '*':
				i++
				sb.WriteString(".*")
			default:
				sb.WriteString("[^/]*")
			}
		case '?':
			sb.WriteString("[^/]")
		default:
			sb.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	return sb.String()
}

var errTooManyAlternatives = fmt.Errorf("brace groups expand to more than %d alternatives", MaxBraceAlternatives)

// expandBraces expands the first {a,b} group recursively, failing as soon as
// the result would exceed limit alternatives.
func expandBraces(p string, limit int) ([]string, error) {
	open := strings.IndexByte(p, '{')
	if open < 0 {
		return []string{p}, nil
	}
	depth, closeIdx := 0, -1
	var cuts []int
	for i := open; i < len(p) && closeIdx < 0; i++ {
		switch p[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				closeIdx = i
			}
		case ',':
			if depth == 1 {
				cuts = append(cuts, i)
			}
		}
	}
	if closeIdx < 0 {
		return []string{p}, nil
	}
	prefix, suffix := p[:open], p[closeIdx+1:]
	var out []string
	start := open + 1
	for _, cut := range append(cuts, closeIdx) {
		tails, err := expandBraces(p[start:cut]+suffix, limit)
		if err != nil {
			return nil, err
		}
		for _, tail := range tails {
			out = append(out, prefix+tail)
			if len(out) > limit {
				return nil, errTooManyAlternatives
			}
		}
		start = cut + 1
	}
	return out, nil
}

// ParseKey splits a key_equals key into path segments. Segments are separated
// by dots; a backslash escapes the next character (`lodash\.merge`, `a\\b`); a
// bracketed quoted segment takes its content literally (`["lodash.merge"]`,
// with `\"` and `\\` as the only escapes); a bracketed integer (`[0]`) is a
// list index and equals a numeric segment (`.0`).
func ParseKey(key string) ([]string, error) {
	if key == "" {
		return nil, errors.New("key is empty")
	}
	var segs []string
	var cur strings.Builder
	have, afterDot, afterBracket := false, false, false
	for i := 0; i < len(key); i++ {
		c := key[i]
		switch {
		case c == '.':
			if !have && !afterBracket {
				return nil, fmt.Errorf("key %q has an empty segment", key)
			}
			if have {
				segs = append(segs, cur.String())
				cur.Reset()
			}
			have, afterDot, afterBracket = false, true, false
		case c == '[':
			if have {
				segs = append(segs, cur.String())
				cur.Reset()
				have = false
			} else if afterBracket && !afterDot {
				// "[0][1]": adjacent brackets are fine.
				afterBracket = false
			}
			seg, next, err := parseBracket(key, i)
			if err != nil {
				return nil, fmt.Errorf("key %q: %w", key, err)
			}
			segs = append(segs, seg)
			i = next
			afterDot, afterBracket = false, true
		default:
			if afterBracket {
				return nil, fmt.Errorf("key %q: unexpected %q after ']'", key, c)
			}
			if c == '\\' {
				i++
				if i >= len(key) {
					return nil, fmt.Errorf("key %q ends with a backslash", key)
				}
				c = key[i]
			}
			cur.WriteByte(c)
			have, afterDot = true, false
		}
	}
	if have {
		segs = append(segs, cur.String())
	} else if afterDot {
		return nil, fmt.Errorf("key %q has an empty segment", key)
	}
	return segs, nil
}

// parseBracket reads `["..."]` or `[123]` starting at key[i] == '[' and returns
// the segment and the index of the closing ']'.
func parseBracket(key string, i int) (string, int, error) {
	j := i + 1
	if j < len(key) && key[j] == '"' {
		var sb strings.Builder
		for j++; j < len(key); j++ {
			switch key[j] {
			case '\\':
				j++
				if j >= len(key) {
					return "", 0, errors.New("unterminated quoted segment")
				}
				sb.WriteByte(key[j])
			case '"':
				if j+1 < len(key) && key[j+1] == ']' {
					return sb.String(), j + 1, nil
				}
				return "", 0, errors.New(`closing quote must be followed by ']'`)
			default:
				sb.WriteByte(key[j])
			}
		}
		return "", 0, errors.New("unterminated quoted segment")
	}
	end := strings.IndexByte(key[j:], ']')
	if end < 0 {
		return "", 0, errors.New("unterminated '['")
	}
	idx := key[j : j+end]
	if idx == "" || strings.Trim(idx, "0123456789") != "" {
		return "", 0, fmt.Errorf("[%s] is neither a list index nor a quoted key", idx)
	}
	return idx, j + end, nil
}
