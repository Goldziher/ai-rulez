// Package usage supports usage telemetry for generated skills without ever
// touching the network: a deterministic index of the skills ai-rulez generated,
// a recorder that turns a harness hook event into one identifier-only log line,
// and a report that joins a log against the index.
//
// Privacy is a design constraint, not an option. The recorder reads a hook
// event's skill name, session id and working directory and nothing else: never a
// prompt, a tool input other than the skill name, or file contents.
package usage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"

	"github.com/samber/oops"
)

// IndexFileName is the index's file name inside the config directory.
const IndexFileName = "skills-index.json"

// IndexSchemaVersion is bumped on any incompatible change to the index shape.
const IndexSchemaVersion = 1

// Index lists every skill a generate run produced.
type Index struct {
	SchemaVersion int           `json:"schema_version"`
	Skills        []SkillRecord `json:"skills"`
}

// SkillRecord is the identity of one generated skill.
type SkillRecord struct {
	// ID is the stable skill identifier: the skill's directory name, which is the
	// name every harness invokes it by.
	ID string `json:"id"`
	// Domain is the domain the skill lives in, empty for a root skill.
	Domain string `json:"domain,omitempty"`
	// Source is the authored SKILL.md, relative to the project root with forward
	// slashes.
	Source string `json:"source"`
	// Hash is a blake3 digest over the authored skill: SKILL.md and its bundled
	// resources. It changes whenever the skill's authored content changes.
	Hash    string `json:"hash"`
	Owner   string `json:"owner,omitempty"`
	Version string `json:"version,omitempty"`
	// Outputs lists, per preset, the generated SKILL.md files relative to the
	// project root.
	Outputs map[string][]string `json:"outputs"`
}

// Marshal renders the index as stable, committed-friendly JSON: skills sorted by
// id then source, map keys sorted by encoding/json, a trailing newline.
func (i *Index) Marshal() ([]byte, error) {
	sorted := *i
	sorted.Skills = append([]SkillRecord(nil), i.Skills...)
	sort.SliceStable(sorted.Skills, func(a, b int) bool {
		if sorted.Skills[a].ID != sorted.Skills[b].ID {
			return sorted.Skills[a].ID < sorted.Skills[b].ID
		}
		return sorted.Skills[a].Source < sorted.Skills[b].Source
	})
	for n := range sorted.Skills {
		if sorted.Skills[n].Outputs == nil {
			sorted.Skills[n].Outputs = map[string][]string{}
		}
	}
	data, err := json.MarshalIndent(sorted, "", "  ")
	if err != nil {
		return nil, oops.Wrapf(err, "encode skills index")
	}
	return append(data, '\n'), nil
}

// LoadIndex reads an index file.
func LoadIndex(path string) (*Index, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, oops.With("path", path).Hint("Set [usage] skills_index = true and run ai-rulez generate").Wrapf(err, "read skills index")
	}
	var index Index
	if err := json.Unmarshal(data, &index); err != nil {
		return nil, oops.With("path", path).Wrapf(err, "parse skills index")
	}
	if index.SchemaVersion != IndexSchemaVersion {
		return nil, oops.With("path", path).With("schema_version", index.SchemaVersion).Errorf("unsupported skills index schema")
	}
	return &index, nil
}

// DefaultIndexPath is where generate writes the index for a project.
func DefaultIndexPath(projectRoot, configDirName string) string {
	return filepath.Join(projectRoot, configDirName, IndexFileName)
}

// byID returns the records sharing an id.
func (i *Index) byID(id string) []SkillRecord {
	var out []SkillRecord
	for _, record := range i.Skills {
		if record.ID == id {
			out = append(out, record)
		}
	}
	return out
}
