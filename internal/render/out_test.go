package render_test

import (
	"bytes"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/render"
)

func TestQuietHidesInfoNeverResults(t *testing.T) {
	for _, quiet := range []bool{false, true} {
		var out, errBuf bytes.Buffer
		o := render.New(&out, &errBuf, quiet)
		o.Result("result %d\n", 1)
		o.Resultln("line")
		o.Info("info\n")
		o.Warn("warn\n")

		if got := out.String(); got != "result 1\nline\n" {
			t.Errorf("quiet=%v stdout = %q", quiet, got)
		}
		want := "info\nwarn\n"
		if quiet {
			want = "warn\n"
		}
		if got := errBuf.String(); got != want {
			t.Errorf("quiet=%v stderr = %q, want %q", quiet, got, want)
		}
	}
}
