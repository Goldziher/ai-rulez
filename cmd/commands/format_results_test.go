package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVersionFormatJSON(t *testing.T) {
	t.Cleanup(func() { _ = VersionCmd.Flags().Set("format", "text") }) //nolint:errcheck // restoring the default
	var out bytes.Buffer
	VersionCmd.SetOut(&out)
	t.Cleanup(func() { VersionCmd.SetOut(nil) })

	require.NoError(t, VersionCmd.Flags().Set("format", "json"))
	require.NoError(t, VersionCmd.RunE(VersionCmd, nil))

	var doc map[string]any
	require.NoError(t, json.Unmarshal(out.Bytes(), &doc))
	assert.Equal(t, Version, doc["version"])
	assert.EqualValues(t, 1, doc["schema_version"])
}

func TestVersionTextStaysOneLine(t *testing.T) {
	var out bytes.Buffer
	VersionCmd.SetOut(&out)
	t.Cleanup(func() { VersionCmd.SetOut(nil) })
	require.NoError(t, VersionCmd.Flags().Set("format", "text"))
	require.NoError(t, VersionCmd.RunE(VersionCmd, nil))
	assert.Equal(t, "ai-rulez version "+Version+"\n", out.String())
}

func TestSignFormatJSON(t *testing.T) {
	f := newSignFixture(t, signingKeyTable)
	signLock, signKey = true, f.privKey
	defer func() { signLock, signKey = false, "" }()

	var code int
	stdout, _ := capture(t, func() { code = codeOf(runSign(withSignRecorder(context.Background()), nil, nil)) })

	require.Equal(t, 0, code)
	var doc struct {
		Status string           `json:"status"`
		Signed []map[string]any `json:"signed"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &doc), stdout)
	assert.Equal(t, "signed", doc.Status)
	require.Len(t, doc.Signed, 1)
	assert.Equal(t, "lock", doc.Signed[0]["kind"])
	assert.NotEmpty(t, doc.Signed[0]["bundle"])
}

func TestExportOKFFormatJSON(t *testing.T) {
	root := okfProject(t)
	okfFormat = formatJSON

	var out bytes.Buffer
	okfCheck = true
	code := codeOf(runOKFExport(context.Background(), nil, &out))
	okfCheck = false
	assert.Equal(t, exitOKFProblems, code, "a missing bundle is drift")
	var check struct {
		Status string `json:"status"`
		Drift  struct {
			Missing []string `json:"missing"`
		} `json:"drift"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &check), out.String())
	assert.Equal(t, "drift", check.Status)
	assert.NotEmpty(t, check.Drift.Missing)

	out.Reset()
	code = codeOf(runOKFExport(context.Background(), nil, &out))
	require.Equal(t, 0, code, out.String())
	var written struct {
		Status string   `json:"status"`
		Dir    string   `json:"dir"`
		Files  []string `json:"files"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &written), out.String())
	assert.Equal(t, "written", written.Status)
	assert.Contains(t, written.Files, "index.md")
	assert.Contains(t, written.Dir, root)
}

func TestExportOKFRejectsUnknownFormat(t *testing.T) {
	okfProject(t)
	okfFormat = "xml"
	var out bytes.Buffer
	assert.Equal(t, exitOKFCannotRun, codeOf(runOKFExport(context.Background(), nil, &out)))
}
