package mcp

import (
	"bytes"
	"fmt"
	"os"
	"path"
	"strings"
	"unicode/utf8"
)

const (
	// defaultFileMode is the mode reported for a file whose source mode is unknown.
	defaultFileMode os.FileMode = 0o644
	// maxReadChunkBytes bounds what one read_skill_file call returns, so a large
	// file is read in pieces instead of in one reply.
	maxReadChunkBytes = 256 * 1024
	// maxShebangBytes bounds the first line read as an interpreter hint.
	maxShebangBytes = 256

	encodingText   = "text"
	encodingBase64 = "base64"

	keyMode        = "mode"
	keyExecutable  = "executable"
	keyInterpreter = "interpreter"

	// filesAccessNote tells an agent that a served skill's files are not on disk
	// and how to get at them (#405).
	filesAccessNote = "This skill's supporting files are not on disk: they are served over MCP. " +
		"Fetch one with read_skill_file {uri} (encoding: base64 for binary files, offset/length for large ones) or resources/read, " +
		"write it under a directory you choose keeping the relative paths, and restore the mode listed for it in resources " +
		"(chmod +x when executable is true) before running a script; interpreter is a hint for how to run it. " +
		"To have the files on disk instead, set delivery = \"both\" for this skill and run ai-rulez generate."
)

// perm is the file's permission bits, 0o644 when the source mode is unknown.
func (f *CatalogFile) perm() os.FileMode {
	if p := f.Mode.Perm(); p != 0 {
		return p
	}
	return defaultFileMode
}

// executable reports whether any execute bit is set.
func (f *CatalogFile) executable() bool { return f.perm()&0o111 != 0 }

// modeString renders the permission bits as octal, for example 0755.
func (f *CatalogFile) modeString() string { return fmt.Sprintf("%04o", uint32(f.perm())) }

// fileModeFields are the mode, executable and interpreter fields of a file; the
// interpreter is left out when there is no hint.
func (f *CatalogFile) fileModeFields() map[string]any {
	m := map[string]any{keyMode: f.modeString(), keyExecutable: f.executable()}
	if in := interpreterHint(f.RelPath, f.Content); in != "" {
		m[keyInterpreter] = in
	}
	return m
}

// metaFields is fileModeFields keyed for a resource's _meta.
func (f *CatalogFile) metaFields() map[string]any {
	out := map[string]any{}
	for k, v := range f.fileModeFields() {
		out[skillsMetaPrefix+k] = v
	}
	return out
}

const interpNode = "node"

var extensionInterpreters = map[string]string{
	".sh": "sh", ".bash": "bash", ".zsh": "zsh", ".py": "python3", ".js": interpNode, ".mjs": interpNode,
	".cjs": interpNode, ".rb": "ruby", ".pl": "perl", ".php": "php", ".ps1": "pwsh",
}

// interpreterHint is how to run a file: its shebang line, else the interpreter
// its extension implies; empty when neither says.
func interpreterHint(rel string, content []byte) string {
	if bytes.HasPrefix(content, []byte("#!")) {
		line, _, _ := bytes.Cut(content, []byte("\n"))
		if len(line) <= maxShebangBytes && utf8.Valid(line) {
			if hint := strings.TrimSpace(strings.TrimPrefix(string(line), "#!")); hint != "" {
				return hint
			}
		}
	}
	if !utf8.Valid(content) {
		return ""
	}
	return extensionInterpreters[strings.ToLower(path.Ext(rel))]
}

// fileWindow is the byte range of a file one read returns.
type fileWindow struct {
	start, end int
}

// readWindow resolves offset and length (0 means to the end) against the size of
// a file, bounded by maxReadChunkBytes. For text, the end moves back to a rune
// boundary and an offset inside a rune is an error.
func readWindow(content []byte, offset, length int, text bool) (fileWindow, error) {
	if offset < 0 || offset > len(content) {
		return fileWindow{}, fmt.Errorf("offset %d is outside the file (%d bytes)", offset, len(content))
	}
	if length < 0 {
		return fileWindow{}, fmt.Errorf("length %d must not be negative", length)
	}
	if length == 0 || length > maxReadChunkBytes {
		length = maxReadChunkBytes
	}
	end := min(offset+length, len(content))
	if text {
		if offset < len(content) && !utf8.RuneStart(content[offset]) {
			return fileWindow{}, fmt.Errorf("offset %d is inside a UTF-8 character; use the next_offset of the previous read or encoding base64", offset)
		}
		for end < len(content) && end > offset && !utf8.RuneStart(content[end]) {
			end--
		}
		if end == offset && offset < len(content) {
			return fileWindow{}, fmt.Errorf("length %d is shorter than one character; raise it or use encoding base64", length)
		}
	}
	return fileWindow{start: offset, end: end}, nil
}
