package review

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnifiedDiffRoundTrips(t *testing.T) {
	long := func(n int) string {
		var sb strings.Builder
		for i := 1; i <= n; i++ {
			sb.WriteString("line " + strings.Repeat("x", i%5) + "\n")
		}
		return sb.String()
	}
	base := long(40)
	tests := []struct {
		name string
		a, b string
	}{
		{"one changed line", "a\nb\nc\n", "a\nB\nc\n"},
		{"insertion", "a\nc\n", "a\nb\nc\n"},
		{"deletion", "a\nb\nc\n", "a\nc\n"},
		{"change at the start", "a\nb\nc\n", "A\nb\nc\n"},
		{"change at the end", "a\nb\nc\n", "a\nb\nC\n"},
		{"two distant hunks", base, strings.ReplaceAll(strings.Replace(base, "line xx\n", "CHANGED\n", 1), "line xxxx\n", "OTHER\n")},
		{"missing final newline added", "a\nb", "a\nb\n"},
		{"final newline removed", "a\nb\n", "a\nb"},
		{"last line changed without a final newline", "a\nb", "a\nB"},
		{"everything replaced", "a\nb\n", "x\ny\nz\n"},
		{"appended lines", "a\n", "a\nb\nc\n"},
		{"emptied", "a\nb\n", ""},
		{"from empty", "", "a\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			diff := UnifiedDiff("dir/f.md", tt.a, tt.b)
			files, err := ParsePatch("# ai-rulez review fix\n# item: i\n# path: dir/f.md\n# digest: d\n" + diff)
			require.NoError(t, err)
			got, aerr := ApplyHunks(tt.a, files[0].Hunks)

			// Assert
			require.NoError(t, aerr)
			assert.Equal(t, tt.b, got)
			assert.Contains(t, diff, "--- a/dir/f.md\n+++ b/dir/f.md\n")
		})
	}
	assert.Empty(t, UnifiedDiff("f", "same\n", "same\n"))
}

func TestUnifiedDiffHasThreeLinesOfContext(t *testing.T) {
	// Arrange
	a := "1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n"
	b := strings.Replace(a, "5\n", "FIVE\n", 1)

	// Act
	diff := UnifiedDiff("f", a, b)

	// Assert
	assert.Equal(t, "--- a/f\n+++ b/f\n@@ -2,7 +2,7 @@\n 2\n 3\n 4\n-5\n+FIVE\n 6\n 7\n 8\n", diff)
}

func TestApplyHunksRefusesATextTheHunkWasNotMadeFor(t *testing.T) {
	// Arrange
	diff := UnifiedDiff("f", "a\nb\nc\n", "a\nB\nc\n")
	files, err := ParsePatch("# ai-rulez review fix\n# path: f\n# digest: d\n" + diff)
	require.NoError(t, err)

	// Act
	_, err = ApplyHunks("a\nsomething else\nc\n", files[0].Hunks)

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "the file changed since the patch was made")
}

func TestParsePatchRejectsWhatIsNotOurs(t *testing.T) {
	tests := []struct {
		name, text, want string
	}{
		{"empty", "", "no file"},
		{"no digest", "--- a/f\n+++ b/f\n@@ -1,1 +1,1 @@\n-a\n+b\n", "no path or digest header"},
		{"garbage line", "# ai-rulez review fix\n# path: f\n# digest: d\n--- a/f\n+++ b/f\nwhat is this\n", "unexpected line"},
		{"path mismatch", "# ai-rulez review fix\n# path: g\n# digest: d\n--- a/f\n+++ b/f\n", "the diff is for"},
		{"bad hunk header", "# ai-rulez review fix\n# path: f\n# digest: d\n--- a/f\n+++ b/f\n@@ nonsense @@\n", "bad hunk header"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParsePatch(tt.text)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}
