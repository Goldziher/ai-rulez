package commands

import "testing"

func TestExcludedFromRecursion(t *testing.T) {
	old := recursiveExclude
	t.Cleanup(func() { recursiveExclude = old })
	recursiveExclude = []string{"tests/fixtures/", "svc/*/gen", "./docs/gen"}

	cases := []struct {
		path string
		want bool
	}{
		{"tests/fixtures", true},
		{"tests/fixtures/config/basic", true},
		{"tests/other", false},
		{"tests/fixtures2", false},
		{"svc/api/gen", true},
		{"svc/api/gen/deep", true},
		{"svc/api", false},
		{"docs/gen/x", true},
		{"docs", false},
	}
	for _, tc := range cases {
		if got := excludedFromRecursion(tc.path); got != tc.want {
			t.Errorf("excludedFromRecursion(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}
