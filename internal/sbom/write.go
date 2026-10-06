package sbom

import "io"

// Write encodes bom as indented CycloneDX JSON with a trailing newline. Output is
// byte-stable: struct field order is fixed and every slice is sorted by Build.
func Write(w io.Writer, bom *BOM) error { return encode(w, bom) }

// Render writes bom in the given format (FormatCycloneDX or FormatSPDXJSON).
func Render(w io.Writer, bom *BOM, format string) error {
	if format == FormatSPDXJSON {
		return WriteSPDX(w, bom.ToSPDX())
	}
	return Write(w, bom)
}
