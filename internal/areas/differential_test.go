package areas

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// recordedVectors are the outputs of the area map on seeded random texts and directories, recorded with the
// regular expressions of the matcher (see matcher) under Python's re module (testdata/match_vectors.json). The
// texts mix every case the matcher must get right: upper, title and mixed case, the characters re.IGNORECASE
// folds specially (U+212A, U+017F, U+0130, U+0131, sigma), word and non-word neighbors, path literals with every
// stop and guard character, and non-ASCII terms.
//
// How they were made: a one-off script built the two maps of the file, drew 450 texts and 80 directories per map
// from a fixed seed, and recorded for each text the stripped text, the matches and the classification without a
// directory, and for each directory every area's match length and the classification with no text and with the
// text of the same index. Extending the vectors means recording new inputs the same way. The recording ran on
// CPython 3.11 (Unicode 14.0); the character predicates the matcher emulates were checked against every assigned
// code point on CPython 3.11 to 3.14, so the recording interpreter does not change the vectors. Go 1.27 ships
// Unicode 17, so a code point unassigned in the CPython build may classify differently; none occurs in the
// vectors.
type recordedVectors struct {
	Maps []struct {
		Name       string                `json:"name"`
		Areas      []config.ExposureArea `json:"areas"`
		Normalized []struct {
			Name      string   `json:"name"`
			Terms     []string `json:"terms"`
			Ambiguous []string `json:"ambiguous"`
		} `json:"normalized"`
		Texts []struct {
			Text     string  `json:"text"`
			Stripped string  `json:"stripped"`
			Find     [][]any `json:"find"`
			Classify string  `json:"classify"`
		} `json:"texts"`
		Scopes []struct {
			Scope        string         `json:"scope"`
			Lengths      map[string]int `json:"lengths"`
			Classify     string         `json:"classify"`
			ClassifyText string         `json:"classify_text"`
		} `json:"scopes"`
	} `json:"maps"`
}

func loadVectors(t *testing.T) recordedVectors {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "match_vectors.json"))
	require.NoError(t, err)
	var v recordedVectors
	require.NoError(t, json.Unmarshal(data, &v))
	require.Len(t, v.Maps, 2)
	return v
}

func TestMatchesTheRecordedVectors(t *testing.T) {
	vectors := loadVectors(t)
	for _, vm := range vectors.Maps {
		t.Run(vm.Name, func(t *testing.T) {
			m, problems := New(vm.Areas, nil)
			require.Empty(t, problems)
			for i, want := range vm.Normalized {
				got := m.Areas()[i]
				assert.Equal(t, want.Name, got.Name)
				assert.Equal(t, want.Terms, nonNil(got.Terms), "terms of %s", want.Name)
				assert.Equal(t, want.Ambiguous, nonNil(got.Ambiguous), "ambiguous of %s", want.Name)
			}
			require.NotEmpty(t, vm.Texts)
			for _, tc := range vm.Texts {
				assert.Equal(t, tc.Stripped, m.StripPathLiterals(tc.Text), "strip %q", tc.Text)
				want := make([]TermMatch, 0, len(tc.Find))
				for _, f := range tc.Find {
					want = append(want, TermMatch{Area: f[0].(string), Term: f[1].(string), Ambiguous: f[2].(bool)})
				}
				assert.Equal(t, want, nonNil(m.Find(tc.Text)), "find %q", tc.Text)
				assert.Equal(t, tc.Classify, m.Classify("", tc.Text), "classify %q", tc.Text)
			}
			require.NotEmpty(t, vm.Scopes)
			for i, tc := range vm.Scopes {
				for _, a := range m.Areas() {
					assert.Equal(t, tc.Lengths[a.Name], a.MatchLength(tc.Scope), "%s claim on %q", a.Name, tc.Scope)
				}
				assert.Equal(t, tc.Classify, m.Classify(tc.Scope, ""), "classify %q", tc.Scope)
				assert.Equal(t, tc.ClassifyText, m.Classify(tc.Scope, vm.Texts[i].Text), "classify %q with text", tc.Scope)
			}
		})
	}
}

// FuzzFindTerms checks the invariants of the matcher on arbitrary text: it never panics, every match names a
// word of the map, the result is sorted and duplicate-free, and stripping path literals is idempotent.
func FuzzFindTerms(f *testing.F) {
	vectors := recordedVectors{}
	data, err := os.ReadFile(filepath.Join("testdata", "match_vectors.json"))
	if err == nil {
		_ = json.Unmarshal(data, &vectors) //nolint:errcheck // seeds are optional
	}
	for _, vm := range vectors.Maps {
		for _, tc := range vm.Texts {
			f.Add(tc.Text)
		}
	}
	f.Add("services/billing_x/a and \xff\xfe invalid")
	m, problems := New([]config.ExposureArea{
		{Name: "billing", Paths: []string{"services/billing_*", "*"}, Terms: []string{"billing", "ledger_sync", "σας", "straße"}, Ambiguous: []string{"account"}},
		{Name: "search", Paths: []string{"products/search"}, Terms: []string{"ranker", "kelvin"}, Ambiguous: []string{"account"}},
	}, nil)
	if len(problems) > 0 {
		f.Fatal(problems)
	}
	words := map[string]bool{"billing": true, "ledger_sync": true, "σας": true, "straße": true, "account": true, "ranker": true, "kelvin": true}
	f.Fuzz(func(t *testing.T, text string) {
		matches := m.Find(text)
		for i, match := range matches {
			if !words[match.Term] {
				t.Fatalf("match %v names no word of the map", match)
			}
			if i > 0 && (matches[i-1].Term > match.Term || matches[i-1] == match) {
				t.Fatalf("matches out of order or repeated: %v", matches)
			}
		}
		once := m.StripPathLiterals(text)
		if again := m.StripPathLiterals(once); again != once {
			t.Fatalf("stripping is not idempotent: %q then %q", once, again)
		}
	})
}

func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
