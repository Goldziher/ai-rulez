package skillsearch

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/Goldziher/ai-rulez/v5/internal/safefs"
	"github.com/samber/oops"
	"gopkg.in/yaml.v3"
)

// Query log. It is opt-in ([search] log_queries = true) because it records what
// an agent asked: query text, which can hold user data. It is a separate local
// file, never sent anywhere, never committed (it lives under local/), and the
// usage log, which holds identifiers only, is untouched. Only user scope turns it
// on: a repository must not be able to start recording what its users ask.
const (
	// QueryLogFile is the log's name inside <config dir>/local.
	QueryLogFile = "search-queries.jsonl"

	queryLogVersion = 1
	maxLoggedQuery  = 512
	maxLogBytes     = 8 << 20
	eventQuery      = "query"
	eventLoaded     = "loaded"
	minedTag        = "mined"
)

// LogEntry is one line of the query log.
//
//nolint:tagliatelle // log keys are snake_case like the other local files
type LogEntry struct {
	Version int      `json:"v"`
	Time    string   `json:"ts"`
	Event   string   `json:"event"`
	Session string   `json:"session,omitempty"`
	Query   string   `json:"query,omitempty"`
	Ranking string   `json:"ranking,omitempty"`
	Results []string `json:"results,omitempty"`
	Skill   string   `json:"skill,omitempty"`
}

// QueryLog appends entries to the log file. A nil *QueryLog does nothing, so
// callers need no check when logging is off. It is safe for concurrent use.
type QueryLog struct {
	Path    string
	Scanner SecretScanner
	// Now is the clock; nil is the wall clock.
	Now ambient.Clock
	// Log receives the log's warning; nil is the CLI's logger.
	Log logger.Logger

	mu     sync.Mutex
	warned bool
}

// LogPath is the query log of a config directory.
func LogPath(configDir string) string { return filepath.Join(configDir, "local", QueryLogFile) }

func sessionKey(s string) string {
	if s == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:6])
}

// Query records a query and the ids it returned. A query that looks like it
// holds a secret is not recorded.
func (l *QueryLog) Query(session, query, ranking string, results []string) {
	if l == nil || strings.TrimSpace(query) == "" {
		return
	}
	if l.Scanner != nil {
		if _, found := l.Scanner(query); found {
			return
		}
	}
	if len(results) > 5 {
		results = results[:5]
	}
	l.append(&LogEntry{Event: eventQuery, Session: sessionKey(session), Query: truncateBytes(strings.Join(strings.Fields(query), " "), maxLoggedQuery), Ranking: ranking, Results: results})
}

// Loaded records that a skill was loaded in a session, which labels the query
// that session asked last.
func (l *QueryLog) Loaded(session, skill string) {
	if l == nil || skill == "" {
		return
	}
	l.append(&LogEntry{Event: eventLoaded, Session: sessionKey(session), Skill: skill})
}

func truncateBytes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	s = s[:n]
	for s != "" && !utf8.ValidString(s) {
		s = s[:len(s)-1]
	}
	return s
}

func (l *QueryLog) append(e *LogEntry) {
	l.mu.Lock()
	defer l.mu.Unlock()
	e.Version, e.Time = queryLogVersion, l.Now.Now().UTC().Format(time.RFC3339)
	line, err := json.Marshal(e)
	if err != nil {
		return
	}
	// The log sits in a directory a repository can plant symlinks in; safefs never follows one.
	f, err := safefs.OpenAppend(l.Path)
	if err != nil {
		l.warnOnce("cannot open the search query log; not recording queries", "path", l.Path, "error", err.Error())
		return
	}
	defer f.Close() //nolint:errcheck // logging must never fail a search
	if info, statErr := f.Stat(); statErr == nil && info.Size() > maxLogBytes {
		l.warnOnce("The search query log is full; not recording more queries (mine it, then run 'ai-rulez search mine --purge')", "path", l.Path, "limit_mib", maxLogBytes>>20)
		return
	}
	_, _ = f.Write(append(line, '\n')) //nolint:errcheck // logging must never fail a search
}

func (l *QueryLog) warnOnce(msg string, args ...any) {
	if l.warned {
		return
	}
	l.warned = true
	logger.Or(l.Log).Warn(msg, args...)
}

// ReadLog reads the entries of a log file; a missing file is an empty log.
// Lines that do not parse are skipped.
func ReadLog(path string) ([]LogEntry, error) {
	raw, err := safefs.ReadRegular(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, oops.Wrapf(err, "read the query log")
	}
	var out []LogEntry
	sc := bufio.NewScanner(bytes.NewReader(raw))
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		var e LogEntry
		if json.Unmarshal(sc.Bytes(), &e) == nil && (e.Event == eventQuery || e.Event == eventLoaded) {
			out = append(out, e)
		}
	}
	return out, nil
}

// MineOptions configures Mine.
type MineOptions struct {
	// Known are the skill ids that exist; a loaded skill outside it is not a label.
	Known []string
	// MinCount is how many times a query must have been followed by the same
	// skill (default 1).
	MinCount int
}

// Mined is the outcome of Mine.
type Mined struct {
	Cases []Case
	// Queries is the number of distinct queries in the log.
	Queries int
	// Unlabelled queries were never followed by a load in their session.
	Unlabelled int
	// Ambiguous queries were followed by different skills with no clear winner.
	Ambiguous int
	// Below queries had a label seen fewer than MinCount times.
	Below int
}

func normQuery(q string) string { return strings.ToLower(strings.Join(strings.Fields(q), " ")) }

// Mine turns a query log into candidate cases. The label of a query is the
// first skill loaded in the same session after it and before the next query:
// a weak label (the agent chose the skill, which does not make it right), so
// the cases carry the tag "mined" and are meant to be reviewed before they gate
// anything.
func Mine(entries []LogEntry, o MineOptions) Mined {
	known := map[string]bool{}
	for _, k := range o.Known {
		known[k] = true
	}
	minCount := max(o.MinCount, 1)
	type tally struct {
		query  string
		labels map[string]int
	}
	byQuery := map[string]*tally{}
	var order []string
	current := map[string]string{} // session -> normalized query awaiting a label
	seen := map[string]bool{}      // session+query already labeled by a load
	for i := range entries {
		e := &entries[i]
		switch e.Event {
		case eventQuery:
			n := normQuery(e.Query)
			if n == "" {
				continue
			}
			if _, ok := byQuery[n]; !ok {
				byQuery[n] = &tally{query: strings.Join(strings.Fields(e.Query), " "), labels: map[string]int{}}
				order = append(order, n)
			}
			current[e.Session] = n
			delete(seen, e.Session)
		case eventLoaded:
			n, ok := current[e.Session]
			if !ok || seen[e.Session] || !known[e.Skill] {
				continue
			}
			seen[e.Session] = true
			byQuery[n].labels[e.Skill]++
		}
	}
	var m Mined
	m.Queries = len(order)
	for _, n := range order {
		t := byQuery[n]
		if len(t.labels) == 0 {
			m.Unlabelled++
			continue
		}
		type kv struct {
			skill string
			n     int
		}
		var ranked []kv
		for s, c := range t.labels {
			ranked = append(ranked, kv{s, c})
		}
		sort.Slice(ranked, func(i, j int) bool {
			if ranked[i].n != ranked[j].n {
				return ranked[i].n > ranked[j].n
			}
			return ranked[i].skill < ranked[j].skill
		})
		if len(ranked) > 1 && ranked[0].n == ranked[1].n {
			m.Ambiguous++
			continue
		}
		if ranked[0].n < minCount {
			m.Below++
			continue
		}
		sum := sha256.Sum256([]byte(n))
		m.Cases = append(m.Cases, Case{
			ID: "mined-" + hex.EncodeToString(sum[:4]), Query: t.query,
			Expect: []Relevant{{ID: ranked[0].skill, Grade: 1}}, Tags: []string{minedTag},
		})
	}
	return m
}

// CasesYAML renders cases as a cases file, for review before use.
func CasesYAML(k int, cases []Case) ([]byte, error) {
	type out struct {
		ID     string   `yaml:"id"`
		Query  string   `yaml:"query"`
		Expect []any    `yaml:"expect"`
		Avoid  []string `yaml:"avoid,omitempty"`
		Role   string   `yaml:"role,omitempty"`
		Tags   []string `yaml:"tags,omitempty"`
	}
	doc := struct {
		Version int   `yaml:"version"`
		K       int   `yaml:"k,omitempty"`
		Cases   []out `yaml:"cases"`
	}{Version: casesSchemaVersion, K: k}
	for i := range cases {
		c := &cases[i]
		doc.Cases = append(doc.Cases, out{ID: c.ID, Query: c.Query, Expect: expectNodes(c.Expect), Avoid: c.Avoid, Role: c.Role, Tags: c.Tags})
	}
	buf := &bytes.Buffer{}
	enc := yaml.NewEncoder(buf)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		return nil, oops.Wrapf(err, "render cases")
	}
	return buf.Bytes(), enc.Close()
}

func expectNodes(rs []Relevant) []any {
	out := make([]any, 0, len(rs))
	for _, r := range rs {
		if r.Graded {
			out = append(out, map[string]any{"id": r.ID, "grade": r.Grade})
		} else {
			out = append(out, r.ID)
		}
	}
	return out
}
