package ard

import (
	"os"
	"slices"
	"testing"
)

func FuzzValidate(f *testing.F) {
	golden, err := os.ReadFile("testdata/golden/ard.json")
	if err != nil {
		f.Fatal(err)
	}
	for _, seed := range [][]byte{
		golden,
		[]byte(`{}`), []byte(`{"entries":[]}`), []byte(`{"entries":{}}`), []byte(`[]`), []byte(`null`), []byte(`{"entries":[null]}`),
		[]byte(`{"entries":[{"identifier":"urn:air:example.com:p:n"}]}`),
		[]byte(`{"entries":[{"identifier":"urn:air:example.com:p:n"},{"identifier":"urn:air:example.com:p:n"}]}`),
		[]byte(`{"entries":[{"identifier":"urn:air:localhost:p:n","representativeQueries":["a"]}]}`),
		[]byte(`{"a":1} {"b":2}`), []byte(``), []byte(`{`), []byte(`{"entries":[{"identifier":1e999}]}`),
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		// Act
		got, err := Validate(data)
		again, againErr := Validate(data)

		// Assert
		if (err == nil) != (againErr == nil) || !slices.Equal(got, again) {
			t.Fatalf("Validate is not deterministic for %q", data)
		}
		if err != nil && got != nil {
			t.Fatalf("an error comes with findings: %+v", got)
		}
		for i := 1; i < len(got); i++ {
			if got[i-1].Rule > got[i].Rule && got[i-1].Identifier == got[i].Identifier && got[i-1].Path == got[i].Path {
				t.Fatalf("findings are not sorted: %+v then %+v", got[i-1], got[i])
			}
		}
	})
}
