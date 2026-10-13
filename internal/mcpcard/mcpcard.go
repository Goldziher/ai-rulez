// Package mcpcard builds MCP Server Cards from the remote `[[mcp_servers]]`
// entries of a project.
//
// A Server Card (SEP-2127, Final; the schema lives in the experimental
// modelcontextprotocol/experimental-ext-server-card extension, pinned in
// tests/conformance) describes a remote server only: its identity and the
// HTTP endpoints a client connects to. It cannot describe a stdio server, so
// those are skipped with a note. Header and environment values are never
// written: a header becomes an input the client asks the user for, marked
// secret when its value is.
package mcpcard

import (
	"encoding/json"
	"errors"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/samber/oops"
)

// Identifiers of the card.
const (
	// MediaType is the media type of a card (an ARD entry type).
	MediaType = "application/mcp-server-card+json"
	// SchemaURL is the only value of a card's `$schema`.
	SchemaURL = "https://static.modelcontextprotocol.io/schemas/v1/server-card.schema.json"
	// SpecCommit is the commit of modelcontextprotocol/experimental-ext-server-card
	// whose schema.json the conformance tests pin.
	SpecCommit = "526201bbc80231daa40ffcdecfc9da4e54e5dc93"
	// DefaultVersion is the card version when the project has none.
	DefaultVersion = "0.0.0"
	// MaxDescription is the longest card description, in characters.
	MaxDescription = 100
)

// Remote transport types of the schema.
const (
	TypeStreamableHTTP = "streamable-http"
	TypeSSE            = "sse"
	transportStdio     = "stdio"
)

var (
	namespacePattern = regexp.MustCompile(`^[a-zA-Z0-9.-]+$`)
	unsafeName       = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)
)

// Header is a header of a remote server. Its value is never carried.
type Header struct {
	Name string
	// Secret marks a header whose value is a credential.
	Secret bool
}

// Server is one `[[mcp_servers]]` entry, reduced to what a card can use.
type Server struct {
	Name, Description string
	// Transport is stdio, http or sse; empty means http with a URL, stdio without.
	Transport string
	URL       string
	Command   string
	Args      []string
	Headers   []Header
}

// Options are what a card needs besides the servers.
type Options struct {
	// Namespace is the reverse-DNS prefix of the card names ("com.example").
	Namespace string
	// Version is the version of every card; DefaultVersion when empty.
	Version string
	// HTTPSOnly rejects http:// URLs, as the public ARD manifest does.
	HTTPSOnly bool
}

// Input is a header (or URL variable) the client supplies.
type Input struct {
	Name       string `json:"name"`
	IsRequired bool   `json:"isRequired,omitempty"`
	IsSecret   bool   `json:"isSecret,omitempty"`
}

// Remote is one endpoint of the server.
type Remote struct {
	Type    string  `json:"type"`
	URL     string  `json:"url"`
	Headers []Input `json:"headers,omitempty"`
}

// Card is an MCP Server Card.
type Card struct {
	Schema      string   `json:"$schema"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Version     string   `json:"version"`
	Remotes     []Remote `json:"remotes"`
}

// JSON is the card as indented JSON ending in a newline.
func (c Card) JSON() ([]byte, error) {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return nil, oops.Wrapf(err, "encode the MCP server card %s", c.Name)
	}
	return append(data, '\n'), nil
}

// Map is the card as a generic JSON object, for embedding in another document.
func (c Card) Map() (map[string]any, error) {
	data, err := json.Marshal(c)
	if err != nil {
		return nil, oops.Wrapf(err, "encode the MCP server card %s", c.Name)
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, oops.Wrapf(err, "decode the MCP server card %s", c.Name)
	}
	return out, nil
}

// Build returns the card of every remote server, sorted by name, and a note for
// each server skipped because it is not remote or its url has a query, a fragment
// or an unresolved placeholder. Servers that are remote but
// unusable (an unknown transport, a bad or credential-bearing URL, a name used
// twice) fail the build, with every problem reported.
func Build(servers []Server, opts Options) ([]Card, []string, error) {
	if !namespacePattern.MatchString(opts.Namespace) {
		return nil, nil, oops.Hint("set the namespace emitter option, or [ard] publisher, to a reverse-DNS name such as com.example").
			Errorf("the MCP server card namespace %q must be a reverse-DNS name (letters, digits, '.' and '-')", opts.Namespace)
	}
	version := opts.Version
	if version == "" {
		version = DefaultVersion
	}
	var (
		cards []Card
		notes []string
		errs  []error
		seen  = map[string]bool{}
	)
	for i := range servers {
		s := &servers[i]
		typ, remote, err := remoteType(s)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if !remote {
			notes = append(notes, `MCP server "`+s.Name+`" is stdio: a server card describes remote servers only, so it has none`)
			continue
		}
		if why := unpublishableURL(s.URL); why != "" {
			notes = append(notes, `MCP server "`+s.Name+`" is skipped: its url `+why+`, and a server card is public`)
			continue
		}
		if err := checkURL(s, opts.HTTPSOnly); err != nil {
			errs = append(errs, err)
			continue
		}
		name := opts.Namespace + "/" + safeName(s.Name)
		if seen[name] {
			errs = append(errs, oops.Errorf("MCP server %q is listed twice (card name %s)", s.Name, name))
			continue
		}
		seen[name] = true
		cards = append(cards, Card{
			Schema: SchemaURL, Name: name, Description: description(s), Version: version,
			Remotes: []Remote{{Type: typ, URL: s.URL, Headers: headers(s.Headers)}},
		})
	}
	if len(errs) > 0 {
		return nil, nil, errors.Join(errs...)
	}
	sort.Slice(cards, func(i, j int) bool { return cards[i].Name < cards[j].Name })
	return cards, notes, nil
}

// remoteType is the schema transport type of a server, and whether it is remote.
func remoteType(s *Server) (typ string, remote bool, err error) {
	switch strings.ToLower(s.Transport) {
	case "":
		if s.URL == "" {
			return "", false, nil
		}
		return TypeStreamableHTTP, true, nil
	case transportStdio:
		return "", false, nil
	case "http", TypeStreamableHTTP:
		return TypeStreamableHTTP, true, nil
	case TypeSSE:
		return TypeSSE, true, nil
	}
	return "", false, oops.Errorf("MCP server %q has the unknown transport %q", s.Name, s.Transport)
}

// unpublishableURL says why a url cannot be written into a public card as it
// is: a query or fragment usually carries a key, and an unresolved ${...}
// placeholder would be published verbatim. "" means it can.
func unpublishableURL(raw string) string {
	switch {
	case strings.Contains(raw, "${"):
		return "holds an unresolved ${...} placeholder"
	case strings.ContainsAny(raw, "?#"):
		return "has a query or fragment, which may carry a credential"
	}
	return ""
}

func checkURL(s *Server, httpsOnly bool) error {
	u, err := url.Parse(s.URL)
	switch {
	case err != nil || u.Host == "" || u.Scheme == "":
		return oops.Errorf("MCP server %q: the url must be an absolute http or https URL", s.Name)
	case u.Scheme != "https" && u.Scheme != "http":
		return oops.Errorf("MCP server %q: the url scheme must be http or https, not %q", s.Name, u.Scheme)
	case httpsOnly && u.Scheme != "https":
		return oops.Errorf("MCP server %q: the url must be an https URL", s.Name)
	case u.User != nil:
		return oops.Errorf("MCP server %q: the url holds credentials, and a server card is public", s.Name)
	case strings.ContainsAny(s.URL, " \t\r\n"):
		return oops.Errorf("MCP server %q: the url holds whitespace", s.Name)
	}
	return nil
}

// headers lists the header inputs by name. A header is required because the
// project sends it; its value is dropped.
func headers(in []Header) []Input {
	if len(in) == 0 {
		return nil
	}
	out := make([]Input, 0, len(in))
	for _, h := range in {
		out = append(out, Input{Name: h.Name, IsRequired: true, IsSecret: h.Secret})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func safeName(name string) string {
	n := strings.Trim(unsafeName.ReplaceAllString(name, "-"), "-")
	if n == "" {
		return "server"
	}
	return n
}

// description is the server's description, cut to the schema limit, or a
// generic one when the server has none (the schema requires one).
func description(s *Server) string {
	d := strings.TrimSpace(s.Description)
	if d == "" {
		d = "MCP server " + s.Name
	}
	if utf8.RuneCountInString(d) > MaxDescription {
		d = string([]rune(d)[:MaxDescription])
	}
	return d
}

// NamespaceFromDomain reverses a domain: docs.acme.test is test.acme.docs.
func NamespaceFromDomain(domain string) string {
	labels := strings.Split(strings.ToLower(strings.Trim(strings.TrimSpace(domain), ".")), ".")
	for i, j := 0, len(labels)-1; i < j; i, j = i+1, j-1 {
		labels[i], labels[j] = labels[j], labels[i]
	}
	return strings.Join(labels, ".")
}

// NamespaceFromRepo is the namespace of a repository: owner/name (GitHub) is
// io.github.owner, github.com/owner/name is io.github.owner, and host/owner/name another host's reversed host followed by the owner. It is empty when the
// repository is not one of those.
func NamespaceFromRepo(repo string) string {
	parts := strings.Split(repo, "/")
	switch len(parts) {
	case 2:
		if parts[0] == "" {
			return ""
		}
		return "io.github." + parts[0]
	case 3:
		if parts[0] == "" {
			return ""
		}
		if strings.EqualFold(parts[0], "github.com") {
			return "io.github." + strings.ToLower(parts[1])
		}
		if parts[1] == "" {
			return ""
		}
		return NamespaceFromDomain(parts[0]) + "." + strings.ToLower(parts[1])
	}
	return ""
}
