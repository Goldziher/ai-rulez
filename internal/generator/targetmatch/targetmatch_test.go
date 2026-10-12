package targetmatch

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMatch(t *testing.T) {
	tests := []struct {
		name    string
		targets []string
		presets []string
		outputs []string
		want    bool
	}{
		{"preset name", []string{"cursor"}, []string{"cursor"}, nil, true},
		{"preset name case-insensitive", []string{"CURSOR"}, []string{"cursor"}, nil, true},
		{"other preset", []string{"cursor"}, []string{"claude"}, []string{"CLAUDE.md"}, false},
		{"exact path", []string{".cursor/rules/x.mdc"}, nil, []string{".cursor/rules/x.mdc"}, true},
		{"path case-insensitive", []string{"claude.md"}, nil, []string{"CLAUDE.md"}, true},
		{"dot slash target", []string{"./CLAUDE.md"}, nil, []string{"CLAUDE.md"}, true},
		{"leading slash target", []string{"/CLAUDE.md"}, nil, []string{"CLAUDE.md"}, true},
		{"windows target", []string{`.cursor\rules\x.mdc`}, nil, []string{".cursor/rules/x.mdc"}, true},
		{"windows directory target", []string{`.cursor\rules\`}, nil, []string{".cursor/rules/x.mdc"}, true},
		{"directory prefix", []string{".cursor/rules/"}, nil, []string{".cursor/rules/a/x.mdc"}, true},
		{"directory without slash is a path", []string{".cursor/rules"}, nil, []string{".cursor/rules/x.mdc"}, false},
		{"tree star", []string{".devin/*"}, nil, []string{".devin/rules/x.md"}, true},
		{"tree double star", []string{".devin/**"}, nil, []string{".devin/rules/x.md"}, true},
		{"tree elsewhere", []string{".devin/*"}, nil, []string{".cursor/rules/x.mdc"}, false},
		{"bare star", []string{"*"}, nil, nil, true},
		{"bare double star", []string{"**"}, nil, nil, true},
		{"glob", []string{"*.mdc"}, nil, []string{"x.mdc"}, true},
		{"glob miss", []string{"*.md"}, nil, []string{"x.mdc"}, false},
		{"invalid glob never matches", []string{"[x"}, nil, []string{"x"}, false},
		{"blank target", []string{" "}, []string{""}, []string{"x"}, false},
		{"no targets matches nothing", nil, []string{"cursor"}, []string{"x"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, Match(tt.targets, tt.presets, tt.outputs...))
		})
	}
}

func TestAllow_NoTargets(t *testing.T) {
	assert.True(t, Allow(nil, []string{"cursor"}, "x"))
	assert.False(t, Allow([]string{"claude"}, []string{"cursor"}, "x"))
}

func TestInvalidGlob(t *testing.T) {
	tests := []struct {
		target string
		want   bool
	}{
		{"[x", true},
		{"*.md", false},
		{"CLAUDE.md", false},
		{"[x]/a", false},
		{"a/[", true},
	}
	for _, tt := range tests {
		t.Run(tt.target, func(t *testing.T) {
			assert.Equal(t, tt.want, InvalidGlob(tt.target))
		})
	}
}

func TestNegation(t *testing.T) {
	tests := []struct {
		name      string
		targets   []string
		presets   []string
		outputs   []string
		wantAllow bool
		wantMatch bool
	}{
		{"only negation allows others", []string{"!baz"}, []string{"claude"}, nil, true, false},
		{"only negation denies the named preset", []string{"!baz"}, []string{"baz"}, nil, false, false},
		{"negation is case-insensitive", []string{"!BAZ"}, []string{"baz"}, nil, false, false},
		{"deny wins over a positive target", []string{"baz", "!baz"}, []string{"baz"}, nil, false, false},
		{"positive plus unrelated negation", []string{"claude", "!baz"}, []string{"claude"}, nil, true, true},
		{"positive plus negation of other preset only", []string{"claude", "!baz"}, []string{"baz"}, nil, false, false},
		{"negated path", []string{"!services/"}, nil, []string{"services/x.md"}, false, false},
		{"negated path elsewhere", []string{"!services/"}, nil, []string{"other/x.md"}, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantAllow, Allow(tt.targets, tt.presets, tt.outputs...), "Allow")
			assert.Equal(t, tt.wantMatch, Match(tt.targets, tt.presets, tt.outputs...), "Match")
		})
	}
}

func TestDenied(t *testing.T) {
	assert.True(t, Denied([]string{"!baz"}, []string{"baz"}))
	assert.False(t, Denied([]string{"baz"}, []string{"baz"}))
	assert.False(t, Denied(nil, []string{"baz"}))
}

func TestInvalidGlob_Negation(t *testing.T) {
	assert.False(t, InvalidGlob("!baz"))
	assert.True(t, InvalidGlob("![x"))
}
