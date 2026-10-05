package generator

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/jsonmerge"
)

func TestUnionOwned(t *testing.T) {
	arrayKey := func(elements ...any) jsonmerge.OwnedKey {
		return jsonmerge.OwnedKey{Path: []string{"hooks", "Stop"}, Value: elements, Elements: elements}
	}
	tests := []struct {
		name   string
		a, b   []jsonmerge.OwnedKey
		want   []jsonmerge.OwnedKey
		wantOK bool
	}{
		{
			name: "disjoint keys are concatenated",
			a:    []jsonmerge.OwnedKey{{Path: []string{"a"}, Value: 1}}, b: []jsonmerge.OwnedKey{{Path: []string{"b"}, Value: 2}},
			want:   []jsonmerge.OwnedKey{{Path: []string{"a"}, Value: 1}, {Path: []string{"b"}, Value: 2}},
			wantOK: true,
		},
		{
			name: "identical keys are kept once",
			a:    []jsonmerge.OwnedKey{{Path: []string{"a"}, Value: 1}}, b: []jsonmerge.OwnedKey{{Path: []string{"a"}, Value: 1}},
			want:   []jsonmerge.OwnedKey{{Path: []string{"a"}, Value: 1}},
			wantOK: true,
		},
		{
			name: "arrays of one path combine their elements",
			a:    []jsonmerge.OwnedKey{arrayKey("x", "y")}, b: []jsonmerge.OwnedKey{arrayKey("y", "z")},
			want:   []jsonmerge.OwnedKey{arrayKey("x", "y", "z")},
			wantOK: true,
		},
		{
			name: "scalars that disagree do not combine",
			a:    []jsonmerge.OwnedKey{{Path: []string{"a"}, Value: 1}}, b: []jsonmerge.OwnedKey{{Path: []string{"a"}, Value: 2}},
			wantOK: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := unionOwned(tt.a, tt.b)

			assert.Equal(t, tt.wantOK, ok)
			if tt.wantOK {
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestFlattenPresetOutputs_DifferentPlainFilesStayAConflict(t *testing.T) {
	_, err := flattenPresetOutputs(map[string][]config.OutputFile{
		"a": {{Path: "x.txt", Content: "one"}},
		"b": {{Path: "x.txt", Content: "two"}},
	})

	require.Error(t, err)
}
