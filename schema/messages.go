package schema

import (
	"fmt"
	"regexp"
	"strings"
)

// removedPresetNotes explains the preset names that no longer exist.
var removedPresetNotes = map[string]string{
	"windsurf":     `was renamed to "devin"; use "devin" instead`,
	"continue-dev": "was removed and has no replacement; drop it from presets",
}

var (
	// "- presets.0.enum: Value windsurf should be one of the allowed values: ..."
	removedPresetRe = regexp.MustCompile(`^- (presets\.\d+)\.enum: Value (\S+) should be one of the allowed values`)
	missingVersion  = map[string]bool{
		"- 'version': required field is missing": true,
		"- version: required field is missing":   true,
	}
)

// refineMessages rewrites the generic schema findings that have a better
// explanation: a removed preset name becomes a rename hint (and its oneOf noise
// is dropped), and a missing version key is reported once, as missing.
func refineMessages(lines []string) []string {
	drop, repl, missing := scanMessages(lines)
	out := make([]string, 0, len(lines))
	versionWritten := false
	for _, line := range lines {
		switch {
		case drop[line]:
			continue
		case missing && (isMissingVersion(line) || strings.HasPrefix(line, "- version.enum:")):
			if !versionWritten {
				out = append(out, `- version: the required key is missing; add version = "4.0" at the top of the file`)
				versionWritten = true
			}
		case repl[line] != "":
			out = append(out, repl[line])
		default:
			out = append(out, line)
		}
	}
	return out
}

// scanMessages finds the lines refineMessages drops or replaces, and whether
// the version key is reported missing.
func scanMessages(lines []string) (drop map[string]bool, repl map[string]string, missing bool) {
	drop = map[string]bool{}
	repl = map[string]string{}
	for _, line := range lines {
		if m := removedPresetRe.FindStringSubmatch(line); m != nil {
			if note, ok := removedPresetNotes[m[2]]; ok {
				repl[line] = fmt.Sprintf("- %s: preset %q %s", m[1], m[2], note)
				dropPresetNoise(lines, m[1], drop)
			}
		}
		if isMissingVersion(line) {
			missing = true
		}
	}
	return drop, repl, missing
}

// dropPresetNoise marks the oneOf, type and not findings of the preset at path.
func dropPresetNoise(lines []string, path string, drop map[string]bool) {
	for _, suffix := range []string{".oneOf:", ".type:", ".not:"} {
		for _, other := range lines {
			if strings.HasPrefix(other, "- "+path+suffix) {
				drop[other] = true
			}
		}
	}
}

func isMissingVersion(line string) bool {
	return missingVersion[line] || strings.HasPrefix(line, "- version: required field is missing")
}
