package skillsearch

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
)

// Ranking modes of [search] mode and of `search --mode`.
const (
	ModeHybrid = "hybrid"
	ModeVector = "vector"
	// ModeLexical is declared in eval.go.
)

// Fusion strategies of [search] fusion.
const (
	// FusionAuto is the default: hybrid ranks by cosine alone while every skill in scope has a current
	// vector (it beat every fusion on this repository's evaluation) and fuses with RRF otherwise.
	FusionAuto     = "auto"
	FusionRRF      = "rrf"
	FusionWeighted = "weighted"
)

// Searchable fields of a skill, as named in [search] fields.
const (
	FieldName        = "name"
	FieldTriggers    = "triggers"
	FieldKeywords    = "keywords"
	FieldDescription = "description"
)

// CodeConfigInvalid is AR9D0: a bad [search] table.
const CodeConfigInvalid = "AR9D0"

// CodeIndexStale is AR9D1: a committed index that no longer matches.
const CodeIndexStale = "AR9D1"

// Defaults of the [search] table.
const (
	DefaultRRFK           = 60
	DefaultCandidates     = 50
	DefaultQueryTimeoutMS = 800
	DefaultBodyChars      = 1200
	DefaultIndexDir       = "local/search"
	maxBodyChars          = 20000
	maxCandidates         = 1000
	maxQueryTimeoutMS     = 60000
	maxBatchSize          = 2048
)

// Weights are the list weights of RRF, or the convex mix of weighted fusion.
//
//nolint:tagliatelle // config keys are snake_case by project convention
type Weights struct {
	Lexical float64 `yaml:"lexical,omitempty" json:"lexical,omitempty" toml:"lexical,omitempty"`
	Vector  float64 `yaml:"vector,omitempty" json:"vector,omitempty" toml:"vector,omitempty"`
}

// EmbeddingsConfig is [search.embeddings]: a per-feature model override and the
// command provider. The command runs a program, so it is honored only from the
// user config file (or with --allow-exec); a repository config cannot set it.
//
//nolint:tagliatelle // config keys are snake_case by project convention
type EmbeddingsConfig struct {
	// Command is the argv of the command provider (no shell).
	Command []string `yaml:"command,omitempty" json:"command,omitempty" toml:"command,omitempty"`
	// Model overrides [llm] embedding_model for search only.
	Model string `yaml:"model,omitempty" json:"model,omitempty" toml:"model,omitempty"`
	// PassEnv names the environment variables handed to the command (PATH and HOME are always passed).
	PassEnv []string `yaml:"pass_env,omitempty" json:"pass_env,omitempty" toml:"pass_env,omitempty"`
}

// Config is the [search] table of config.toml. The zero value is lexical,
// offline and deterministic.
//
//nolint:tagliatelle // config keys are snake_case by project convention
type Config struct {
	// Mode is lexical (default), hybrid or vector.
	Mode string `yaml:"mode,omitempty" json:"mode,omitempty" toml:"mode,omitempty"`
	// Fields are the skill fields sent to the embedder (default: all four).
	Fields []string `yaml:"fields,omitempty" json:"fields,omitempty" toml:"fields,omitempty"`
	// IndexBody adds the first BodyChars characters of SKILL.md to the embedded text.
	IndexBody bool `yaml:"index_body,omitempty" json:"index_body,omitempty" toml:"index_body,omitempty"`
	BodyChars int  `yaml:"body_chars,omitempty" json:"body_chars,omitempty" toml:"body_chars,omitempty"`
	// Fusion is auto (default), rrf or weighted.
	Fusion  string  `yaml:"fusion,omitempty" json:"fusion,omitempty" toml:"fusion,omitempty"`
	RRFK    int     `yaml:"rrf_k,omitempty" json:"rrf_k,omitempty" toml:"rrf_k,omitempty"`
	Weights Weights `yaml:"weights,omitempty" json:"weights,omitempty" toml:"weights,omitempty"`
	// VectorMinSim is the abstention threshold: a skill whose cosine to the query is below it is not a
	// match, and a vector ranking with none above it returns nothing (0 is off: the nearest skills are
	// always listed). Cosines differ per model: take the value `search --eval` calibrates.
	VectorMinSim float64 `yaml:"vector_min_sim,omitempty" json:"vector_min_sim,omitempty" toml:"vector_min_sim,omitempty"`
	// Candidates is the length of each list before fusion.
	Candidates int `yaml:"candidates,omitempty" json:"candidates,omitempty" toml:"candidates,omitempty"`
	// QueryTimeoutMS bounds the query embedding of one search; on timeout the ranking is lexical.
	QueryTimeoutMS int `yaml:"query_timeout_ms,omitempty" json:"query_timeout_ms,omitempty" toml:"query_timeout_ms,omitempty"`
	// BatchSize is how many texts one embedding call carries (default 64): lower it for a provider
	// that caps or rate-limits batches.
	BatchSize int `yaml:"batch_size,omitempty" json:"batch_size,omitempty" toml:"batch_size,omitempty"`
	// IndexDir is the index directory under the config directory ("local/search" by
	// default, machine-local; any other directory is meant to be committed).
	IndexDir string `yaml:"index_dir,omitempty" json:"index_dir,omitempty" toml:"index_dir,omitempty"`
	// DType is the stored vector type: float32 (default) or float16.
	DType string `yaml:"dtype,omitempty" json:"dtype,omitempty" toml:"dtype,omitempty"`
	// LogQueries records the text of find_skill and search queries, and the skill loaded after
	// each, in <config dir>/local/search-queries.jsonl so `search mine` can turn them into cases.
	// Honored only from the user config file or AI_RULEZ_SEARCH_LOG_QUERIES: a repository config cannot turn it on.
	LogQueries bool              `yaml:"log_queries,omitempty" json:"log_queries,omitempty" toml:"log_queries,omitempty"`
	Embeddings *EmbeddingsConfig `yaml:"embeddings,omitempty" json:"embeddings,omitempty" toml:"embeddings,omitempty"`
}

// Resolved returns the config with every default applied.
func (c Config) Resolved() Config {
	out := c
	if out.Mode == "" {
		out.Mode = ModeLexical
	}
	if len(out.Fields) == 0 {
		out.Fields = []string{FieldName, FieldTriggers, FieldKeywords, FieldDescription}
	}
	if out.BodyChars <= 0 {
		out.BodyChars = DefaultBodyChars
	}
	if out.Fusion == "" {
		out.Fusion = FusionAuto
	}
	if out.RRFK <= 0 {
		out.RRFK = DefaultRRFK
	}
	if out.Weights.Lexical <= 0 {
		out.Weights.Lexical = 1
	}
	if out.Weights.Vector <= 0 {
		out.Weights.Vector = 1
	}
	if out.Candidates <= 0 {
		out.Candidates = DefaultCandidates
	}
	if out.BatchSize <= 0 {
		out.BatchSize = DefaultBatchSize
	}
	if out.QueryTimeoutMS <= 0 {
		out.QueryTimeoutMS = DefaultQueryTimeoutMS
	}
	if out.IndexDir == "" {
		out.IndexDir = DefaultIndexDir
	}
	if out.DType == "" {
		out.DType = DTypeFloat32
	}
	return out
}

// Validate returns the problems of the table, one message each (AR9D0).
func (c *Config) Validate() []string {
	if c == nil {
		return nil
	}
	var p []string
	add := func(format string, a ...any) { p = append(p, fmt.Sprintf(format, a...)) }
	c.validateEnums(add)
	if c.BodyChars < 0 || c.BodyChars > maxBodyChars {
		add("search.body_chars must be between 0 and %d", maxBodyChars)
	}
	c.validateNumbers(add)
	if c.IndexDir != "" {
		if err := checkIndexDir(c.IndexDir); err != "" {
			add("search.index_dir %s", err)
		}
	}
	c.validateEmbeddings(add)
	return p
}

// validateEnums checks the fields that take one of a fixed set of values.
func (c *Config) validateEnums(add func(format string, a ...any)) {
	switch c.Mode {
	case "", ModeLexical, ModeHybrid, ModeVector:
	default:
		add("search.mode %q must be lexical, hybrid or vector", c.Mode)
	}
	switch c.Fusion {
	case "", FusionAuto, FusionRRF, FusionWeighted:
	default:
		add("search.fusion %q must be auto, rrf or weighted", c.Fusion)
	}
	switch c.DType {
	case "", DTypeFloat32, DTypeFloat16:
	default:
		add("search.dtype %q must be float32 or float16", c.DType)
	}
	for _, f := range c.Fields {
		if !slices.Contains([]string{FieldName, FieldTriggers, FieldKeywords, FieldDescription}, f) {
			add("search.fields entry %q must be name, triggers, keywords or description", f)
		}
	}
}

// validateEmbeddings checks the embeddings command table.
func (c *Config) validateEmbeddings(add func(format string, a ...any)) {
	e := c.Embeddings
	if e == nil {
		return
	}
	if len(e.Command) > 0 && strings.TrimSpace(e.Command[0]) == "" {
		add("search.embeddings.command must start with a program")
	}
	for _, name := range e.PassEnv {
		if strings.TrimSpace(name) == "" || strings.ContainsAny(name, "=\x00") {
			add("search.embeddings.pass_env holds an invalid variable name %q", name)
		}
	}
}

// validateNumbers checks the numeric keys of the table.
func (c *Config) validateNumbers(add func(format string, a ...any)) {
	if c.RRFK < 0 {
		add("search.rrf_k must not be negative")
	}
	if c.Weights.Lexical < 0 || c.Weights.Vector < 0 {
		add("search.weights must not be negative")
	}
	if c.VectorMinSim < 0 || c.VectorMinSim > 1 {
		add("search.vector_min_sim must be between 0 and 1")
	}
	if c.Candidates < 0 || c.Candidates > maxCandidates {
		add("search.candidates must be between 0 and %d", maxCandidates)
	}
	if c.BatchSize < 0 || c.BatchSize > maxBatchSize {
		add("search.batch_size must be between 0 and %d", maxBatchSize)
	}
	if c.QueryTimeoutMS < 0 || c.QueryTimeoutMS > maxQueryTimeoutMS {
		add("search.query_timeout_ms must be between 0 and %d", maxQueryTimeoutMS)
	}
}

// checkIndexDir returns why dir cannot be an index directory: it must stay
// inside the config directory, so a committed config cannot point elsewhere.
func checkIndexDir(dir string) string {
	if filepath.IsAbs(dir) || strings.HasPrefix(dir, "/") || strings.HasPrefix(dir, "\\") || strings.Contains(dir, "\x00") {
		return "must be a relative path inside the config directory"
	}
	clean := filepath.ToSlash(filepath.Clean(dir))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || strings.Contains(dir, "..") {
		return "must stay inside the config directory"
	}
	if strings.ContainsRune(dir, ':') {
		return "must be a relative path inside the config directory"
	}
	return ""
}

// CommittedIndexDir reports whether dir (a [search] index_dir, defaults applied)
// is meant to be committed: anything outside the machine-local "local/" tree.
func CommittedIndexDir(dir string) bool {
	if dir == "" {
		return false
	}
	clean := filepath.ToSlash(filepath.Clean(dir))
	return clean != "local" && !strings.HasPrefix(clean, "local/")
}

// IndexPath is the absolute index directory for a config directory.
func (c Config) IndexPath(configDir string) (string, error) {
	r := c.Resolved()
	if msg := checkIndexDir(r.IndexDir); msg != "" {
		return "", fmt.Errorf("search.index_dir %s", msg)
	}
	return filepath.Join(configDir, filepath.FromSlash(r.IndexDir)), nil
}
