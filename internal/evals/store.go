package evals

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
)

// StoreFileName is the results store's file name inside the config directory.
const StoreFileName = "eval-results.json"

// StoreSchemaVersion is bumped on any incompatible change to the store.
const StoreSchemaVersion = 1

// PassMark remembers the last run that passed.
type PassMark struct {
	Digest string `json:"digest"`
	Date   string `json:"date,omitempty"`
}

// SkillRecord is the latest recorded eval run of one skill.
type SkillRecord struct {
	ID string `json:"id"`
	// Digest is the sha256 of the skill's authored content when the run happened.
	Digest      string `json:"digest"`
	CasesDigest string `json:"cases_digest"`
	// CacheKey identifies everything that determines the result; an equal key means
	// the run need not be repeated.
	CacheKey string `json:"cache_key"`
	Runner   string `json:"runner"`
	Harness  string `json:"harness"`
	Model    string `json:"model"`
	Ablation bool   `json:"ablation"`
	// Date is supplied by the caller (--date or AI_RULEZ_EVAL_DATE); the store never
	// reads the clock, so equal inputs give equal files.
	Date string `json:"date,omitempty"`
	// Passing says the run met the pass threshold.
	Passing bool       `json:"passing"`
	Score   SkillScore `json:"score"`
	// LastPass is the most recent passing run, kept when a later run fails.
	LastPass *PassMark `json:"last_pass,omitempty"`
}

// Store is the on-disk results file.
type Store struct {
	SchemaVersion int           `json:"schema_version"`
	Skills        []SkillRecord `json:"skills"`
}

// NewStore returns an empty store.
func NewStore() *Store { return &Store{SchemaVersion: StoreSchemaVersion} }

// DefaultStorePath is where eval run records results for a project.
func DefaultStorePath(configDir string) string { return filepath.Join(configDir, StoreFileName) }

// LoadStore reads a store. A missing file is an empty store.
func LoadStore(path string) (*Store, error) {
	data, err := os.ReadFile(path) //nolint:gosec // the project's own results file
	if errors.Is(err, os.ErrNotExist) {
		return NewStore(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("read eval results: %w", err)
	}
	var store Store
	if err := json.Unmarshal(data, &store); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if store.SchemaVersion != StoreSchemaVersion {
		return nil, fmt.Errorf("%s has schema_version %d; this ai-rulez reads %d", path, store.SchemaVersion, StoreSchemaVersion)
	}
	return &store, nil
}

// Get returns the record of a skill.
func (s *Store) Get(id string) (*SkillRecord, bool) {
	for i := range s.Skills {
		if s.Skills[i].ID == id {
			return &s.Skills[i], true
		}
	}
	return nil, false
}

// Put stores a record, replacing the skill's previous one and carrying its
// last-pass mark forward unless the new run passed.
func (s *Store) Put(record SkillRecord) {
	if old, ok := s.Get(record.ID); ok {
		if record.Passing {
			record.LastPass = &PassMark{Digest: record.Digest, Date: record.Date}
		} else if record.LastPass == nil {
			record.LastPass = old.LastPass
		}
		*old = record
		return
	}
	if record.Passing {
		record.LastPass = &PassMark{Digest: record.Digest, Date: record.Date}
	}
	s.Skills = append(s.Skills, record)
}

// Stale reports whether the skill changed after its last passing run. A skill
// with no passing run on record is not reported stale.
func (s *Store) Stale(id, currentDigest string) (last PassMark, stale bool) {
	record, ok := s.Get(id)
	if !ok || record.LastPass == nil {
		return PassMark{}, false
	}
	return *record.LastPass, record.LastPass.Digest != currentDigest
}

// Marshal renders the store as stable JSON: skills sorted by id, a trailing newline.
func (s *Store) Marshal() ([]byte, error) {
	sorted := *s
	sorted.SchemaVersion = StoreSchemaVersion
	sorted.Skills = append([]SkillRecord(nil), s.Skills...)
	sort.SliceStable(sorted.Skills, func(a, b int) bool { return sorted.Skills[a].ID < sorted.Skills[b].ID })
	if sorted.Skills == nil {
		sorted.Skills = []SkillRecord{}
	}
	data, err := json.MarshalIndent(sorted, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode eval results: %w", err)
	}
	return append(data, '\n'), nil
}

// Save writes the store atomically.
func (s *Store) Save(path string) error {
	data, err := s.Marshal()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(path), err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".eval-results-*")
	if err != nil {
		return fmt.Errorf("write eval results: %w", err)
	}
	defer os.Remove(tmp.Name()) //nolint:errcheck // best-effort cleanup after a rename
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close() //nolint:errcheck // the write error is the one to report
		return fmt.Errorf("write eval results: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("write eval results: %w", err)
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil { //nolint:gosec // committed file, world-readable like the skills index
		return fmt.Errorf("write eval results: %w", err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("write eval results: %w", err)
	}
	return nil
}

// CacheInputs is everything that determines a run's result. Any change produces a
// different key, so a stored run is reused only when none of them changed.
type CacheInputs struct {
	// Digest and CasesDigest cover the skill, and the cases with their fixtures,
	// rubrics, graders and assertions.
	Digest, CasesDigest string
	Runner              string
	// RunnerFingerprint holds the runner's own settings (command, runs per case,
	// judge model, extra arguments); see Fingerprinter.
	RunnerFingerprint string
	Harness, Model    string
	Ablation          bool
	// AllowExec changes how command_exit assertions grade.
	AllowExec   bool
	ToolVersion string
}

// CacheKey derives the key that decides whether a stored run can be reused.
func CacheKey(in CacheInputs) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("v%d\n%s\n%s\n%s\n%s\n%s\n%s\n%s\nexec=%s\nversion=%s",
		StoreSchemaVersion, in.Digest, in.CasesDigest, in.Runner, in.RunnerFingerprint, in.Harness, in.Model,
		strconv.FormatBool(in.Ablation), strconv.FormatBool(in.AllowExec), in.ToolVersion)))
	return "sha256:" + hex.EncodeToString(sum[:])
}
