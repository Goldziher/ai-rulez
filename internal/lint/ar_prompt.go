package lint

import (
	"regexp"
	"strings"
)

// Codes for the prompt-injection and persistence rules.
const (
	CodeDirectiveLabel  = "AR017"
	CodeFakeTag         = "AR018"
	CodeConfigTamper    = "AR019"
	CodeSelfPropagation = "AR020"
)

func init() {
	registerRules(
		RuleInfo{CodeDirectiveLabel, "directive-label-prefix", SeverityWarning, "a prose line starts with an uppercase SYSTEM:, OVERRIDE:, ADMIN:, ROOT: or IGNORE: label that imitates a privileged message"},
		RuleInfo{CodeFakeTag, "fake-directive-tag", SeverityWarning, "prose contains a literal <system> or <override> tag, or a chat-template token, that imitates a privileged message"},
		RuleInfo{CodeConfigTamper, "agent-config-tamper", SeverityInfo, "text tells the agent to write to its own memory or instruction files (MEMORY.md, CLAUDE.md, AGENTS.md, .cursorrules, settings.json)"},
		RuleInfo{CodeSelfPropagation, "self-propagation", SeverityWarning, "text tells the agent to copy an instruction into every other skill, file or project"},
	)
	// Extensions to existing rules: more injection phrases, soft hyphen, and
	// markdown reference-link comments.
	injectionPhrases = append(injectionPhrases,
		regexp.MustCompile(`(?i)\bhide\s+this\s+from\s+the\s+user\b`),
		regexp.MustCompile(`(?i)\bremove\s+(?:this|it|that)\s+from\s+(?:the\s+)?(?:chat|conversation)\s+history\b`),
		regexp.MustCompile(`(?i)\bnever\s+(?:mention|reveal|disclose)\s+this\s+(?:instruction|rule|message|prompt)s?\b`),
		regexp.MustCompile(`^\s*(?:[-*>]\s*)*(?:DEVELOPER MODE|DEV MODE|DAN MODE|JAILBREAK)\b`),
	)
	hiddenRunes[0x00AD] = "SOFT HYPHEN"
	registerTextScan(scanDirectiveLabels)
	registerTextScan(scanFakeTags)
	registerTextScan(scanConfigTamper)
	registerTextScan(scanSelfPropagation)
	registerTextScan(scanRefComments)
}

var (
	directiveRe = regexp.MustCompile(`^\s*(?:[-*>+]\s*)*[*_]{0,2}(?:SYSTEM|OVERRIDE|ADMIN|ROOT|IGNORE)[*_]{0,2}\s*:(.*)$`)
	// scalarValueRe is the right-hand side of a config-like `KEY: value` line.
	scalarValueRe = regexp.MustCompile(`^\s*(?:true|false|null|~|\d+(?:\.\d+)?|\.{0,2}/[\w./-]*|~/[\w./-]*|\{.*\}|\[.*\])\s*$`)
)

func scanDirectiveLabels(r *runner, t *scanText) {
	for _, l := range t.lines {
		if !t.prose(l) {
			continue
		}
		if m := directiveRe.FindStringSubmatch(l.Text); m != nil && !scalarValueRe.MatchString(m[1]) {
			r.add(CodeDirectiveLabel, t.abs, l.No, "the line starts with an uppercase privileged-looking label; a model may read it as a system or administrator message")
		}
	}
}

var fakeTagRe = regexp.MustCompile(`(?i)</?\s*(?:system|override)(?:\s[^>]*)?/?>|<\|(?:im_start|im_end|system|endoftext)\|>|<<\s*/?SYS\s*>>|\[/?INST\]`)

func scanFakeTags(r *runner, t *scanText) {
	for _, l := range t.lines {
		if !t.prose(l) {
			continue
		}
		if m := fakeTagRe.FindString(l.Plain); m != "" {
			r.add(CodeFakeTag, t.abs, l.No, "%q imitates a system message or chat-template token", m)
		}
	}
}

var configTamperRe = regexp.MustCompile(`(?i)\b(?:write|modify|edit|update|append|add|insert|inject)\w*\b.{0,30}?(?:\b(?:MEMORY|SOUL|CLAUDE|AGENTS)\.md\b|\.cursorrules\b|\.windsurfrules\b|\.clinerules\b|\bsettings\.json\b)`)

func scanConfigTamper(r *runner, t *scanText) {
	for _, l := range t.lines {
		if l.Front || l.Neg || strings.Contains(strings.ToLower(l.Text), "ai-rulez") {
			continue
		}
		if m := configTamperRe.FindString(l.Text); m != "" {
			r.add(CodeConfigTamper, t.abs, l.No, "tells the agent to modify its own instruction or settings files (%q); text that persists itself this way outlives the session", strings.TrimSpace(m))
		}
	}
}

var selfPropRe = regexp.MustCompile(`(?i)\b(?:add|inject|insert|include|append|copy|propagate)\s+(?:this|these|the\s+following)\s+(?:instruction|rule|directive|text|prompt|message)s?\s+(?:to|into|in|at\s+the\s+(?:start|end)\s+of)\s+(?:all|every|each|any|other)\s+(?:other\s+)?(?:skills?|agents?|files?|rules?|projects?|repos?\w*|prompts?|instructions?|configs?|documents?|sessions?|conversations?|memory|memories|commands?|plugins?)\b`)

func scanSelfPropagation(r *runner, t *scanText) {
	for _, l := range t.lines {
		if l.Front {
			continue
		}
		if m := selfPropRe.FindString(l.Text); m != "" {
			r.add(CodeSelfPropagation, t.abs, l.No, "asks the agent to copy an instruction into other places (%q), the way a worm spreads", m)
		}
	}
}

var refCommentRe = regexp.MustCompile(`^\s{0,3}\[(?://|comment|_)\]:\s*(?:#|<>)\s*(?:\((.*)\)|"(.*)"|'(.*)')\s*$`)

// scanRefComments applies the AR003 checks to markdown reference-link comments
// ([//]: # (text)), which render as nothing just like HTML comments.
func scanRefComments(r *runner, t *scanText) {
	for _, l := range t.lines {
		if !t.prose(l) {
			continue
		}
		m := refCommentRe.FindStringSubmatch(l.Text)
		if m == nil {
			continue
		}
		body := m[1] + m[2] + m[3]
		if strings.Contains(body, "ai-rulez-lint-ignore") {
			continue
		}
		suspicious := imperativeRe.MatchString(body)
		for _, re := range r.injectionRes() {
			suspicious = suspicious || re.MatchString(body)
		}
		if suspicious {
			r.add(CodeCommentInstruction, t.abs, l.No, "markdown comment contains instruction-like text the reader will not see")
		}
	}
}
