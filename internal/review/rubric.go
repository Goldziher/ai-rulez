// Package review scores skills, agents, commands and rules against a rubric.
//
// Phase 0 (this package today) is offline: the score is built from the lint
// findings each rubric dimension names as its "twins", and no model is ever
// called. It also plans what a later judge would send, so `review --estimate`
// can print the egress manifest and a cost range first (docs/review.md).
package review

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/safefs"
)

// RubricsDir is the directory of user rubrics under the config directory.
const RubricsDir = "rubrics"

// File names inside a rubric directory.
const (
	RubricFile      = "rubric.toml"
	CalibrationFile = "calibration.json"
	GoldenDir       = "golden"
	// SystemFile is the optional system prompt override.
	SystemFile = "system.md"
)

// SchemaVersion is the only rubric, golden and calibration schema version.
const SchemaVersion = 1

// Item kinds a rubric can apply to.
const (
	KindSkill   = "skill"
	KindAgent   = "agent"
	KindCommand = "command"
	KindRule    = "rule"
)

// Dimension groups: intrinsic needs the item alone, contextual needs siblings.
const (
	GroupIntrinsic  = "intrinsic"
	GroupContextual = "contextual"
)

// Verdict levels and their score values.
const (
	VerdictPass = "pass"
	VerdictWarn = "warn"
	VerdictFail = "fail"
)

// verdictValue is the published score value of a verdict.
var verdictValue = map[string]float64{VerdictPass: 1, VerdictWarn: 0.5, VerdictFail: 0}

//go:embed builtin
var builtinFS embed.FS

// Rubric is a parsed rubric.toml.
//
//nolint:tagliatelle // rubric keys are snake_case by project convention
type Rubric struct {
	SchemaVersion int         `toml:"schema_version" json:"schema_version"`
	ID            string      `toml:"id" json:"id"`
	Version       int         `toml:"version" json:"version"`
	AppliesTo     []string    `toml:"applies_to" json:"applies_to"`
	Limits        Limits      `toml:"limits" json:"limits"`
	Votes         Votes       `toml:"votes" json:"votes"`
	Calibration   Calibration `toml:"calibration" json:"calibration"`
	Dimensions    []Dimension `toml:"dimension" json:"dimensions"`

	// Ref is how the rubric is named on the command line: builtin:<id> or <id>.
	Ref string `toml:"-" json:"ref"`
	// Digest is the sha256 of the rubric's files (hex, "sha256:" prefixed).
	Digest string `toml:"-" json:"digest"`
	// Dir is the rubric directory; empty for a built-in rubric.
	Dir string `toml:"-" json:"-"`
	// SystemPrompt is the contents of system.md, when present.
	SystemPrompt string `toml:"-" json:"-"`
}

// Limits bound the size of what one item sends.
//
//nolint:tagliatelle // rubric keys are snake_case by project convention
type Limits struct {
	MaxItemTokens   int `toml:"max_item_tokens" json:"max_item_tokens"`
	MaxSiblings     int `toml:"max_siblings" json:"max_siblings"`
	MaxOutputTokens int `toml:"max_output_tokens" json:"max_output_tokens"`
}

// Votes configure the later judge phases; phase 0 records them only.
//
//nolint:tagliatelle // rubric keys are snake_case by project convention
type Votes struct {
	FirstTemperature     float64 `toml:"first_temperature" json:"first_temperature"`
	ExtraTemperature     float64 `toml:"extra_temperature" json:"extra_temperature"`
	Max                  int     `toml:"max" json:"max"`
	InstabilityThreshold float64 `toml:"instability_threshold" json:"instability_threshold"`
}

// Calibration holds the thresholds a calibration record must meet before a
// judge may gate; phase 0 records them only.
//
//nolint:tagliatelle // rubric keys are snake_case by project convention
type Calibration struct {
	GoldenMinItems   int                `toml:"golden_min_items" json:"golden_min_items"`
	MinWeightedKappa float64            `toml:"min_weighted_kappa" json:"min_weighted_kappa"`
	MinConsistency   float64            `toml:"min_consistency" json:"min_consistency"`
	MinHumanKappa    float64            `toml:"min_human_kappa" json:"min_human_kappa"`
	MaxAgeDays       int                `toml:"max_age_days" json:"max_age_days"`
	MinRecall        map[string]float64 `toml:"min_recall" json:"min_recall,omitempty"`
}

// Dimension is one scored question.
//
//nolint:tagliatelle // rubric keys are snake_case by project convention
type Dimension struct {
	ID           string   `toml:"id" json:"id"`
	Code         string   `toml:"code" json:"code,omitempty"`
	Group        string   `toml:"group" json:"group"`
	Weight       float64  `toml:"weight" json:"weight"`
	Severity     string   `toml:"severity" json:"severity"`
	Twins        []string `toml:"twins" json:"twins,omitempty"`
	AllowAbsence bool     `toml:"allow_absence" json:"allow_absence,omitempty"`
	Question     string   `toml:"question" json:"question"`
	Pass         string   `toml:"pass" json:"pass"`
	Warn         string   `toml:"warn" json:"warn"`
	Fail         string   `toml:"fail" json:"fail"`
}

var (
	idRe       = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	dimCodeRe  = regexp.MustCompile(`^AR9G[1-7]$`)
	validKinds = map[string]bool{KindSkill: true, KindAgent: true, KindCommand: true, KindRule: true}
)

// ParseRubric decodes rubric.toml bytes strictly: an unknown key is an error.
func ParseRubric(data []byte) (*Rubric, error) {
	var r Rubric
	dec := toml.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&r); err != nil {
		var strict *toml.StrictMissingError
		if errors.As(err, &strict) {
			return nil, oops.Errorf("%s", strict.String())
		}
		return nil, oops.Wrapf(err, "parse rubric")
	}
	return &r, nil
}

// LoadBuiltin returns the embedded rubric with the given id.
func LoadBuiltin(id string) (*Rubric, error) {
	dir := "builtin/" + id
	data, err := builtinFS.ReadFile(dir + "/" + RubricFile)
	if err != nil {
		return nil, oops.Errorf("unknown built-in rubric %q (available: %s)", id, strings.Join(BuiltinIDs(), ", "))
	}
	r, err := ParseRubric(data)
	if err != nil {
		return nil, oops.Wrapf(err, "built-in rubric %q", id)
	}
	r.Ref = config.BuiltinRubricPrefix + id
	r.Digest = digestOf([]fileBytes{{RubricFile, data}})
	return r, nil
}

// BuiltinIDs lists the embedded rubric ids, sorted.
func BuiltinIDs() []string {
	entries, err := fs.ReadDir(builtinFS, "builtin")
	if err != nil {
		return nil
	}
	var ids []string
	for _, e := range entries {
		if e.IsDir() {
			ids = append(ids, e.Name())
		}
	}
	sort.Strings(ids)
	return ids
}

// Load resolves a rubric reference: builtin:<id> or the id of a directory under
// <configDir>/rubrics. A rubric that does not pass Lint is refused so that a
// review never scores against something other than what the file says.
func Load(configDir, ref string) (*Rubric, error) {
	if ref == "" {
		ref = config.DefaultReviewRubric
	}
	if id, ok := strings.CutPrefix(ref, config.BuiltinRubricPrefix); ok {
		return LoadBuiltin(id)
	}
	if !idRe.MatchString(ref) {
		return nil, oops.Errorf("rubric id %q must be lowercase letters, digits and single hyphens (or builtin:<id>)", ref)
	}
	dir := filepath.Join(configDir, RubricsDir, ref)
	r, problems, err := loadDir(dir)
	if err != nil {
		return nil, err
	}
	if len(problems) > 0 {
		return nil, oops.Hint("run `ai-rulez rubric lint "+ref+"`").Errorf("rubric %q is invalid: %s", ref, problems[0].Message)
	}
	return r, nil
}

type fileBytes struct {
	name string
	data []byte
}

func digestOf(files []fileBytes) string {
	sort.Slice(files, func(i, j int) bool { return files[i].name < files[j].name })
	h := sha256.New()
	for _, f := range files {
		fmt.Fprintf(h, "%d:%s\n%d:", len(f.name), f.name, len(f.data)) //nolint:errcheck // hash writes cannot fail
		h.Write(f.data)                                                //nolint:errcheck // hash writes cannot fail
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}

// ListIDs lists the user rubric directories under configDir, sorted.
func ListIDs(configDir string) ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(configDir, RubricsDir))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, oops.Wrapf(err, "read %s", RubricsDir)
	}
	var ids []string
	for _, e := range entries {
		if e.IsDir() {
			ids = append(ids, e.Name())
		}
	}
	sort.Strings(ids)
	return ids, nil
}

// readRegular reads a file that must not be a symlink.
func readRegular(path string) ([]byte, error) {
	data, err := safefs.ReadRegular(path)
	if err != nil {
		return nil, oops.Wrapf(err, "read %s", path)
	}
	return data, nil
}

// Dimension looks a dimension up by id.
func (r *Rubric) Dimension(id string) (Dimension, bool) {
	for _, d := range r.Dimensions {
		if d.ID == id {
			return d, true
		}
	}
	return Dimension{}, false
}

// Applies reports whether the rubric scores items of kind.
func (r *Rubric) Applies(kind string) bool {
	for _, k := range r.AppliesTo {
		if k == kind {
			return true
		}
	}
	return false
}

// Formula is the published scoring formula, emitted in every report.
const Formula = "score = round(100 * sum(weight_i * value_i) / sum(weight_i)) over the dimensions that were scored; pass=1, warn=0.5, fail=0"
