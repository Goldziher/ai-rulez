package safefs

import "testing"

// Windows file systems ignore case, so containment must too: a refusal here
// would reject a path that is plainly inside the root.
func TestWithinIgnoresCaseOnWindows(t *testing.T) {
	for _, tc := range []struct {
		root, target string
		want         bool
	}{
		{`C:\Proj`, `c:\proj\src`, true},
		{`C:\Proj`, `C:\PROJ`, true},
		{`C:\Proj`, `c:\projx`, false},
		{`C:\Proj`, `D:\proj`, false},
	} {
		if got := Within(tc.root, tc.target); got != tc.want {
			t.Errorf("Within(%q, %q) = %v, want %v", tc.root, tc.target, got, tc.want)
		}
	}
}
