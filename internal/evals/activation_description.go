package evals

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// maxDescriptionBytes bounds a candidate description (--description-from).
const maxDescriptionBytes = 16 << 10

const descriptionReplacedWarning = "description replaced for this run only (--description-from); nothing was recorded and the source is unchanged"

// RunActivation runs the activation mode on opts' surface. With
// opts.Description set it measures a candidate description of one skill: the
// skill is copied to a scratch directory with that description in its SKILL.md,
// the run reads the copy, and nothing is recorded (the result stays out of the
// store), so an author can compare two descriptions on the same prompts without
// editing a source.
func RunActivation(ctx context.Context, opts *ActivationOptions) (*ActivationReport, error) {
	if opts.Description == "" && opts.DescriptionSkill == "" {
		return runActivationSurface(ctx, opts)
	}
	scratch, err := os.MkdirTemp("", "ai-rulez-description-*")
	if err != nil {
		return nil, fmt.Errorf("create scratch directory: %w", err)
	}
	defer os.RemoveAll(scratch) //nolint:errcheck // throwaway directory
	o, err := withCandidateDescription(opts, scratch)
	if err != nil {
		return nil, err
	}
	report, err := runActivationSurface(ctx, o)
	if report != nil {
		for i := range report.Skills {
			if report.Skills[i].ID == opts.DescriptionSkill {
				report.Skills[i].Warnings = append(report.Skills[i].Warnings, descriptionReplacedWarning)
			}
		}
	}
	return report, err
}

// withCandidateDescription returns a copy of opts that reads the skill from a
// scratch copy carrying the candidate description, and records nothing.
func withCandidateDescription(opts *ActivationOptions, scratch string) (*ActivationOptions, error) {
	desc := strings.TrimSpace(opts.Description)
	if desc == "" {
		return nil, errors.New("the candidate description is empty")
	}
	if len(desc) > maxDescriptionBytes {
		return nil, fmt.Errorf("the candidate description is %d bytes, over the %d byte limit", len(desc), maxDescriptionBytes)
	}
	if opts.DescriptionSkill == "" {
		return nil, errors.New("--description-from needs the skill it describes (--skill)")
	}
	if len(opts.Skills) != 1 || opts.Skills[0] != opts.DescriptionSkill {
		return nil, fmt.Errorf("skill %q is not among the skills of this run: a candidate description is measured for exactly one skill", opts.DescriptionSkill)
	}
	all, err := FindSkills(opts.ConfigDir)
	if err != nil {
		return nil, err
	}
	var skill *Skill
	for i := range all {
		if all[i].ID == opts.DescriptionSkill {
			skill = &all[i]
			break
		}
	}
	if skill == nil {
		return nil, fmt.Errorf("unknown skill %q", opts.DescriptionSkill)
	}
	dir := filepath.Join(scratch, skill.ID)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("create %s: %w", dir, err)
	}
	if err := copyTree(skill.Dir, dir, "evals"); err != nil {
		return nil, fmt.Errorf("copy skill %q: %w", skill.ID, err)
	}
	file := filepath.Join(dir, "SKILL.md")
	data, err := readBounded(file)
	if err != nil {
		return nil, fmt.Errorf("read SKILL.md of %q: %w", skill.ID, err)
	}
	rewritten, err := withDescription(data, desc)
	if err != nil {
		return nil, fmt.Errorf("SKILL.md of %q: %w", skill.ID, err)
	}
	if err := os.WriteFile(file, rewritten, 0o600); err != nil {
		return nil, fmt.Errorf("write %s: %w", file, err)
	}
	o := *opts
	o.dirOverride = map[string]string{skill.ID: dir}
	o.Store = nil // a candidate is never a measurement of the skill
	return &o, nil
}

// withDescription returns SKILL.md text with its frontmatter description set,
// the other keys and the body kept (a file without frontmatter gets one).
func withDescription(data []byte, desc string) ([]byte, error) {
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	rest, hasFront := strings.CutPrefix(text, "---\n")
	var front, body string
	if hasFront {
		end := strings.Index(rest, "\n---")
		if end < 0 {
			return nil, errors.New("unterminated frontmatter")
		}
		front = rest[:end]
		body = strings.TrimPrefix(rest[end+len("\n---"):], "\n")
	} else {
		body = text
	}
	var doc yaml.Node
	if err := yaml.NewDecoder(strings.NewReader(front)).Decode(&doc); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("frontmatter: %w", err)
	}
	var mapping *yaml.Node
	switch {
	case doc.Kind == yaml.DocumentNode && len(doc.Content) == 1 && doc.Content[0].Kind == yaml.MappingNode:
		mapping = doc.Content[0]
	case doc.Kind == 0:
		mapping = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		doc = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{mapping}}
	default:
		return nil, errors.New("frontmatter is not a mapping")
	}
	value := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: desc}
	set := false
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == frontmatterDescription {
			mapping.Content[i+1] = value
			set = true
		}
	}
	if !set {
		mapping.Content = append(mapping.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: frontmatterDescription}, value)
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		return nil, fmt.Errorf("render frontmatter: %w", err)
	}
	_ = enc.Close() //nolint:errcheck // a bytes.Buffer cannot fail to close
	return []byte("---\n" + buf.String() + "---\n" + body), nil
}
