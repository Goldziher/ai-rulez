package harnesslimits

import "testing"

func TestTableLoadsAndKnownRowsHaveTheirValues(t *testing.T) {
	// Arrange
	want := map[string]int{
		"claude.skill_listing_chars":  1536,
		"codex.agents_md_chain_bytes": 32768,
		"devin.rule_file_chars":       12000,
		"antigravity.rule_file_bytes": 24000,
	}
	// Act / Assert
	for id, v := range want {
		l, ok := Get(id)
		if !ok {
			t.Errorf("%s missing", id)
			continue
		}
		if l.Value != v || MustValue(id) != v {
			t.Errorf("%s = %d, want %d", id, l.Value, v)
		}
	}
	if _, ok := Get("nope"); ok {
		t.Error("unknown id found")
	}
}
