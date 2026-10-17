package lint

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runWith is run with extra runner options.
func (p *scannerProject) runWith(opts Options, ropts ...Option) []Finding {
	p.t.Helper()
	cfg := loadNoRemote(p.t, p.root)
	tree, err := LoadTree(p.root)
	require.NoError(p.t, err)
	opts.External = true
	rep, err := RunWith(cfg, tree, opts, ropts...)
	require.NoError(p.t, err)
	return rep.Findings
}

func TestScannerMessageLimit(t *testing.T) {
	const length = 1200
	tests := []struct {
		name      string
		policy    string
		entry     string
		wantKept  int
		wantTrunc bool
	}{
		{"default keeps 500 characters", "", "", 500, true},
		{"policy raises the limit", "\n[lint.scanner_policy]\nmax_message = 800\n", "", 800, true},
		{"the entry wins over the policy", "\n[lint.scanner_policy]\nmax_message = 800\n", "max_message = 100\n", 100, true},
		{"a limit above the message keeps it whole", "", "max_message = 5000\n", length, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			p := fakeProject(t, tt.policy, fakeScannerCommand(t, "--msg-len", "1200")+tt.entry)

			// Act
			findings := scannerFindings(p.run(Options{}))

			// Assert
			require.Len(t, findings, 1)
			f := findings[0]
			assert.Equal(t, tt.wantKept, strings.Count(f.Message, "x"))
			assert.Equal(t, tt.wantTrunc, strings.HasSuffix(f.Message, "..."), f.Message)
			if !tt.wantTrunc {
				assert.Empty(t, f.FullMessage())
				return
			}
			assert.Equal(t, length, strings.Count(f.FullMessage(), "x"), "the full text is kept")
			assert.Contains(t, f.FullMessage(), "[fake] FAKE1: ")
		})
	}
}

func TestScannerMessageIsFullInJSONAndSARIFProperties(t *testing.T) {
	// Arrange
	p := fakeProject(t, "", fakeScannerCommand(t, "--msg-len", "700"))
	f := scannerFindings(p.run(Options{}))[0]

	// Act
	raw, err := json.Marshal(f)
	require.NoError(t, err)
	var doc map[string]any
	require.NoError(t, json.Unmarshal(raw, &doc))

	// Assert
	assert.Equal(t, 500, strings.Count(doc["message"].(string), "x"))
	assert.Equal(t, 700, strings.Count(doc["full_message"].(string), "x"))
	assert.Equal(t, 700, strings.Count(resultProperties(&f)["fullMessage"].(string), "x"))
	short := Finding{Code: CodeExternalFinding, Message: "short"}
	assert.NotContains(t, resultProperties(&short), "fullMessage")
}

func TestScannerMessageTruncationIsCountedInVerboseOutput(t *testing.T) {
	// Arrange
	p := fakeProject(t, "", fakeScannerCommand(t, "--msg-len", "900"))
	rec := &testutil.LogRecorder{}

	// Act
	p.runWith(Options{}, WithHost(ambient.Host{Log: rec}))

	// Assert
	assert.Contains(t, strings.Join(rec.Lines(), "\n"), "truncated=1 max_message=500")
}

func TestScannerMaxMessageRejectsOutOfRangeValues(t *testing.T) {
	tests := []struct{ name, policy, entry string }{
		{"negative entry", "", "max_message = -1\n"},
		{"entry above the bound", "", "max_message = 10000000\n"},
		{"negative policy", "\n[lint.scanner_policy]\nmax_message = -5\n", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			p := fakeProject(t, tt.policy, fakeScannerCommand(t)+tt.entry)

			// Act
			findings := p.run(Options{})

			// Assert
			invalid := ofCode(findings, CodeScannerConfigInvalid)
			require.NotEmpty(t, invalid, dump(findings))
			assert.Contains(t, invalid[0].Message, "max_message")
		})
	}
}
