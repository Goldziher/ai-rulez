package gitignore

import "strings"

// ReplaceMarkedBlock replaces the region delimited by the exact begin and end
// marker lines with block (which carries its own markers), leaving every other
// line, including other ai-rulez blocks with different markers, untouched. With
// no such region the block is appended; an empty block removes the region.
func ReplaceMarkedBlock(content, begin, end, block string) string {
	var before, after []string
	inRegion, seen := false, false
	for _, line := range strings.Split(content, "\n") {
		switch strings.TrimSpace(line) {
		case begin:
			inRegion, seen = true, true
			continue
		case end:
			if inRegion {
				inRegion = false
				continue
			}
		}
		switch {
		case inRegion:
		case seen:
			after = append(after, line)
		default:
			before = append(before, line)
		}
	}

	head := strings.TrimRight(strings.Join(before, "\n"), "\n")
	tail := strings.TrimLeft(strings.Join(after, "\n"), "\n")
	block = strings.Trim(block, "\n")

	var out strings.Builder
	out.WriteString(head)
	if block != "" {
		if head != "" {
			out.WriteString("\n\n")
		}
		out.WriteString(block)
	}
	if out.Len() > 0 {
		out.WriteString("\n")
	}
	if tail != "" {
		if out.Len() > 0 {
			out.WriteString("\n")
		}
		out.WriteString(tail)
	}
	return out.String()
}
