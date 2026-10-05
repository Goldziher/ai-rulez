package docmerge

import (
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/generator/jsonmerge"
	"github.com/samber/oops"
)

// FormatMarkdown is a Markdown document that ai-rulez shares with its user. It
// owns only marker-delimited blocks:
//
//	<!-- ai-rulez:NAME:begin -->
//	...generated text...
//	<!-- ai-rulez:NAME:end -->
//
// Everything outside the markers is the user's and is kept as it is. An owned key
// is a block: Name is the block name and Value the text (a string). A document
// that has no such block gets it appended after a blank line, so a hand-written
// REVIEW.md survives, and Unmerge takes only the block back out. A key named
// HeaderKey is not a block: its string Value is written once, above the block,
// when the document is created, and is claimed so a document that holds nothing
// else still counts as generated.
const FormatMarkdown Format = "markdown"

// HeaderKey names the owned key that carries the text written above a block when
// the document is created (frontmatter a tool needs at the top of the file).
const HeaderKey = "header"

// BlockMarkers returns the begin and end marker lines of the named block.
func BlockMarkers(name string) (begin, end string) {
	return "<!-- ai-rulez:" + name + ":begin -->", "<!-- ai-rulez:" + name + ":end -->"
}

type mdBlock struct {
	name, text string
}

// mdSpan is the byte range of a block, from the start of its begin line through
// the end of its end line (newline included).
type mdSpan struct{ start, end int }

func markdownOwned(owned []OwnedKey) (header string, blocks []mdBlock, err error) {
	for _, key := range owned {
		segs := key.Segments()
		if len(segs) != 1 || key.Remove || key.Members || key.Elements != nil {
			if key.Remove || len(segs) == 0 {
				continue
			}
			return "", nil, oops.With("key", strings.Join(segs, ".")).Errorf("a markdown document owns whole blocks only")
		}
		text, ok := key.Value.(string)
		if !ok {
			return "", nil, oops.With("key", segs[0]).Errorf("the value of a markdown block must be a string")
		}
		if segs[0] == HeaderKey {
			header = text
			continue
		}
		begin, end := BlockMarkers(segs[0])
		for _, line := range strings.Split(text, "\n") {
			if t := strings.TrimSpace(line); t == begin || t == end {
				return "", nil, oops.With("block", segs[0]).
					Hint("remove the marker comment from the text").
					Errorf("the text of block %q contains its own marker line", segs[0])
			}
		}
		blocks = append(blocks, mdBlock{name: segs[0], text: text})
	}
	return header, blocks, nil
}

func lineEnding(doc string) string {
	if strings.Contains(doc, "\r\n") {
		return "\r\n"
	}
	return "\n"
}

// withEndings rewrites every line break of text to nl.
func withEndings(text, nl string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	if nl != "\n" {
		text = strings.ReplaceAll(text, "\n", nl)
	}
	return text
}

// findBlock locates the named block's marker lines. found is false when there is
// no begin marker; a begin without a following end, an end without a begin, or a
// second block of the same name is an error, since replacing or removing text
// between unmatched markers would take the user's content along.
func findBlock(path, doc, name string) (span mdSpan, found bool, err error) {
	begin, end := BlockMarkers(name)
	beginAt, endAt := -1, -1
	pos := 0
	for pos < len(doc) {
		next := len(doc)
		lineEnd := len(doc)
		if nl := strings.IndexByte(doc[pos:], '\n'); nl >= 0 {
			next = pos + nl + 1
			lineEnd = pos + nl
		}
		switch strings.TrimSpace(doc[pos:lineEnd]) {
		case begin:
			if beginAt >= 0 {
				return mdSpan{}, false, markerError(path, name, "has more than one begin marker")
			}
			beginAt = pos
		case end:
			if beginAt < 0 || endAt >= 0 {
				return mdSpan{}, false, markerError(path, name, "has an end marker without a matching begin marker")
			}
			endAt = next
		}
		pos = next
	}
	switch {
	case beginAt < 0 && endAt < 0:
		return mdSpan{}, false, nil
	case beginAt < 0 || endAt < 0:
		return mdSpan{}, false, markerError(path, name, "has a begin marker without a matching end marker")
	}
	return mdSpan{start: beginAt, end: endAt}, true, nil
}

func markerError(path, name, problem string) error {
	begin, end := BlockMarkers(name)
	return oops.With("path", path, "block", name).
		Hint("Fix or delete the marker lines "+begin+" and "+end+"; ai-rulez will not edit a file whose markers do not pair up.").
		Errorf("the ai-rulez %q block in %s %s", name, path, problem)
}

func renderBlock(b mdBlock, nl string) string {
	begin, end := BlockMarkers(b.name)
	body := strings.TrimRight(withEndings(b.text, nl), "\r\n")
	return begin + nl + body + nl + end + nl
}

func applyMarkdown(path, existing string, owned []OwnedKey) (Result, error) {
	header, blocks, err := markdownOwned(owned)
	if err != nil {
		return Result{}, oops.With("path", path).Wrap(err)
	}
	bom, doc := jsonmerge.SplitBOM(existing)
	nl := lineEnding(doc)
	creating := strings.TrimSpace(doc) == ""
	if creating {
		doc = ""
		bom = ""
		if header != "" {
			doc = strings.TrimRight(withEndings(header, nl), "\r\n") + nl + nl
		}
	}
	for _, b := range blocks {
		span, found, ferr := findBlock(path, doc, b.name)
		if ferr != nil {
			return Result{}, ferr
		}
		block := renderBlock(b, nl)
		if found {
			doc = doc[:span.start] + block + doc[span.end:]
			continue
		}
		if doc != "" && !strings.HasSuffix(doc, "\n") {
			doc += nl
		}
		if doc != "" && !strings.HasSuffix(doc, nl+nl) {
			doc += nl
		}
		doc += block
	}
	for _, b := range blocks {
		span, found, ferr := findBlock(path, doc, b.name)
		if ferr != nil || !found || !strings.Contains(doc[span.start:span.end], strings.TrimRight(withEndings(b.text, nl), "\r\n")) {
			return Result{}, oops.With("path", path, "block", b.name).Errorf("the merged markdown document does not hold the block")
		}
	}

	rest, headerPresent := outsideBlocks(path, doc, blocks, header, nl)
	claims := make([]Claim, 0, len(blocks)+1)
	for _, b := range blocks {
		claims = append(claims, Claim{Path: []string{b.name}, Sum: jsonmerge.Digest(b.text)})
	}
	if headerPresent {
		claims = append(claims, Claim{Path: []string{HeaderKey}, Equals: header})
	}
	if !creating {
		claims = jsonmerge.NoteFinalNewline(claims, existing)
	}
	return Result{Body: bom + doc, PartiallyOwned: strings.TrimSpace(rest) != "", Claims: claims}, nil
}

// outsideBlocks returns the document without its blocks and without the header
// that opens it (when the document starts with it), and whether it did.
func outsideBlocks(path, doc string, blocks []mdBlock, header, nl string) (rest string, headerPresent bool) {
	for _, b := range blocks {
		if span, found, err := findBlock(path, doc, b.name); err == nil && found {
			doc = doc[:span.start] + doc[span.end:]
		}
	}
	if header = strings.TrimRight(withEndings(header, nl), "\r\n"); header != "" && strings.HasPrefix(doc, header) {
		return doc[len(header):], true
	}
	return doc, false
}

func unmergeMarkdown(path, existing string, claims []Claim) (Unmerged, error) {
	bom, doc := jsonmerge.SplitBOM(existing)
	nl := lineEnding(doc)
	var out Unmerged
	header := ""
	for _, claim := range claims {
		if len(claim.Path) != 1 {
			continue
		}
		if claim.Path[0] == HeaderKey {
			if text, ok := claim.Equals.(string); ok {
				header = text
			}
			continue
		}
		span, found, err := findBlock(path, doc, claim.Path[0])
		if err != nil {
			return Unmerged{}, err
		}
		if !found {
			continue
		}
		before, after := doc[:span.start], doc[span.end:]
		// Take back one of the blank lines around the block: the one Apply put in
		// front of an appended block, or the doubled one where it sat between
		// paragraphs.
		switch {
		case after == "" && strings.HasSuffix(before, nl+nl):
			before = strings.TrimSuffix(before, nl)
		case strings.HasSuffix(before, nl+nl) && strings.HasPrefix(after, nl):
			after = strings.TrimPrefix(after, nl)
		}
		doc = before + after
		out.Changed = true
	}
	if !out.Changed {
		return Unmerged{}, nil
	}
	rest := doc
	if header = strings.TrimRight(withEndings(header, nl), "\r\n"); header != "" && strings.HasPrefix(rest, header) {
		rest = rest[len(header):]
	}
	if strings.TrimSpace(rest) == "" {
		return Unmerged{Changed: true, Empty: true}, nil
	}
	out.Body = bom + jsonmerge.RestoreFinalNewline(claims, doc)
	return out, nil
}
