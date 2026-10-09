package lint

import (
	"regexp"
	"strings"
	"sync"
)

// Codes for the prompt-injection and persistence rules.
const (
	CodeDirectiveLabel  = "AR017"
	CodeFakeTag         = "AR018"
	CodeConfigTamper    = "AR019"
	CodeSelfPropagation = "AR020"
)

func registerArPrompt(s *ruleSet) {
	s.addRules(
		RuleInfo{CodeDirectiveLabel, "directive-label-prefix", SeverityWarning, "a prose line starts with an uppercase SYSTEM:, OVERRIDE:, ADMIN:, ROOT: or IGNORE: label that imitates a privileged message"},
		RuleInfo{CodeFakeTag, "fake-directive-tag", SeverityWarning, "prose contains a literal <system> or <override> tag, or a chat-template token, that imitates a privileged message"},
		RuleInfo{CodeConfigTamper, "agent-config-tamper", SeverityInfo, "text tells the agent to write to its own memory or instruction files (MEMORY.md, CLAUDE.md, AGENTS.md, .cursorrules, settings.json)"},
		RuleInfo{CodeSelfPropagation, "self-propagation", SeverityWarning, "text tells the agent to copy an instruction into every other skill, file or project"},
	)
	// Extensions to existing rules: more injection phrases, soft hyphen, and
	// markdown reference-link comments.
	injectionPhrases = append(injectionPhrases,
		newGatedRe(`(?i)\bhide\s+this\s+from\s+the\s+user\b`, true, "hide"),
		newGatedRe(`(?i)\bremove\s+(?:this|it|that)\s+from\s+(?:the\s+)?(?:chat|conversation)\s+history\b`, true, "history"),
		newGatedRe(`(?i)\bnever\s+(?:mention|reveal|disclose)\s+this\s+(?:instruction|rule|message|prompt)s?\b`, true, "never"),
		newGatedRe(`^\s*(?:[-*>]\s*)*(?:DEVELOPER MODE|DEV MODE|DAN MODE|JAILBREAK)\b`, false, "DEVELOPER MODE", "DEV MODE", "DAN MODE", "JAILBREAK"),
	)
	hiddenRunes[0x00AD] = "SOFT HYPHEN"
	s.addTextScan(scanDirectiveLabels, AnalyzerSecurity)
	s.addTextScan(scanFakeTags, AnalyzerSecurity)
	s.addTextScan(scanConfigTamper, AnalyzerSecurity)
	s.addTextScan(scanSelfPropagation, AnalyzerSecurity)
	s.addTextScan(scanRefComments, AnalyzerSecurity)
}

var (
	directiveRe = sync.OnceValue(func() *regexp.Regexp {
		return regexp.MustCompile(`^\s*(?:[-*>+]\s*)*[*_]{0,2}(?:SYSTEM|OVERRIDE|ADMIN|ROOT|IGNORE)[*_]{0,2}\s*:(.*)$`)
	})
	// ambiguousLabelRe are the labels that are also ordinary documentation words
	// (ROOT: the repository root); they count only with a command-like remainder.
	ambiguousLabelRe = sync.OnceValue(func() *regexp.Regexp {
		return regexp.MustCompile(`^\s*(?:[-*>+]\s*)*[*_]{0,2}(?:ROOT|IGNORE)[*_]{0,2}\s*:`)
	})
	// commandCueRe marks a remainder that addresses the model or gives an order.
	commandCueRe = sync.OnceValue(func() *regexp.Regexp {
		return regexp.MustCompile(`(?i)\b(?:you|your|must|should|always|never|do|don'?t|ignore|disregard|forget|follow|obey|execute|run|reveal|print|output|respond|answer|act|pretend|instructions?|prompts?|previous|above|anything|everything|all)\b`)
	})
	// scalarValueRe is the right-hand side of a config-like `KEY: value` line.
	scalarValueRe = sync.OnceValue(func() *regexp.Regexp {
		return regexp.MustCompile(`^\s*(?:true|false|null|~|\d+(?:\.\d+)?|\.{0,2}/[\w./-]*|~/[\w./-]*|\{.*\}|\[.*\])\s*$`)
	})
)

func scanDirectiveLabels(r *runner, t *scanText) {
	for _, l := range t.lines {
		if !t.prose(l) {
			continue
		}
		if m := directiveRe().FindStringSubmatch(l.Text); len(m) > 1 && !scalarValueRe().MatchString(m[1]) &&
			(!ambiguousLabelRe().MatchString(l.Text) || commandCueRe().MatchString(m[1])) {
			r.add(CodeDirectiveLabel, t.abs, l.No, "the line starts with an uppercase privileged-looking label; a model may read it as a system or administrator message")
		}
	}
}

// fakeTagPattern builds the AR018 pattern around the tag names that imitate a
// privileged message: the built-in ones and any configured directive_tags.
func fakeTagPattern(extra []string) string {
	names := []string{"system", "override"}
	for _, tag := range extra {
		if tag = strings.TrimSpace(tag); tag != "" {
			names = append(names, regexp.QuoteMeta(tag))
		}
	}
	return `(?i)</?\s*(?:` + strings.Join(names, "|") + `)(?:\s[^>]*)?/?>|<\|(?:im_start|im_end|system|endoftext)\|>|<<\s*/?SYS\s*>>|\[/?INST\]`
}

var fakeTagRe = sync.OnceValue(func() *regexp.Regexp { return regexp.MustCompile(fakeTagPattern(nil)) })

// fakeTags is the AR018 matcher for this run's configuration.
func (r *runner) fakeTags() *regexp.Regexp {
	extra := r.security().DirectiveTags
	if len(extra) == 0 {
		return fakeTagRe()
	}
	r.fakeTagOnce.Do(func() { r.fakeTagRe = regexp.MustCompile(fakeTagPattern(extra)) })
	return r.fakeTagRe
}

func scanFakeTags(r *runner, t *scanText) {
	re := r.fakeTags()
	for _, l := range t.lines {
		if !t.prose(l) {
			continue
		}
		if m := re.FindString(l.Plain); m != "" {
			r.add(CodeFakeTag, t.abs, l.No, "%q imitates a system message or chat-template token", m)
		}
	}
}

var configTamperRe = newGatedRe(`(?i)\b(?:write|modify|edit|update|append|add|insert|inject)\w*\b.{0,30}?(?:\b(?:MEMORY|SOUL|CLAUDE|AGENTS)\.md\b|\.cursorrules\b|\.windsurfrules\b|\.clinerules\b|\.claude/settings(?:\.local)?\.json\b)`, true, "memory.md", "soul.md", "claude.md", "agents.md", ".cursorrules", ".windsurfrules", ".clinerules", "settings")

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

var selfPropRe = newGatedRe(`(?i)\b(?:add|inject|insert|include|append|copy|propagate)\s+(?:this|these|the\s+following)\s+(?:instruction|rule|directive|text|prompt|message)s?\s+(?:to|into|in|at\s+the\s+(?:start|end)\s+of)\s+(?:all|every|each|any|other)\s+(?:other\s+)?(?:skills?|agents?|files?|rules?|projects?|repos?\w*|prompts?|instructions?|configs?|documents?|sessions?|conversations?|memory|memories|commands?|plugins?)\b`, true, "instruction", "rule", "directive", " text", "prompt", "message")

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

var refCommentRe = sync.OnceValue(func() *regexp.Regexp {
	return regexp.MustCompile(`^\s{0,3}\[(?://|comment|_)\]:\s*(?:#|<>)\s*(?:\((.*)\)|"(.*)"|'(.*)')\s*$`)
})

// scanRefComments applies the AR003 checks to markdown reference-link comments
// ([//]: # (text)), which render as nothing just like HTML comments.
func scanRefComments(r *runner, t *scanText) {
	for _, l := range t.lines {
		if !t.prose(l) {
			continue
		}
		m := refCommentRe().FindStringSubmatch(l.Text)
		if m == nil {
			continue
		}
		body := m[1] + m[2] + m[3]
		if strings.Contains(body, "ai-rulez-lint-ignore") {
			continue
		}
		suspicious := imperativeRe().MatchString(body)
		for _, re := range r.injectionRes() {
			suspicious = suspicious || re.MatchString(body)
		}
		if suspicious {
			r.add(CodeCommentInstruction, t.abs, l.No, "markdown comment contains instruction-like text the reader will not see")
		}
	}
}
