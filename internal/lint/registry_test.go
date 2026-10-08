package lint

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
)

var codeFormatRe = sync.OnceValue(func() *regexp.Regexp { return regexp.MustCompile(`^AR[0-9A-Z]{3}$`) })

func TestRegisteredCodesAreUniqueAndDocumented(t *testing.T) {
	docs := ""
	entries, err := filepath.Glob(filepath.Join("..", "..", "docs", "*.md"))
	if err != nil || len(entries) == 0 {
		t.Fatalf("cannot read docs: %v", err)
	}
	for _, e := range entries {
		data, rerr := os.ReadFile(e)
		if rerr != nil {
			t.Fatal(rerr)
		}
		docs += string(data)
	}
	codes, names := map[string]bool{}, map[string]bool{}
	for _, r := range ruleTables().rules {
		if !codeFormatRe().MatchString(r.Code) {
			t.Errorf("code %q is not of the form ARxxx", r.Code)
		}
		if codes[r.Code] {
			t.Errorf("code %s is registered twice", r.Code)
		}
		if names[r.Name] {
			t.Errorf("name %s is registered twice", r.Name)
		}
		codes[r.Code], names[r.Name] = true, true
		if strings.TrimSpace(r.Describe) == "" {
			t.Errorf("%s has no description", r.Code)
		}
		if !strings.Contains(docs, "| "+r.Code+" |") {
			t.Errorf("%s (%s) has no row in docs/*.md", r.Code, r.Name)
		}
		if !strings.Contains(docs, "`"+r.Name+"`") {
			t.Errorf("%s: name %q is not documented", r.Code, r.Name)
		}
	}
}
