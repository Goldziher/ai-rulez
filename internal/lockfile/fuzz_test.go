package lockfile

import (
	"os"
	"testing"
)

func FuzzParse(f *testing.F) {
	for _, seed := range []string{
		"", "version = 2\n", "version = 1\n[[include]]\nname = \"a\"\nsource = \"https://example.com/a\"\ncommit = \"c\"\ndigest = \"sha256:1\"\n",
		"[[skill]]\nname = \"s\"\npath = \"skills/s\"\n", "[[item]]\nkind = \"rule\"\nid = \"x\"\n", "version = \"two\"\n", "[[include]\n",
		"[[approval]]\nname = \"a\"\n", "[[deny]]\nname = \"a\"\n", "[[scan]]\nname = \"a\"\n", "version = 99999999999999999999\n",
		"a = 1979-05-27T07:32:00Z\n", "[[output]]\nrole = \"r\"\npath = \"p\"\n",
	} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		// Act
		parsed, err := Parse(data)
		if err != nil {
			return
		}

		// Assert: whatever parsed can be queried and saved, and saving settles
		// after one round (the second save is the first, byte for byte).
		_ = parsed.FormatVersion()
		_ = parsed.HasContentPins()
		_ = parsed.Find(KindInclude, "a")
		_ = parsed.DenySet()
		_ = parsed.ApprovalsDigest()
		dir := t.TempDir()
		if err := Save(dir, parsed); err != nil {
			return
		}
		first, err := os.ReadFile(Path(dir))
		if err != nil {
			t.Fatal(err)
		}
		reparsed, err := Parse(first)
		if err != nil {
			t.Fatalf("a saved lock does not parse: %v\n%s", err, first)
		}
		if err := Save(dir, reparsed); err != nil {
			t.Fatalf("a saved lock does not save again: %v", err)
		}
		second, err := os.ReadFile(Path(dir))
		if err != nil {
			t.Fatal(err)
		}
		if string(first) != string(second) {
			t.Fatalf("save is not idempotent:\n%s\n---\n%s", first, second)
		}
	})
}
