package lint

import (
	"testing"
)

// A scope renders the root content into its own root file under
// BaseDir/<scope.Path>, so a relative link or backticked path in that content is
// written relative to the scope directory, not to the source file. AR201 and
// AR401 must resolve there too, and must keep firing when the target is nowhere.
func TestScopeResolvesRelativeLinkAndPathAtEmittedLocation(t *testing.T) {
	const scoped = baseConfig + "\n[[scopes]]\nname = \"terragrunt\"\npath = \"deploy/terragrunt\"\n"
	tests := []struct {
		name string
		body string
		// extra adds files on top of the shared fixture.
		extra    map[string]string
		wantLink int
		wantPath int
	}{
		{
			name:     "link resolves at the emitted scope directory",
			body:     "[notifications](../CLAUDE.md#notifications-channel)\n",
			extra:    map[string]string{"deploy/CLAUDE.md": "# Notifications channel\n"},
			wantLink: 0, wantPath: 0,
		},
		{
			name:     "link that resolves nowhere is still reported",
			body:     "[notifications](../CLAUDE.md#notifications-channel)\n",
			wantLink: 1, wantPath: 0,
		},
		{
			name:     "path resolves under the emitted scope directory",
			body:     "Run `bin/capture-text.sh` before committing.\n",
			wantLink: 0, wantPath: 0,
		},
		{
			name:     "path missing everywhere is still reported",
			body:     "Run `bin/absent.sh` before committing.\n",
			wantLink: 0, wantPath: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := t.TempDir()
			files := map[string]string{
				".ai-rulez/config.toml":                 scoped,
				".ai-rulez/rules/doc.md":                "# Doc\n" + tt.body,
				"bin/.keep":                             "",
				"deploy/terragrunt/bin/capture-text.sh": "#!/bin/sh\necho hi\n",
			}
			for k, v := range tt.extra {
				files[k] = v
			}
			writeFiles(t, root, files)
			gitAdd(t, root)

			// Act
			fs := lintDir(t, root)

			// Assert
			if got := countCode(fs, CodeLinkUnresolved); got != tt.wantLink {
				t.Errorf("AR201 count = %d, want %d:\n%s", got, tt.wantLink, dump(fs))
			}
			if got := countCode(fs, CodePathMissing); got != tt.wantPath {
				t.Errorf("AR401 count = %d, want %d:\n%s", got, tt.wantPath, dump(fs))
			}
		})
	}
}

// A backticked container image reference has no file extension and the shape of
// an image ("namespace/name"), so it is not a repository path claim. A missing
// path that keeps a path shape (an extension, no image-name underscore) stays
// reported.
func TestPathMissingIgnoresContainerImages(t *testing.T) {
	tests := []struct {
		name string
		body string
		want int
	}{
		{"image reference with an underscore is not a path", "Build the container image `acme/sra_session_vnc` for the service.", 0},
		{"image reference with a registry host is not a path", "Push `ghcr.io/acme/sra_session_vnc` to the registry.", 0},
		{"missing path with an extension is reported", "Edit `acme/gone.go` after the move.", 1},
		{"missing directory without an underscore is reported", "Read `acme/legacy` for the old shape.", 1},
		{"missing path with an extension and an underscore is reported", "Read `acme/sra_session_vnc.go` for the shapes.", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := t.TempDir()
			writeFiles(t, root, precisionTree(tt.body, "", map[string]string{
				"acme/keep.txt": "x\n",
				"ghcr.io/.keep": "",
			}))
			gitAdd(t, root)

			// Act
			fs := lintDir(t, root)

			// Assert
			if got := countCode(fs, CodePathMissing); got != tt.want {
				t.Errorf("AR401 count = %d, want %d:\n%s", got, tt.want, dump(fs))
			}
		})
	}
}
