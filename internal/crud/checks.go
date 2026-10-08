package crud

import (
	"bytes"
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/samber/oops"
	"gopkg.in/yaml.v3"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/frontmatter"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/targetmatch"
)

// CheckFields are the structured frontmatter fields of a check. A zero value
// (empty string, nil slice) means "not given": the field is left as it is.
type CheckFields struct {
	Description string
	Severity    string
	Tools       []string
	Targets     []string
}

// IsZero reports whether no field is given.
func (f CheckFields) IsZero() bool {
	return f.Description == "" && f.Severity == "" && len(f.Tools) == 0 && len(f.Targets) == 0
}

// ValidateCheckSeverity accepts one of config.CheckSeverities (any case).
func ValidateCheckSeverity(severity string) error {
	severity = strings.ToLower(strings.TrimSpace(severity))
	if severity == "" || slices.Contains(config.CheckSeverities, severity) {
		return nil
	}
	return oops.
		With("field", "severity").
		With("value", severity).
		Hint("Use one of: "+strings.Join(config.CheckSeverities, ", ")).
		Errorf("invalid severity %q: use one of %s", severity, strings.Join(config.CheckSeverities, ", "))
}

// ValidateCheckTargets accepts a preset name, "*" or "**", or a path or glob that
// can match an output file (anything with a "/", a glob character or a file
// extension). A bare word that is none of those, such as a misspelled preset, would
// silently select no output at all, so it is rejected.
func ValidateCheckTargets(targets []string) error {
	for _, raw := range targets {
		target := strings.TrimSpace(raw)
		if target == "" {
			continue
		}
		lower := strings.ToLower(target)
		switch {
		case lower == "*" || lower == "**":
		case config.IsBuiltInPresetName(lower):
		case targetmatch.InvalidGlob(target):
			return oops.With("field", "targets").With("value", target).
				Errorf("invalid target %q: it is not a valid glob pattern", target)
		case strings.ContainsAny(target, "/*?[.\\"):
		default:
			return oops.With("field", "targets").With("value", target).
				Hint("Targets are preset names ("+strings.Join(config.IndividualPresetNames(), ", ")+
					"), or paths and globs such as src/** or REVIEW.md.").
				Errorf("unknown target %q: not a preset name, path or glob", target)
		}
	}
	return nil
}

// splitFrontmatter separates a leading YAML frontmatter block from the body. The
// body loses the one blank line that conventionally follows the closing fence.
func splitFrontmatter(content string) (fm, body string, has bool) {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	block := frontmatter.SplitString(content)
	if !block.Closed {
		return "", content, false // never closed: not frontmatter
	}
	return block.Raw, strings.TrimPrefix(block.Body, "\n"), true
}

// frontmatterMapping parses frontmatter text into a mapping node, keeping key
// order and comments. Empty text is an empty mapping.
func frontmatterMapping(fm string) (*yaml.Node, error) {
	empty := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	if strings.TrimSpace(fm) == "" {
		return empty, nil
	}
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(fm), &doc); err != nil {
		return nil, oops.Hint("Fix the YAML frontmatter of the check first.").Wrapf(err, "parse the check's frontmatter")
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return empty, nil
	}
	root := doc.Content[0]
	if root.Kind == yaml.ScalarNode && root.Tag == "!!null" {
		return empty, nil
	}
	if root.Kind != yaml.MappingNode {
		return nil, oops.Errorf("the check's frontmatter is not a YAML mapping")
	}
	return root, nil
}

// setFrontmatterKey replaces the value of key, or appends the pair.
func setFrontmatterKey(m *yaml.Node, key string, value any) error {
	var node yaml.Node
	if err := node.Encode(value); err != nil {
		return oops.With("key", key).Wrapf(err, "encode frontmatter value")
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			node.HeadComment, node.LineComment = m.Content[i+1].HeadComment, m.Content[i+1].LineComment
			m.Content[i+1] = &node
			return nil
		}
	}
	m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, &node)
	return nil
}

func applyCheckFields(m *yaml.Node, f CheckFields) error {
	if err := ValidateCheckSeverity(f.Severity); err != nil {
		return err
	}
	targets := NormalizeTargets(f.Targets)
	if err := ValidateCheckTargets(targets); err != nil {
		return err
	}
	// Fixed order, so a new frontmatter always reads description, severity, tools, targets.
	steps := []struct {
		key   string
		value any
		given bool
	}{
		{"description", f.Description, f.Description != ""},
		{"severity", strings.ToLower(strings.TrimSpace(f.Severity)), strings.TrimSpace(f.Severity) != ""},
		{"tools", f.Tools, len(f.Tools) > 0},
		{"targets", targets, len(targets) > 0},
	}
	for _, step := range steps {
		if !step.given {
			continue
		}
		if err := setFrontmatterKey(m, step.key, step.value); err != nil {
			return err
		}
	}
	return nil
}

// renderFrontmatter marshals the mapping between fences, followed by a blank
// line. An empty mapping renders nothing.
func renderFrontmatter(m *yaml.Node) (string, error) {
	if len(m.Content) == 0 {
		return "", nil
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(m); err != nil {
		return "", oops.Wrapf(err, "render the check's frontmatter")
	}
	if err := enc.Close(); err != nil {
		return "", oops.Wrapf(err, "render the check's frontmatter")
	}
	return "---\n" + buf.String() + "---\n\n", nil
}

// MergeCheckContent builds the new content of a check from its existing file, new
// content and structured fields:
//
//   - content that carries its own frontmatter replaces the whole file;
//   - content without frontmatter replaces the body and keeps the existing
//     frontmatter, comments and unknown keys included;
//   - no content keeps the existing body;
//   - then the given fields are set on the frontmatter.
//
// The frontmatter is marshaled with a YAML encoder, never assembled from text, so
// any value is quoted correctly. An existing is "" when creating a check.
func MergeCheckContent(existing, content string, contentGiven bool, f CheckFields) (string, error) {
	if contentGiven && f.IsZero() {
		if _, _, has := splitFrontmatter(content); has {
			return content, nil // a full file, taken as written
		}
	}
	var fmText, body string
	switch {
	case contentGiven:
		if given, contentBody, has := splitFrontmatter(content); has {
			fmText, body = given, contentBody
		} else {
			fmText, _, _ = splitFrontmatter(existing)
			body = content
		}
	default:
		fmText, body, _ = splitFrontmatter(existing)
	}
	mapping, err := frontmatterMapping(fmText)
	if err != nil {
		return "", err
	}
	if err := applyCheckFields(mapping, f); err != nil {
		return "", err
	}
	head, err := renderFrontmatter(mapping)
	if err != nil {
		return "", err
	}
	return head + body, nil
}

// BuildCheckContent assembles a new check file from a body and structured fields.
// A body that already starts with frontmatter keeps it, with the fields applied
// over it; severity and targets are validated (see MergeCheckContent).
func BuildCheckContent(body, description, severity string, tools, targets []string) (string, error) {
	return MergeCheckContent("", body, true, CheckFields{
		Description: description, Severity: severity, Tools: tools, Targets: targets,
	})
}

// UpdateCheck rewrites an existing check atomically. Content without frontmatter
// replaces only the body and the given fields are set on the existing
// frontmatter; supplying neither content nor a field is an error, since there is
// nothing to change.
func (op *OperatorImpl) UpdateCheck(ctx context.Context, domain, name, content string, contentGiven bool, f CheckFields) (*FileResult, error) {
	if err := ValidateCheckName(name); err != nil {
		return nil, err
	}
	if !contentGiven && f.IsZero() {
		return nil, oops.
			Hint("Pass content, or at least one of description, severity, tools and targets.").
			Errorf("nothing to update for check %q", name)
	}
	if domain != "" {
		if err := ValidateDomainName(domain); err != nil {
			return nil, err
		}
		if !op.filesMgr.DomainExists(domain) {
			return nil, &DomainNotFoundError{Name: domain, Path: op.filesMgr.GetDomainPath(domain)}
		}
	}
	if !op.filesMgr.FileOrSkillExists(domain, ContentTypeChecks, name) {
		return nil, ErrFileNotFound
	}
	filePath := op.filesMgr.GetFilePath(domain, ContentTypeChecks, name)
	existing, err := op.filesMgr.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	merged, err := MergeCheckContent(config.NativeContent(existing), content, contentGiven, f)
	if err != nil {
		return nil, fmt.Errorf("update check %q: %w", name, err)
	}
	if err := op.overwriteConcept(ctx, filePath, ContentTypeChecks, domain, name, EnsureTrailingNewline(merged), existing); err != nil {
		return nil, err
	}
	return &FileResult{Name: name, FullPath: filePath, Type: ContentTypeChecks, Domain: domain}, nil
}
