package parser

import (
	"math/rand"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// splitParse is the line-splitting implementation ParseFrontmatter replaced,
// kept as the reference the scanning one must agree with.
func splitParse(content string) (*Metadata, string, error) {
	normalized := normalizeLFLineEndings(strings.TrimPrefix(content, "\ufeff"))
	if !strings.HasPrefix(normalized, "---\n") {
		return nil, normalized, nil
	}
	lines := strings.Split(normalized, "\n")
	if len(lines) < 3 {
		return nil, normalized, nil
	}
	endIdx := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			endIdx = i
			break
		}
	}
	if endIdx == -1 {
		return nil, normalized, nil
	}
	var metadata Metadata
	if err := yaml.Unmarshal([]byte(strings.Join(lines[1:endIdx], "\n")), &metadata); err != nil {
		return nil, normalized, err
	}
	return &metadata, strings.TrimPrefix(strings.Join(lines[endIdx+1:], "\n"), "\n"), nil
}

func TestParseFrontmatterAgreesWithTheLineSplittingReference(t *testing.T) {
	pieces := []string{"---", "\n", "\n", "\r\n", "priority: high", "a: b", "  ", " --- ", "tools: [x, y]", "body text", "\ufeff", "- item", "bad: [", "---\n"}
	rng := rand.New(rand.NewSource(7))
	for range 4000 {
		var sb strings.Builder
		for n := rng.Intn(9); n >= 0; n-- {
			sb.WriteString(pieces[rng.Intn(len(pieces))])
		}
		in := sb.String()
		wantMeta, wantBody, wantErr := splitParse(in)
		gotMeta, gotBody, gotErr := ParseFrontmatter(in)
		if (wantErr == nil) != (gotErr == nil) || wantBody != gotBody || !reflect.DeepEqual(wantMeta, gotMeta) {
			t.Fatalf("input %q:\n want meta=%+v body=%q err=%v\n  got meta=%+v body=%q err=%v", in, wantMeta, wantBody, wantErr, gotMeta, gotBody, gotErr)
		}
	}
}
