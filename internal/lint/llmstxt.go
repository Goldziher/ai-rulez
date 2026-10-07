package lint

import (
	"path/filepath"

	"github.com/Goldziher/ai-rulez/v5/internal/llmstxt"
)

// The llms.txt family (AR9P0-AR9P9) lints the files the llms-txt preset writes.
// The checks live in internal/llmstxt; this file registers the codes and turns
// a file's findings into strict-validation findings.

// Rule codes of the llms.txt family. They repeat the literals of
// internal/llmstxt so the registry test can see them; TestLLMsTxtCodesMatchPackage
// keeps them in step.
const (
	CodeLLMsTxtTitleMissing      = "AR9P0"
	CodeLLMsTxtSummaryMisplaced  = "AR9P1"
	CodeLLMsTxtHeadingInvalid    = "AR9P2"
	CodeLLMsTxtLinkEntryInvalid  = "AR9P3"
	CodeLLMsTxtOptionalMisplaced = "AR9P4"
	CodeLLMsTxtSectionEmptyOrDup = "AR9P5"
	CodeLLMsTxtLinkTargetInvalid = "AR9P6"
)

// LLMsTxtFile is the findings of one llms.txt file; Path is where it is on disk.
type LLMsTxtFile struct {
	Path     string
	Findings []llmstxt.Finding
}

func registerLLMsTxt(s *ruleSet) {
	for _, r := range llmstxt.Rules() {
		s.addRules(RuleInfo{Code: r.Code, Name: r.Name, Default: Severity(r.Default), Describe: r.Describe})
	}
	s.addDocs(map[string]RuleDoc{
		CodeLLMsTxtTitleMissing: {
			Why:  "The H1 title is the one element every llms.txt reader depends on; without it (or with two) the file is not an llms.txt file.",
			Bad:  "A file that opens with a paragraph or an H2, or has two H1 lines",
			Good: "`# Project name` as the first line, once",
		},
		CodeLLMsTxtSummaryMisplaced: {
			Why:  "The blockquote summary carries the key facts a reader needs before the links; away from the title, or empty, it is not read as the summary.",
			Bad:  "`> Summary` after a detail paragraph, or a bare `>`",
			Good: "A one-paragraph `> Summary` directly under the H1",
		},
		CodeLLMsTxtHeadingInvalid: {
			Why:  "llms.txt allows only the H1 title and H2 section names; any other heading breaks the section structure readers split the file on.",
			Bad:  "`### Rules` inside a section, or a bare `##`",
			Good: "Name each section with an H2; put detail text in plain paragraphs",
		},
		CodeLLMsTxtLinkEntryInvalid: {
			Why:  "A file-list section is a list of links; prose or an entry without a link has no target a reader can fetch.",
			Bad:  "`- see the style guide` under `## Rules`",
			Good: "`- [Style guide](rules/style.md): naming and layout`",
		},
		CodeLLMsTxtOptionalMisplaced: {
			Why:  "Readers drop the Optional section to shorten the context; when it is not last they cannot drop it as one trailing block.",
			Bad:  "`## Optional` followed by `## Docs`",
			Good: "Move `## Optional` to the end",
		},
		CodeLLMsTxtSectionEmptyOrDup: {
			Why:  "An empty section is noise, and a repeated section name makes readers pick one list arbitrarily.",
			Bad:  "Two `## Docs` sections, or a `## Docs` with no entries",
			Good: "Merge the lists; drop empty sections",
		},
		CodeLLMsTxtLinkTargetInvalid: {
			Why:  "A link with an empty target points nowhere.",
			Bad:  "`- [Guide]()`",
			Good: "`- [Guide](docs/guide.md)`",
		},
	})
}

// WithLLMsTxt supplies the findings of the project's generated llms.txt files
// to report as AR9P0-AR9P9.
func WithLLMsTxt(files []LLMsTxtFile) Option {
	return func(r *runner) { r.llmsTxtFiles = files }
}

func (r *runner) checkLLMsTxt() {
	for i := range r.llmsTxtFiles {
		file := r.llmsTxtFiles[i]
		for j := range file.Findings {
			f := file.Findings[j]
			// A finding may be milder than its rule's default. Honor that unless
			// the config set a severity.
			saved, had := r.sev[f.Code]
			if had && saved == Severity(ruleDefault(f.Code)) && Severity(f.Severity) != saved {
				r.sev[f.Code] = Severity(f.Severity)
			}
			r.add(f.Code, filepath.FromSlash(file.Path), f.Line, "%s", f.Message)
			if had {
				r.sev[f.Code] = saved
			}
		}
	}
}

func ruleDefault(code string) llmstxt.Severity {
	for _, r := range llmstxt.Rules() {
		if r.Code == code {
			return r.Default
		}
	}
	return llmstxt.SeverityWarning
}
