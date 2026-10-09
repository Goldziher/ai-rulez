package frontmatter

import "testing"

func TestSplit(t *testing.T) {
	cases := []struct {
		name            string
		in              string
		present, closed bool
		raw, body       string
		head, tail      string
		bodyLine        int
	}{
		{name: "none", in: "# Title\n", body: "# Title\n"},
		{name: "empty", in: "", body: ""},
		{name: "plain", in: "---\na: 1\n---\nbody\n", present: true, closed: true, raw: "a: 1\n", body: "body\n", head: "---\na: 1\n---", tail: "\nbody\n", bodyLine: 4},
		{name: "blank separator kept in body", in: "---\na: 1\n---\n\nbody", present: true, closed: true, raw: "a: 1\n", body: "\nbody", head: "---\na: 1\n---", tail: "\n\nbody", bodyLine: 4},
		{name: "bom", in: "\xef\xbb\xbf---\na: 1\n---\nx", present: true, closed: true, raw: "a: 1\n", body: "x", head: "---\na: 1\n---", tail: "\nx", bodyLine: 4},
		{name: "crlf", in: "---\r\na: 1\r\n---\r\nx\r\n", present: true, closed: true, raw: "a: 1\n", body: "x\r\n", head: "---\r\na: 1\r\n---\r", tail: "\nx\r\n", bodyLine: 4},
		{name: "empty block", in: "---\n---\nbody\n---\nmore\n", present: true, closed: true, raw: "", body: "body\n---\nmore\n", head: "---\n---", tail: "\nbody\n---\nmore\n", bodyLine: 3},
		{name: "closing fence at EOF", in: "---\na: 1\n---", present: true, closed: true, raw: "a: 1\n", body: "", head: "---\na: 1\n---", bodyLine: 4},
		{name: "longer dash run does not close", in: "---\na: 1\n----\nb: 2\n---\nx", present: true, closed: true, raw: "a: 1\n----\nb: 2\n", body: "x", head: "---\na: 1\n----\nb: 2\n---", tail: "\nx", bodyLine: 6},
		{name: "dashes with text do not close", in: "---\na: 1\n---foo\n---\n", present: true, closed: true, raw: "a: 1\n---foo\n", body: "", head: "---\na: 1\n---foo\n---", tail: "\n", bodyLine: 5},
		{name: "trailing space on fence", in: "---\na: 1\n--- \nx", present: true, closed: true, raw: "a: 1\n", body: "x", head: "---\na: 1\n--- ", tail: "\nx", bodyLine: 4},
		{name: "unclosed", in: "---\na: 1\nbody\n", present: true, body: "---\na: 1\nbody\n", raw: "a: 1\nbody\n"},
		{name: "opening only without newline", in: "---", body: "---"},
		{name: "leading blank line is not frontmatter", in: "\n---\na: 1\n---\n", body: "\n---\na: 1\n---\n"},
		{name: "indented fence is not a fence", in: "---\na: 1\n ---\n", present: true, raw: "a: 1\n ---\n", body: "---\na: 1\n ---\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := SplitString(c.in)
			if got.Present != c.present || got.Closed != c.closed {
				t.Fatalf("present/closed = %v/%v, want %v/%v", got.Present, got.Closed, c.present, c.closed)
			}
			if got.Raw != c.raw && (c.closed || c.present) {
				t.Errorf("raw = %q, want %q", got.Raw, c.raw)
			}
			if got.Body != c.body {
				t.Errorf("body = %q, want %q", got.Body, c.body)
			}
			if got.Head != c.head || got.Tail != c.tail {
				t.Errorf("head/tail = %q/%q, want %q/%q", got.Head, got.Tail, c.head, c.tail)
			}
			if got.BodyLine != c.bodyLine {
				t.Errorf("bodyLine = %d, want %d", got.BodyLine, c.bodyLine)
			}
		})
	}
}

func TestSplitBytesMatchesString(t *testing.T) {
	in := "---\nk: v\n---\nb"
	if Split([]byte(in)) != SplitString(in) {
		t.Fatal("Split and SplitString disagree")
	}
}

func TestTrimSeparator(t *testing.T) {
	for in, want := range map[string]string{"\nx": "x", "\r\nx": "x", "\n\nx": "\nx", "x": "x"} {
		if got := TrimSeparator(in); got != want {
			t.Errorf("TrimSeparator(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestClosingLineAgreesWithSplit(t *testing.T) {
	for _, in := range []string{
		"", "# Title\n", "---\na: 1\n---\nbody\n", "\xef\xbb\xbf---\na: 1\n---\nx", "---\r\na: 1\r\n---\r\nx\r\n",
		"---\n---\nbody\n", "---\na: 1\n---", "---\na: 1\nbody\n", "---", "\n---\na: 1\n---\n", "---\na: 1\n ---\n", "---\na: 1\n--- \nx",
	} {
		b := SplitString(in)
		line, ok := ClosingLine(in)
		if ok != b.Closed {
			t.Errorf("%q: ClosingLine ok = %v, Split closed = %v", in, ok, b.Closed)
		}
		if ok && line != b.BodyLine-1 {
			t.Errorf("%q: ClosingLine = %d, Split body line = %d", in, line, b.BodyLine)
		}
	}
}
