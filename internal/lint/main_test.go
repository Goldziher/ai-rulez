package lint

import (
	"fmt"
	"os"
	"sort"
	"sync"
	"testing"
)

// TestMain fails the package when any test run made a unit report a rule of an
// analyzer it did not declare: such a unit would be skipped wrongly when only
// that analyzer is selected, so `--analyzer x` would miss findings.
func TestMain(m *testing.M) {
	var mu sync.Mutex
	seen := map[string]bool{}
	onUndeclaredEmission = func(unit, code string) {
		mu.Lock()
		defer mu.Unlock()
		seen[fmt.Sprintf("unit %s reports %s (%s) but does not declare that analyzer", unit, code, AnalyzerFor(code).Name)] = true
	}
	code := m.Run()
	if len(seen) > 0 {
		lines := make([]string, 0, len(seen))
		for l := range seen {
			lines = append(lines, l)
		}
		sort.Strings(lines)
		for _, l := range lines {
			fmt.Fprintln(os.Stderr, "FAIL:", l)
		}
		code = 1
	}
	os.Exit(code)
}
