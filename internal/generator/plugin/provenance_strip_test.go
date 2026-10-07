package plugin

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStripProvenanceUndoesInsertProvenanceHeader(t *testing.T) {
	cases := map[string]struct{ path, body string }{
		"markdown with front matter": {"skills/a/SKILL.md", "---\nname: a\ndescription: d\n---\n\n# A\n"},
		"markdown without":           {"skills/a/NOTE.md", "# Note\n\ntext\n"},
		"python":                     {"skills/a/scripts/x.py", "import os\nprint(1)\n"},
		"python with shebang":        {"skills/a/scripts/x.py", "#!/usr/bin/env python3\nprint(1)\n"},
		"typescript":                 {"skills/a/scripts/x.ts", "export const x = 1\n"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			header := provenanceHeader(tc.path, "blake3:aa", "blake3:bb")
			require := insertProvenanceHeader([]byte(tc.body), tc.path, header)
			assert.NotEqual(t, tc.body, string(require))

			assert.Equal(t, tc.body, string(StripProvenance(tc.path, require)))
		})
	}
}

func TestStripProvenanceLeavesOtherFilesAlone(t *testing.T) {
	for _, body := range []string{"plain\n", "<!-- an ordinary comment -->\ntext\n"} {
		assert.Equal(t, body, string(StripProvenance("a.md", []byte(body))))
	}
	assert.Equal(t, "echo 1\n", string(StripProvenance("run.sh", []byte("echo 1\n"))))
}

func TestInterfaceFromDocReadsTheCodexInterfaceBlock(t *testing.T) {
	got, err := InterfaceFromDoc(map[string]any{"displayName": "Acme", "defaultPrompt": []any{"a", "b"}, "websiteURL": "https://x.test"})
	assert.NoError(t, err)
	assert.Equal(t, "Acme", got.DisplayName)
	assert.Equal(t, []string{"a", "b"}, got.DefaultPrompt)
	assert.Equal(t, "https://x.test", got.WebsiteURL)
}
