package verifiers

import (
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/samber/oops"
)

var templateVarRe = regexp.MustCompile(`\{[a-z]+\}`)

var templateVars = map[string]bool{"{path}": true, "{dir}": true, "{stem}": true, "{ext}": true, "{rel}": true}

func itoa(n int) string { return strconv.Itoa(n) }

func hasTemplate(s string) bool { return templateVarRe.MatchString(s) }

// validateTemplate rejects an unknown {variable}.
func validateTemplate(field, tmpl string) string {
	for _, v := range templateVarRe.FindAllString(tmpl, -1) {
		if !templateVars[v] {
			return field + ": unknown template variable " + v + " (use {path}, {dir}, {stem}, {ext}, {rel})"
		}
	}
	return ""
}

// expandTemplate derives a path from a scoped file. {ext} includes the dot;
// {dir} is "." for a top-level file. The result must stay inside the project.
func expandTemplate(tmpl, file, rel string) (string, error) {
	base := path.Base(file)
	ext := path.Ext(base)
	repl := strings.NewReplacer(
		"{path}", file,
		"{dir}", path.Dir(file),
		"{stem}", strings.TrimSuffix(base, ext),
		"{ext}", ext,
		"{rel}", rel,
	)
	out := repl.Replace(tmpl)
	if strings.ContainsRune(out, 0) {
		return "", oops.Errorf("template %q expands to a path with a NUL byte for %s", tmpl, file)
	}
	clean := path.Clean(out)
	if path.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, "../") || clean == "." {
		return "", oops.Errorf("template %q expands outside the project for %s: %s", tmpl, file, out)
	}
	return clean, nil
}
