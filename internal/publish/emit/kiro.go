package emit

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"

	"github.com/samber/oops"
	"gopkg.in/yaml.v3"
)

const kiroSteeringDir = ".kiro/steering/"

var kiroNameUnsafe = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

type kiroFileEntry struct {
	Path   string `json:"path"`
	Size   int    `json:"size"`
	Digest string `json:"digest"`
}

type kiroDistribution struct {
	SchemaVersion int `json:"schema_version"`
	// Target is where the files belong: the workspace .kiro/steering directory,
	// or ~/.kiro/steering for global steering.
	Target string          `json:"target"`
	Files  []kiroFileEntry `json:"files"`
}

// kiroSteering writes a .kiro/steering tree (https://kiro.dev/docs/steering/,
// checked 2026-10-06) and distribution.json, a file list with digests an operator
// packages for an MDM tool. Rules become steering files with the inclusion mode
// that matches their activation; skills become auto-included files. The MDM
// payload itself is the operator's: Kiro only reads ~/.kiro/steering, and how
// the files get there is outside ai-rulez.
type kiroSteering struct{}

// Kiro steering front matter keys.
const keyInclusion = "inclusion"

func init() { register(kiroSteering{}) }

func (kiroSteering) Name() string   { return "kiro-steering" }
func (kiroSteering) Status() string { return StatusExperimental }

func (kiroSteering) Emit(in Input) ([]File, []Finding, error) {
	var (
		files    []File
		findings []Finding
	)
	used := map[string]bool{}
	add := func(prefix string, d Doc, front map[string]any) error {
		base := kiroNameUnsafe.ReplaceAllString(d.Name, "-")
		base = strings.Trim(base, "-.")
		if base == "" {
			return oops.Errorf("kiro-steering: %q cannot be turned into a file name", d.Name)
		}
		file := kiroSteeringDir + prefix + base + ".md"
		if used[file] {
			return oops.Errorf("kiro-steering: two documents render to %s", file)
		}
		used[file] = true
		body, err := steeringFile(front, d.Body)
		if err != nil {
			return err
		}
		files = append(files, File{Path: file, Data: body})
		return nil
	}
	for _, r := range in.Rules {
		front, note := kiroRuleFront(r)
		if note != "" {
			findings = append(findings, Finding{Message: note})
		}
		if err := add("", Doc{Name: r.Name, Body: stripFrontMatter(r.Body)}, front); err != nil {
			return nil, nil, err
		}
	}
	for i := range in.Plugins {
		for _, s := range Skills(in.Plugins[i].Files) {
			front := map[string]any{keyInclusion: "auto", keyName: s.Name, keyDescription: descriptionOr(s)}
			if err := add("skill-", Doc{Name: s.Name, Body: stripFrontMatter(s.Body)}, front); err != nil {
				return nil, nil, err
			}
		}
	}
	if len(files) == 0 {
		return nil, nil, oops.Errorf("kiro-steering: nothing to emit, the project has no rules and the bundle no skills")
	}
	dist := kiroDistribution{SchemaVersion: 1, Target: ".kiro/steering"}
	for _, f := range files {
		sum := sha256.Sum256(f.Data)
		dist.Files = append(dist.Files, kiroFileEntry{Path: f.Path, Size: len(f.Data), Digest: "sha256:" + hex.EncodeToString(sum[:])})
	}
	data, err := jsonBytes(dist)
	if err != nil {
		return nil, nil, err
	}
	files = append(files, File{Path: "distribution.json", Data: data})
	out, err := finish(files)
	return out, findings, err
}

// kiroRuleFront is the steering front matter of a rule, and a note when its
// mode cannot be kept.
func kiroRuleFront(r Doc) (front map[string]any, note string) {
	switch r.Mode {
	case ModeGlob:
		if len(r.Globs) == 0 {
			return map[string]any{keyInclusion: "always"}, "rule " + r.Name + " is glob-scoped without globs; included always"
		}
		return map[string]any{keyInclusion: "fileMatch", "fileMatchPattern": globValue(r.Globs)}, ""
	case ModeAuto:
		return map[string]any{keyInclusion: "auto", keyName: r.Name, keyDescription: descriptionOr(r)}, ""
	case ModeManual:
		return map[string]any{keyInclusion: "manual"}, ""
	}
	return map[string]any{keyInclusion: "always"}, ""
}

func descriptionOr(d Doc) string {
	if strings.TrimSpace(d.Description) != "" {
		return d.Description
	}
	return d.Name
}

func globValue(globs []string) any {
	if len(globs) == 1 {
		return globs[0]
	}
	return globs
}

// steeringFile renders the YAML front matter, which Kiro requires to be the first
// content of the file, followed by the body.
func steeringFile(front map[string]any, body string) ([]byte, error) {
	order := []string{keyInclusion, "fileMatchPattern", keyName, keyDescription}
	node := &yaml.Node{Kind: yaml.MappingNode}
	for _, k := range order {
		v, ok := front[k]
		if !ok {
			continue
		}
		var val yaml.Node
		if err := val.Encode(v); err != nil {
			return nil, oops.Wrapf(err, "encode steering front matter")
		}
		node.Content = append(node.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: k}, &val)
	}
	y, err := yaml.Marshal(node)
	if err != nil {
		return nil, oops.Wrapf(err, "encode steering front matter")
	}
	return []byte("---\n" + string(y) + "---\n\n" + strings.TrimLeft(body, "\n")), nil
}

func stripFrontMatter(body string) string {
	text := strings.ReplaceAll(body, "\r\n", "\n")
	rest, ok := strings.CutPrefix(text, "---\n")
	if !ok {
		return text
	}
	_, after, ok := strings.Cut(rest, "\n---")
	if !ok {
		return text
	}
	return strings.TrimLeft(strings.TrimPrefix(after, "\n"), "\n")
}
