package obscure

import (
	"bytes"
	"strings"
	"testing"
)

func TestEncodeDecodeRoundTrip(t *testing.T) {
	cases := []struct {
		name  string
		plain string
	}{
		{name: "empty", plain: ""},
		{name: "short", plain: "a = 1\n"},
		{name: "longer than one line", plain: strings.Repeat("pattern = '\\bword\\b'\n", 20)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			enc := Encode([]byte(tc.plain))
			if tc.plain != "" && bytes.Contains(enc, []byte(tc.plain)) {
				t.Fatalf("encoded data holds the plain text")
			}
			for _, line := range strings.Split(strings.TrimSuffix(string(enc), "\n"), "\n") {
				if len(line) > lineWidth {
					t.Fatalf("line is %d columns, want at most %d", len(line), lineWidth)
				}
			}
			got, err := Decode(enc)
			if err != nil {
				t.Fatalf("Decode: %v", err)
			}
			if string(got) != tc.plain {
				t.Fatalf("Decode = %q, want %q", got, tc.plain)
			}
		})
	}
}

func TestDecodeRejectsInvalidBase64(t *testing.T) {
	if _, err := Decode([]byte("not base64!")); err == nil {
		t.Fatal("Decode accepted invalid base64")
	}
}
