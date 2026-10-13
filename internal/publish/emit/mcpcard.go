package emit

import (
	"strings"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/mcpcard"
)

// MCPServerCardEmitter is the name of the MCP Server Card emitter.
const MCPServerCardEmitter = "mcp-server-card"

// Options of the emitter ([[publish.emitters]] options).
const (
	// optNamespace sets the reverse-DNS prefix of the card names.
	optNamespace = "namespace"
	// optVersion sets the version of every card.
	optVersion = "version"
)

// mcpServerCard writes one MCP Server Card (SEP-2127) per remote
// `[[mcp_servers]]` entry, <server>.json under emit/mcp-server-card/. Stdio
// servers have no card and are reported as findings. Header values never reach
// a card. The schema is the experimental server-card extension's, pinned by the
// conformance tests.
type mcpServerCard struct{}

func init() { register(mcpServerCard{}) }

func (mcpServerCard) Name() string   { return MCPServerCardEmitter }
func (mcpServerCard) Status() string { return StatusVerified }

func (mcpServerCard) Emit(in Input) ([]File, []Finding, error) {
	opts := mcpcard.Options{Namespace: cardNamespace(in), Version: in.Version}
	if v := in.Options[optVersion]; v != "" {
		opts.Version = v
	}
	cards, notes, err := mcpcard.Build(in.Servers, opts)
	if err != nil {
		return nil, nil, oops.Wrapf(err, "build the MCP server cards")
	}
	if len(cards) == 0 {
		return nil, nil, oops.Hint("add an [[mcp_servers]] entry with a url").
			Errorf("the %s emitter needs a remote MCP server: none of the %d servers has a url", MCPServerCardEmitter, len(in.Servers))
	}
	files := make([]File, 0, len(cards))
	for _, c := range cards {
		data, err := c.JSON()
		if err != nil {
			return nil, nil, err
		}
		_, name, _ := strings.Cut(c.Name, "/")
		files = append(files, File{Path: name + ".json", Data: data})
	}
	out, err := finish(files)
	return out, findingsOf(notes), err
}

// cardNamespace is the namespace option, else the reversed [ard] publisher,
// else the namespace of the repository; empty when none is known.
func cardNamespace(in Input) string {
	switch {
	case in.Options[optNamespace] != "":
		return in.Options[optNamespace]
	case in.ARD != nil && in.ARD.Publisher != "":
		return mcpcard.NamespaceFromDomain(in.ARD.Publisher)
	}
	return mcpcard.NamespaceFromRepo(in.Repo)
}

func findingsOf(notes []string) []Finding {
	var out []Finding
	for _, n := range notes {
		out = append(out, Finding{Message: n})
	}
	return out
}
