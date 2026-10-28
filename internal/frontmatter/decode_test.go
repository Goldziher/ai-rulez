package frontmatter

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestDecode(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    map[string]any
		wantErr bool
		unclose bool
	}{
		{name: "no block", in: "# Title\n"},
		{name: "empty block", in: "---\n---\nbody\n", want: map[string]any{}},
		{name: "mapping", in: "---\nname: a\npaths: [x/**]\n---\n", want: map[string]any{"name": "a", "paths": []any{"x/**"}}},
		{name: "unquoted colon", in: "---\ndescription: Use when: deploying\n---\n", wantErr: true},
		{name: "a list is not a mapping", in: "---\n- a\n---\n", wantErr: true},
		{name: "a scalar is not a mapping", in: "---\nplain\n---\n", wantErr: true},
		{name: "unclosed", in: "---\nname: a\nbody\n", wantErr: true, unclose: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := Decode(SplitString(c.in))
			if (err != nil) != c.wantErr {
				t.Fatalf("err = %v, want error %v", err, c.wantErr)
			}
			if c.unclose != errors.Is(err, ErrUnclosed) {
				t.Fatalf("err = %v, want ErrUnclosed %v", err, c.unclose)
			}
			if len(got) != len(c.want) || (c.want == nil) != (got == nil) {
				t.Fatalf("got %v, want %v", got, c.want)
			}
			for k, v := range c.want {
				if s, ok := v.(string); ok && got[k] != s {
					t.Errorf("%s = %v, want %v", k, got[k], v)
				}
			}
		})
	}
}

func TestDecode_AYAMLErrorKeepsWhatDecoded(t *testing.T) {
	cases := []struct {
		name     string
		in       string
		want     map[string]any
		typeErr  bool
		wantNull bool
	}{
		{
			name:    "a complex key: a type error decodes every other key",
			in:      "---\nname: badkey\ndescription: d\n? [a, b]\n: v\npaths: [src/**]\n---\n",
			want:    map[string]any{"name": "badkey", "description": "d", "paths": []any{"src/**"}},
			typeErr: true,
		},
		{
			name: "a tag that does not resolve keeps the keys before it",
			in:   "---\ndescription: d\nk: !!int notanint\npaths: [src/**]\n---\n",
			want: map[string]any{"description": "d"},
		},
		{name: "a syntax error decodes nothing", in: "---\ndescription: Use when: x\n---\n", wantNull: true},
		{name: "a list decodes nothing", in: "---\n- a\n---\n", typeErr: true, wantNull: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := Decode(SplitString(c.in))

			require.Error(t, err)
			var typeErr *yaml.TypeError
			assert.Equal(t, c.typeErr, errors.As(err, &typeErr))
			if c.wantNull {
				assert.Nil(t, got)
				return
			}
			assert.Equal(t, c.want, got)
		})
	}
}

func TestNamesKey(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		keys []string
		want bool
	}{
		{name: "a top-level key", raw: "description: Use when: x\npaths: [src/**]\n", keys: []string{"paths"}, want: true},
		{name: "an indented key", raw: "description: d\n\tpaths: [src/**]\n", keys: []string{"paths"}, want: true},
		{name: "a blank before the colon", raw: "paths : [src/**]\n", keys: []string{"paths"}, want: true},
		{name: "any of several keys", raw: "user-invocable: no\n", keys: []string{"paths", "user-invocable"}, want: true},
		{name: "a longer key is another key", raw: "pathsx: [a]\nmy-paths: [b]\n", keys: []string{"paths"}},
		{name: "a key named in a value", raw: "description: set paths: here\n", keys: []string{"paths"}},
		{name: "a key with no colon", raw: "paths\n", keys: []string{"paths"}},
		{name: "an empty block", raw: "", keys: []string{"paths"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, NamesKey(c.raw, c.keys...))
		})
	}
}
