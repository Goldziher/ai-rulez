package pluginimport

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
)

// Marketplace index formats, named after the harness whose index it is.
const (
	IndexClaude = "claude"
	IndexCodex  = "codex"
	IndexCursor = "cursor"
)

// IndexFileName is the name every marketplace index file has.
const IndexFileName = "marketplace.json"

// Source kinds of a marketplace entry.
const (
	// SourcePath is a directory of the marketplace repository itself.
	SourcePath = "path"
	// SourceRepo is another git repository, optionally a subdirectory of it.
	SourceRepo = "repo"
)

// IndexLocations lists where each format keeps its index below a repository root.
func IndexLocations() []IndexLocation {
	return []IndexLocation{
		{IndexClaude, ".claude-plugin/" + IndexFileName},
		{IndexCodex, ".agents/plugins/" + IndexFileName},
		{IndexCursor, ".cursor-plugin/" + IndexFileName},
	}
}

// IndexLocation is a format and the index file path it uses.
type IndexLocation struct{ Format, Path string }

// FindIndex locates the marketplace index below dir and returns its format and
// absolute path. The first format of IndexLocations that has a regular file wins.
func FindIndex(dir string) (format, file string, err error) {
	for _, loc := range IndexLocations() {
		p, joinErr := SafeJoin(dir, loc.Path)
		if joinErr != nil {
			continue
		}
		if info, statErr := os.Lstat(p); statErr == nil && info.Mode().IsRegular() {
			return loc.Format, p, nil
		}
	}
	var names []string
	for _, loc := range IndexLocations() {
		names = append(names, loc.Path)
	}
	return "", "", oops.With("dir", dir).Errorf("no marketplace index found in %s (looked for %s)", dir, strings.Join(names, ", "))
}

// Source says where a marketplace entry's plugin lives.
type Source struct {
	Kind string
	// Path is, for SourcePath, the plugin directory relative to the marketplace
	// root (the index's pluginRoot already applied), slash-separated.
	Path string
	// URL, Ref, SHA and Subdir are, for SourceRepo, the repository, the ref and
	// the full commit the index pins, and the directory of the plugin inside it.
	URL, Ref, SHA, Subdir string
}

// Describe is the source as it is shown and recorded: no credentials.
func (s Source) Describe() string {
	if s.Kind == SourcePath {
		return "./" + s.Path
	}
	d := redactedURL(s.URL)
	if s.Subdir != "" {
		d += "#" + s.Subdir
	}
	switch {
	case s.SHA != "":
		d += "@" + s.SHA
	case s.Ref != "":
		d += "@" + s.Ref
	}
	return d
}

func redactedURL(raw string) string {
	if i := strings.Index(raw, "://"); i >= 0 {
		rest := raw[i+3:]
		if at := strings.LastIndex(strings.SplitN(rest, "/", 2)[0], "@"); at >= 0 {
			return raw[:i+3] + rest[at+1:]
		}
	}
	return raw
}

// Entry is one plugin of a marketplace index.
type Entry struct {
	Name        string
	Description string
	Version     string
	Source      Source
	// Strict is the entry's strict flag; nil is the default (true): the plugin's own
	// manifest is the authority and the entry only supplements it.
	Strict *bool
	// Inline holds the component fields the entry itself declares (skills,
	// agents, commands, hooks, mcpServers), which a non-strict entry uses in
	// place of a plugin manifest.
	Inline map[string]json.RawMessage
	// Problem says why the entry cannot be installed ("" when it can).
	Problem string
}

// Index is a parsed marketplace index.
type Index struct {
	Format  string
	Name    string
	Plugins []Entry
}

// Find returns the entry called name.
func (ix *Index) Find(name string) (*Entry, bool) {
	for i := range ix.Plugins {
		if ix.Plugins[i].Name == name {
			return &ix.Plugins[i], true
		}
	}
	return nil, false
}

// Names lists the entry names in index order.
func (ix *Index) Names() []string {
	out := make([]string, len(ix.Plugins))
	for i := range ix.Plugins {
		out[i] = ix.Plugins[i].Name
	}
	return out
}

type rawIndex struct {
	Name     string            `json:"name"`
	Root     string            `json:"pluginRoot"`
	Metadata json.RawMessage   `json:"metadata"`
	Plugins  []json.RawMessage `json:"plugins"`
}

type rawEntry struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Version     json.RawMessage `json:"version"`
	Source      json.RawMessage `json:"source"`
	Strict      *bool           `json:"strict"`
}

// inlineFields are the component fields a non-strict entry can declare.
func inlineFields() []string {
	return []string{dirSkills, dirAgents, dirCommands, "hooks", "mcpServers", "rules"}
}

// ParseIndex parses the bytes of a marketplace index of the given format. It
// never fails on one bad entry: that entry carries its Problem and the rest stay
// usable. A document that is not a marketplace at all is an error.
func ParseIndex(data []byte, format string) (*Index, error) {
	if len(data) > MaxIndexBytes {
		return nil, oops.Errorf("the marketplace index is larger than %d bytes", MaxIndexBytes)
	}
	var raw rawIndex
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, oops.Wrapf(err, "the marketplace index is not valid JSON")
	}
	if len(raw.Plugins) > MaxIndexPlugins {
		return nil, oops.Errorf("the marketplace lists %d plugins, more than the limit of %d", len(raw.Plugins), MaxIndexPlugins)
	}
	root := raw.Root
	if root == "" && len(raw.Metadata) > 0 {
		var meta struct {
			PluginRoot string `json:"pluginRoot"`
		}
		if json.Unmarshal(raw.Metadata, &meta) == nil {
			root = meta.PluginRoot
		}
	}
	ix := &Index{Format: format, Name: raw.Name}
	cleanRoot := ""
	if root != "" {
		var err error
		if cleanRoot, err = CleanRel(root); err != nil {
			return nil, oops.Wrapf(err, "metadata.pluginRoot")
		}
		if cleanRoot == "." {
			cleanRoot = ""
		}
	}
	seen := map[string]bool{}
	for i, msg := range raw.Plugins {
		e := parseEntry(msg, cleanRoot)
		if e.Name == "" {
			e.Name = fmt.Sprintf("(entry %d)", i)
		}
		if seen[e.Name] {
			e.Problem = "duplicate plugin name in the marketplace"
		}
		seen[e.Name] = true
		ix.Plugins = append(ix.Plugins, e)
	}
	return ix, nil
}

func parseEntry(msg json.RawMessage, pluginRoot string) Entry {
	var r rawEntry
	if err := json.Unmarshal(msg, &r); err != nil {
		return Entry{Problem: "the entry is not a JSON object"}
	}
	e := Entry{Name: r.Name, Description: r.Description, Strict: r.Strict}
	var version string
	if json.Unmarshal(r.Version, &version) == nil {
		e.Version = version
	}
	if !ValidName(r.Name) {
		e.Problem = fmt.Sprintf("%q is not a valid plugin name", r.Name)
		return e
	}
	if len(r.Source) == 0 {
		e.Problem = "the entry has no source"
		return e
	}
	src, problem := parseSource(r.Source, pluginRoot)
	e.Source, e.Problem = src, problem
	var all map[string]json.RawMessage
	if json.Unmarshal(msg, &all) == nil {
		for _, f := range inlineFields() {
			if v, ok := all[f]; ok {
				if e.Inline == nil {
					e.Inline = map[string]json.RawMessage{}
				}
				e.Inline[f] = v
			}
		}
	}
	return e
}

type rawSource struct {
	Kind   string `json:"source"`
	Type   string `json:"type"`
	Repo   string `json:"repo"`
	URL    string `json:"url"`
	Path   string `json:"path"`
	Subdir string `json:"subdir"`
	Ref    string `json:"ref"`
	SHA    string `json:"sha"`
}

// parseSource reads the three shapes an entry's source takes: a relative path
// string, a {source: github|url|git|git-subdir, ...} object and a
// {source: local, path} object.
func parseSource(msg json.RawMessage, pluginRoot string) (src Source, problem string) {
	var asString string
	if json.Unmarshal(msg, &asString) == nil {
		return pathSource(asString, pluginRoot)
	}
	var r rawSource
	if err := json.Unmarshal(msg, &r); err != nil {
		return Source{}, "the source is neither a path nor an object"
	}
	kind := r.Kind
	if kind == "" {
		kind = r.Type
	}
	switch kind {
	case "local", "path":
		return pathSource(r.Path, pluginRoot)
	case "github":
		if !githubRepo(r.Repo) {
			return Source{}, fmt.Sprintf("%q is not an owner/repo", r.Repo)
		}
		return repoSource(config.PluginGitURL(r.Repo), r.Ref, r.SHA, firstNonEmpty(r.Path, r.Subdir))
	case "url", "git":
		return repoSource(r.URL, r.Ref, r.SHA, firstNonEmpty(r.Path, r.Subdir))
	case "git-subdir":
		return repoSource(r.URL, r.Ref, r.SHA, firstNonEmpty(r.Path, r.Subdir))
	case "":
		return Source{}, "the source object names no type"
	}
	return Source{}, fmt.Sprintf("source type %q is not supported (use a path, github, url, git or git-subdir)", kind)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func githubRepo(repo string) bool {
	return config.ClassifyPluginSource(repo) == config.PluginSourceGitHub && !strings.Contains(repo, "://") && !strings.HasPrefix(repo, ".")
}

func pathSource(p, pluginRoot string) (src Source, problem string) {
	explicit := strings.HasPrefix(p, "./")
	clean, err := CleanRel(p)
	if err != nil {
		return Source{}, err.Error()
	}
	if pluginRoot != "" && !explicit {
		clean = path.Join(pluginRoot, clean)
	}
	if clean == "." {
		return Source{}, "the source is the marketplace root itself; name a plugin directory"
	}
	return Source{Kind: SourcePath, Path: clean}, ""
}

func repoSource(rawURL, ref, sha, subdir string) (src Source, problem string) {
	if rawURL == "" {
		return Source{}, "the source has no url or repo"
	}
	if reason := config.PluginSourceProblem(rawURL); reason != "" {
		return Source{}, "the source url " + reason
	}
	if config.ClassifyPluginSource(rawURL) == config.PluginSourceLocal {
		return Source{}, "a plugin repository must be a git URL or owner/repo"
	}
	if ref != "" {
		if reason := config.PluginRefProblem(ref); reason != "" {
			return Source{}, "the source ref " + reason
		}
	}
	if sha != "" && !lockfile.IsFullSHA(sha) {
		return Source{}, "the source sha must be a full 40-character lowercase commit hash"
	}
	if subdir != "" {
		clean, err := CleanRel(subdir)
		if err != nil {
			return Source{}, "the source path: " + err.Error()
		}
		if reason := config.PluginPathProblem(clean); reason != "" {
			return Source{}, "the source path " + reason
		}
		if clean == "." {
			clean = ""
		}
		subdir = clean
	}
	return Source{Kind: SourceRepo, URL: rawURL, Ref: ref, SHA: sha, Subdir: subdir}, ""
}

// ReadIndex finds and parses the index below a marketplace directory.
func ReadIndex(dir string) (*Index, error) {
	format, file, err := FindIndex(dir)
	if err != nil {
		return nil, err
	}
	data, err := readRegular(file, MaxIndexBytes)
	if err != nil {
		return nil, err
	}
	ix, err := ParseIndex(data, format)
	if err != nil {
		return nil, oops.With("file", filepath.ToSlash(file)).Wrap(err)
	}
	return ix, nil
}

// ReadIndexFile parses the single marketplace.json file in dir (the cache
// directory of an https marketplace), taking the format from its content: an
// object source of type "local" is Codex's.
func ReadIndexFile(dir string) (*Index, error) {
	data, err := readRegular(filepath.Join(dir, IndexFileName), MaxIndexBytes)
	if err != nil {
		return nil, err
	}
	format := IndexClaude
	if strings.Contains(string(data), `"local"`) {
		format = IndexCodex
	}
	return ParseIndex(data, format)
}

// SortedNames returns the entry names sorted, for display.
func (ix *Index) SortedNames() []string {
	names := ix.Names()
	sort.Strings(names)
	return names
}
