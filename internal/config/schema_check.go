package config

import (
	"fmt"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/schema"
)

// SchemaFinding is one problem the JSON schema reports in the configuration or
// its machine-local overlay. validate fails on the first of them; generate
// reports them all as warnings, or fails with --strict.
type SchemaFinding struct {
	// File is the configuration file the finding is in.
	File string
	// Path is the table the finding is in ("" for the document root, "lint" for [lint]).
	Path string
	// Key is the unknown key, "" for any other kind of finding.
	Key string
	// Suggestion is the nearest known key of Path, "" when none is close.
	Suggestion string
	// TopLevel is set when Key is a known top-level key found inside the table
	// Path: it was written after a [table] header and so belongs to that table.
	TopLevel bool
	// Message describes a finding that is not an unknown key.
	Message string
}

// String is the one-line text printed for the finding.
func (f SchemaFinding) String() string {
	if f.Key == "" {
		return fmt.Sprintf("%s: %s", f.File, f.Message)
	}
	full := f.Key
	if f.Path != "" {
		full = f.Path + "." + f.Key
	}
	text := fmt.Sprintf("%s: unknown key %q", f.File, full)
	switch {
	case f.TopLevel:
		table := f.Path
		if i := strings.LastIndex(table, "."); i >= 0 {
			table = table[i+1:]
		}
		text += fmt.Sprintf(" (%s is a top-level key: a key after a [%s] line belongs to that table, so move it above the first [table])", f.Key, table)
	case f.Suggestion != "":
		text += fmt.Sprintf(" (did you mean %q?)", f.Suggestion)
	}
	return text
}

// SchemaFindings runs the same schema validation as `ai-rulez validate`
// (schema.ValidateFile for the configuration, schema.ValidateLocalFile for the
// machine-local overlay) and returns every finding. An error is returned only
// when a file could not be read at all.
func SchemaFindings(cfg *Config) ([]SchemaFinding, error) {
	var out []SchemaFinding
	if cfg.ConfigFile != "" {
		path := filepath.Join(cfg.ConfigDir, cfg.ConfigFile)
		found, err := findingsOf(path, schema.ValidateFile(path))
		if err != nil {
			return nil, err
		}
		out = append(out, found...)
	}
	if cfg.LocalOverlay != nil {
		path := cfg.LocalOverlay.Path
		found, err := findingsOf(path, schema.ValidateLocalFile(path))
		if err != nil {
			return nil, err
		}
		out = append(out, found...)
	}
	return out, nil
}

var (
	// "- lint.additionalProperties: Additional properties 'a', 'b' do not match the schema"
	multiUnknownRe = regexp.MustCompile(`^- (?:(.*)\.)?additionalProperties: Additional propert(?:y|ies) (.+) (?:do|does) not match the schema$`)
	// "- lint: additional property 'a' is not allowed"
	singleUnknownRe = regexp.MustCompile(`^- (.*?): additional property '([^']+)' is not allowed$`)
	quotedRe        = regexp.MustCompile(`'([^']+)'`)
	// Container lines that only repeat a more specific finding.
	noiseRe = regexp.MustCompile(`^- (?:.*\.)?(?:properties: Properties .* do not match their schemas|items: Item at index \d+ does not match the schema|additionalProperties: .*)$`)
)

// findingsOf turns the error of a schema validation into findings. A nil error
// is no finding; an error without a list of problems (a file that cannot be read)
// is returned.
func findingsOf(path string, err error) ([]SchemaFinding, error) {
	if err == nil {
		return nil, nil
	}
	oopsErr, ok := oops.AsOops(err)
	var lines []string
	if ok {
		lines, _ = oopsErr.Context()["errors"].([]string) //nolint:errcheck // absent means a non-validation error
	}
	if len(lines) == 0 {
		return nil, err //nolint:wrapcheck // already contextual
	}
	var out []SchemaFinding
	seen := map[string]bool{}
	add := func(f SchemaFinding) {
		if key := f.String(); !seen[key] {
			seen[key] = true
			out = append(out, f)
		}
	}
	for _, line := range lines {
		line = strings.TrimSpace(line)
		switch {
		case multiUnknownRe.MatchString(line):
			m := multiUnknownRe.FindStringSubmatch(line)
			for _, q := range quotedRe.FindAllStringSubmatch(m[2], -1) {
				add(unknownKey(path, m[1], q[1]))
			}
		case singleUnknownRe.MatchString(line):
			m := singleUnknownRe.FindStringSubmatch(line)
			add(unknownKey(path, m[1], m[2]))
		case noiseRe.MatchString(line):
		default:
			add(SchemaFinding{File: path, Message: strings.TrimPrefix(line, "- ")})
		}
	}
	return out, nil
}

func unknownKey(file, parent, key string) SchemaFinding {
	f := SchemaFinding{File: file, Path: parent, Key: key, Suggestion: nearestKey(key, knownKeys(parent))}
	if parent != "" && slices.Contains(knownKeys(""), key) && !slices.Contains(knownKeys(parent), key) {
		f.TopLevel = true
	}
	return f
}

// knownKeys lists the keys of the configuration table at the dotted path
// ("lint", "mcp_servers.0"), read from the struct tags of Config. It is empty
// for a path it cannot follow (a map of free-form keys).
func knownKeys(path string) []string {
	t := reflect.TypeOf(Config{})
	if path != "" {
		for _, seg := range strings.Split(path, ".") {
			t = derefType(t)
			if _, err := strconv.Atoi(seg); err == nil && t.Kind() == reflect.Slice {
				t = t.Elem()
				continue
			}
			field, ok := fieldByKey(t, seg)
			if !ok {
				return nil
			}
			t = field.Type
		}
	}
	t = derefType(t)
	for t.Kind() == reflect.Slice {
		t = derefType(t.Elem())
	}
	if t.Kind() != reflect.Struct {
		return nil
	}
	var keys []string
	for i := 0; i < t.NumField(); i++ {
		if name := tagKey(t.Field(i)); name != "" {
			keys = append(keys, name)
		}
	}
	sort.Strings(keys)
	return keys
}

func derefType(t reflect.Type) reflect.Type {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t
}

func fieldByKey(t reflect.Type, key string) (reflect.StructField, bool) {
	t = derefType(t)
	for t.Kind() == reflect.Slice {
		t = derefType(t.Elem())
	}
	if t.Kind() != reflect.Struct {
		return reflect.StructField{}, false
	}
	for i := 0; i < t.NumField(); i++ {
		if tagKey(t.Field(i)) == key {
			return t.Field(i), true
		}
	}
	return reflect.StructField{}, false
}

// tagKey is the TOML key of a struct field, "" for a field that is not a key.
func tagKey(f reflect.StructField) string {
	if !f.IsExported() {
		return ""
	}
	for _, tag := range []string{"toml", "json", "yaml"} {
		if v, ok := f.Tag.Lookup(tag); ok {
			name, _, _ := strings.Cut(v, ",")
			if name == "-" {
				return ""
			}
			if name != "" {
				return name
			}
		}
	}
	return ""
}

// nearestKey returns the candidate closest to key by edit distance, "" when none
// is within a third of the key's length (and at most three edits).
func nearestKey(key string, candidates []string) string {
	best, bestDist := "", -1
	for _, c := range candidates {
		d := editDistance(strings.ToLower(key), strings.ToLower(c))
		if bestDist < 0 || d < bestDist {
			best, bestDist = c, d
		}
	}
	limit := len(key) / 3
	if limit < 1 {
		limit = 1
	}
	if limit > 3 {
		limit = 3
	}
	if bestDist < 0 || bestDist > limit {
		return ""
	}
	return best
}

func editDistance(a, b string) int {
	prev := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur := make([]int, len(b)+1)
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(b)]
}
