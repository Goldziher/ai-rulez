package safefs

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"testing/iotest"
)

func TestReadLimited(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		limit   int64
		want    string
		tooLong bool
	}{
		{"under the limit", "abc", 10, "abc", false},
		{"exactly the limit", "abcde", 5, "abcde", false},
		{"one byte over", "abcdef", 5, "", true},
		{"empty", "", 5, "", false},
		{"zero limit with data", "a", 0, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ReadLimited(strings.NewReader(tt.in), tt.limit)
			if tt.tooLong {
				if !errors.Is(err, ErrTooLarge) {
					t.Fatalf("err = %v, want ErrTooLarge", err)
				}
				if got != nil {
					t.Errorf("a refused read must not return a truncated prefix, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("err = %v", err)
			}
			if !bytes.Equal(got, []byte(tt.want)) {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestReadLimitedPropagatesReaderErrors(t *testing.T) {
	boom := errors.New("boom")
	_, err := ReadLimited(iotest.ErrReader(boom), 10)
	if !errors.Is(err, boom) || errors.Is(err, ErrTooLarge) {
		t.Fatalf("err = %v, want the reader's error", err)
	}
}
