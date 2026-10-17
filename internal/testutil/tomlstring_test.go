package testutil

import (
	"testing"

	"github.com/pelletier/go-toml/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTOMLString_RoundTripsPaths(t *testing.T) {
	tests := []struct{ name, in string }{
		{"posix path", "/tmp/a b/c"},
		{"windows path", `C:\Users\RUNNER~1\AppData\Local\Temp\x`},
		{"quote", `a"b`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out struct{ P string }

			require.NoError(t, toml.Unmarshal([]byte("P = "+TOMLString(tt.in)+"\n"), &out))

			assert.Equal(t, tt.in, out.P)
		})
	}
}
