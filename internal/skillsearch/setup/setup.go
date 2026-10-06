// Package setup resolves the [search] and [llm] configuration into what a search
// needs: the effective settings, an embedder, the index directory and the query
// log. The CLI and the MCP server share it, so both rank the same way.
package setup

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	toml "github.com/pelletier/go-toml/v2"
	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/llm"
	"github.com/Goldziher/ai-rulez/v5/internal/skillsearch"
)

// ModeEnv overrides [search] mode.
const ModeEnv = "AI_RULEZ_SEARCH_MODE"

// LogQueriesEnv turns the query log on (1 or true) or off for this process. With the user config
// file's [search] log_queries it is the only way to enable the log: a repository cannot.
const LogQueriesEnv = "AI_RULEZ_SEARCH_LOG_QUERIES"

// Options tune Resolve.
type Options struct {
	// Mode overrides the mode (the --mode flag); "" keeps the configured one.
	Mode string
	// AllowExec honours [search.embeddings] command from a repository config
	// (the --allow-exec flag). The command is always honoured from the user config.
	AllowExec bool
	// Getenv resolves environment variables; nil means the process environment.
	Getenv func(string) string
}

// Resolved is the effective search setup.
type Resolved struct {
	// Search is the merged [search] table with defaults applied and the mode override.
	Search skillsearch.Config
	// LLM is the effective [llm] table (trust rule and AI_RULEZ_LLM_* applied).
	LLM llm.Config
	// ConfigDir is the project's config directory ("" without a project).
	ConfigDir string
	// Command is the command provider argv ("" entries never occur); empty means the LLM provider.
	Command []string
	PassEnv []string
	// Model is the embedding model: [search.embeddings] model, else [llm] embedding_model.
	Model string
	// Notes are things the user should know: ignored keys.
	Notes  []string
	getenv func(string) string

	logOnce  sync.Once
	queryLog *skillsearch.QueryLog
}

// Resolve layers the repository [search] table, the user config file and the
// environment. A repository config cannot run a program: its
// [search.embeddings] command is ignored unless AllowExec is set.
func Resolve(cfg *config.Config, opts Options) (*Resolved, error) {
	getenv := opts.Getenv
	if getenv == nil {
		getenv = func(name string) string { return ambient.Getenv(nil, name) }
	}
	r := &Resolved{getenv: getenv}
	var repo *skillsearch.Config
	if cfg != nil {
		repo = cfg.Search
		r.ConfigDir = cfg.ConfigDir
	}
	user, err := loadUser(config.UserConfigFile(getenv))
	if err != nil {
		return nil, err
	}
	merged := merge(repo, user)
	// The query log records what users ask, so only user scope may start it.
	if repo != nil && repo.LogQueries && !(user != nil && user.LogQueries) {
		r.Notes = append(r.Notes, "search.log_queries in the repository config is ignored: a repository cannot start recording queries; set it in the user config file or "+LogQueriesEnv+"=1")
	}
	merged.LogQueries = user != nil && user.LogQueries
	if v := strings.ToLower(strings.TrimSpace(getenv(LogQueriesEnv))); v != "" {
		merged.LogQueries = v == "1" || v == "true"
	}
	if problems := merged.Validate(); len(problems) > 0 {
		return nil, oops.Code(skillsearch.CodeConfigInvalid).Hint("Fix the [search] table; 'ai-rulez validate' lists every problem").
			Errorf("%s: %s", skillsearch.CodeConfigInvalid, strings.Join(problems, "; "))
	}
	// Who may run a program: the user config, or an explicit flag.
	if e := merged.Embeddings; e != nil && len(e.Command) > 0 {
		fromUser := user != nil && user.Embeddings != nil && len(user.Embeddings.Command) > 0
		switch {
		case fromUser || opts.AllowExec:
			r.Command, r.PassEnv = e.Command, e.PassEnv
		default:
			r.Notes = append(r.Notes, "search.embeddings.command in the repository config is ignored: a repository cannot run a program; set it in the user config file or pass --allow-exec")
		}
	}
	if v := strings.TrimSpace(getenv(ModeEnv)); v != "" {
		merged.Mode = v
	}
	if opts.Mode != "" {
		merged.Mode = opts.Mode
	}
	if problems := merged.Validate(); len(problems) > 0 {
		return nil, oops.Code(skillsearch.CodeConfigInvalid).Errorf("%s: %s", skillsearch.CodeConfigInvalid, strings.Join(problems, "; "))
	}
	r.Search = merged.Resolved()
	res, err := cfg.ResolveLLM(getenv)
	if err != nil {
		return nil, err //nolint:wrapcheck // already contextual
	}
	r.LLM = res.Config
	if len(res.Ignored) > 0 {
		r.Notes = append(r.Notes, llm.IgnoredKeysMessage(res.Ignored))
	}
	r.Model = r.LLM.EmbeddingModel
	if e := merged.Embeddings; e != nil && e.Model != "" {
		r.Model = e.Model
	}
	return r, nil
}

func loadUser(path string) (*skillsearch.Config, error) {
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path) //nolint:gosec // the user's own config file
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, oops.With("path", path).Wrapf(err, "read user config")
	}
	var doc struct {
		Search *skillsearch.Config `toml:"search"`
	}
	if err := toml.Unmarshal(data, &doc); err != nil {
		return nil, oops.With("path", path).Wrapf(err, "parse user config")
	}
	return doc.Search, nil
}

// merge lays user over repo field by field: a user value that is set wins.
func merge(repo, user *skillsearch.Config) skillsearch.Config {
	var out skillsearch.Config
	if repo != nil {
		out = *repo
	}
	if user == nil {
		return out
	}
	if user.Mode != "" {
		out.Mode = user.Mode
	}
	if len(user.Fields) > 0 {
		out.Fields = user.Fields
	}
	out.IndexBody = out.IndexBody || user.IndexBody
	if user.BodyChars > 0 {
		out.BodyChars = user.BodyChars
	}
	if user.Fusion != "" {
		out.Fusion = user.Fusion
	}
	if user.RRFK > 0 {
		out.RRFK = user.RRFK
	}
	if user.Weights.Lexical > 0 {
		out.Weights.Lexical = user.Weights.Lexical
	}
	if user.Weights.Vector > 0 {
		out.Weights.Vector = user.Weights.Vector
	}
	if user.Candidates > 0 {
		out.Candidates = user.Candidates
	}
	if user.BatchSize > 0 {
		out.BatchSize = user.BatchSize
	}
	if user.QueryTimeoutMS > 0 {
		out.QueryTimeoutMS = user.QueryTimeoutMS
	}
	if user.IndexDir != "" {
		out.IndexDir = user.IndexDir
	}
	if user.DType != "" {
		out.DType = user.DType
	}
	if user.Embeddings != nil {
		e := skillsearch.EmbeddingsConfig{}
		if out.Embeddings != nil {
			e = *out.Embeddings
		}
		if len(user.Embeddings.Command) > 0 {
			e.Command, e.PassEnv = user.Embeddings.Command, user.Embeddings.PassEnv
		}
		if user.Embeddings.Model != "" {
			e.Model = user.Embeddings.Model
		}
		out.Embeddings = &e
	}
	return out
}

// IndexPath is the absolute index directory.
func (r *Resolved) IndexPath() (string, error) {
	if r.ConfigDir == "" {
		return "", oops.Errorf("search index: no project config directory")
	}
	return r.Search.IndexPath(r.ConfigDir)
}

// QueryLog returns the query log when log_queries is on in user scope, else nil. The same
// log is returned on every call, so its full-log warning is given once.
func (r *Resolved) QueryLog(scan skillsearch.SecretScanner) *skillsearch.QueryLog {
	if !r.Search.LogQueries || r.ConfigDir == "" {
		return nil
	}
	r.logOnce.Do(func() {
		r.queryLog = &skillsearch.QueryLog{Path: skillsearch.LogPath(r.ConfigDir), Scanner: scan}
	})
	return r.queryLog
}

// Provider describes where embeddings come from, for `search status` and --dry-run.
type Provider struct {
	// Fingerprint keys vector reuse; Host is shown to the user.
	Fingerprint string
	Host        string
	Model       string
	Network     bool
	Command     bool
}

// Describe reports the provider without contacting it.
func (r *Resolved) Describe() Provider {
	if len(r.Command) > 0 {
		return Provider{Fingerprint: "command:" + r.Command[0], Host: "(local command " + filepath.Base(r.Command[0]) + ")", Model: r.Model, Command: true}
	}
	host := "default"
	if u, err := url.Parse(r.LLM.BaseURL); err == nil && u.Host != "" {
		host = u.Host
	}
	provider := r.LLM.Provider
	if provider == "" {
		provider = "openai"
	}
	return Provider{Fingerprint: provider + "@" + host, Host: host, Model: r.Model, Network: true}
}

// Embedder builds the embedder and a function that releases it. The network
// gate, budget and cache of internal/llm apply to every call; with
// allow_network false each call fails with a network-disabled error, which the
// ranker reports as degraded. A command provider needs no network.
func (r *Resolved) Embedder() (skillsearch.Embedder, func(), error) {
	p := r.Describe()
	if p.Command {
		return &skillsearch.CommandEmbedder{
			Argv: r.Command, Dir: filepath.Dir(r.ConfigDir), PassEnv: r.PassEnv, ModelName: r.Model, Getenv: r.getenv,
		}, func() {}, nil
	}
	if r.Model == "" {
		return nil, func() {}, oops.Hint("Set [llm] embedding_model (or [search.embeddings] model), or a command provider").
			Errorf("no embedding model is configured")
	}
	managed, err := llm.New(r.LLM, llm.Options{ConfigDir: r.ConfigDir, Getenv: r.getenv})
	if err != nil {
		return nil, func() {}, oops.Wrapf(err, "set up the embedding client")
	}
	return &skillsearch.LLMEmbedder{Client: managed, ModelName: r.Model, RequestModel: r.requestModel(), Provider: p.Fingerprint}, func() { _ = managed.Close() }, nil //nolint:errcheck // nothing to do on close
}

// requestModel is the model to put on the request: only a [search.embeddings] model
// overrides [llm] embedding_model, and with the literllm backend it takes the provider
// prefix that backend routes on.
func (r *Resolved) requestModel() string {
	if r.Model == r.LLM.EmbeddingModel {
		return ""
	}
	if r.LLM.Provider != "" && !strings.Contains(r.Model, "/") && llm.ResolveBackend(r.LLM.Backend) == llm.BackendLiterLLM {
		return r.LLM.Provider + "/" + r.Model
	}
	return r.Model
}

// Wants reports whether the mode needs vectors.
func (r *Resolved) Wants() bool { return r.Search.Mode != skillsearch.ModeLexical }

// String is a one-line summary for logs; it never holds a key.
func (r *Resolved) String() string {
	p := r.Describe()
	return fmt.Sprintf("mode=%s provider=%s model=%s", r.Search.Mode, p.Fingerprint, p.Model)
}
