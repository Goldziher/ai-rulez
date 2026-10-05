package docmerge_test

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/generator/docmerge"
	toml "github.com/pelletier/go-toml/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tailscale/hujson"
	"gopkg.in/yaml.v3"
)

const propertyCases = 200

var (
	words    = []string{"alpha", "beta", "gamma", "delta", "epsilon", "zeta", "eta", "theta", "iota", "kappa"}
	comments = []string{"keep me", "TODO: later", "see docs", "why: because", "# nested hash"}
)

// docGen builds random, valid user documents in one format.
type docGen struct {
	r  *rand.Rand
	nl string
}

func (g *docGen) pick(items []string) string { return items[g.r.Intn(len(items))] }
func (g *docGen) chance(percent int) bool    { return g.r.Intn(100) < percent }

func (g *docGen) scalar() string {
	switch g.r.Intn(4) {
	case 0:
		return fmt.Sprint(g.r.Intn(1000))
	case 1:
		return "true"
	case 2:
		return fmt.Sprintf("%q", g.pick(words)+" "+g.pick(words))
	default:
		return fmt.Sprintf("%q", g.pick(words))
	}
}

// uniqueKeys returns n distinct key names, never the owned top-level key.
func (g *docGen) uniqueKeys(n int) []string {
	perm := g.r.Perm(len(words))
	keys := make([]string, 0, n)
	for _, i := range perm[:n] {
		keys = append(keys, words[i])
	}
	return keys
}

// finish joins the lines; mustEnd forces a final newline.
func (g *docGen) finish(lines []string, mustEnd bool) string {
	doc := strings.Join(lines, g.nl)
	if mustEnd || g.chance(85) {
		doc += g.nl
	}
	if g.chance(10) {
		doc = "\xef\xbb\xbf" + doc
	}
	return doc
}

func (g *docGen) toml() string {
	var lines []string
	if g.chance(40) {
		lines = append(lines, "# "+g.pick(comments))
	}
	for _, key := range g.uniqueKeys(1 + g.r.Intn(3)) {
		line := key + " = " + g.scalar()
		if g.chance(30) {
			line += " # " + g.pick(comments)
		}
		lines = append(lines, line)
	}
	if g.chance(30) {
		lines = append(lines, "list = ["+g.scalar()+", "+g.scalar()+"]")
	}
	if g.chance(15) {
		lines = append(lines, "notes = \"\"\"", "multi # line", "\"\"\"")
	}
	for i, table := range []string{"profiles", "mcp_servers.mine", "tools"}[:g.r.Intn(4)] {
		if g.chance(60) || i == 0 {
			lines = append(lines, "")
		}
		lines = append(lines, "["+table+"]", "command = "+g.scalar())
		if g.chance(30) {
			lines = append(lines, "# "+g.pick(comments))
		}
	}
	if g.chance(15) {
		lines = append(lines, "", "[mcp_servers]")
	}
	return g.finish(lines, false)
}

func (g *docGen) yaml() string {
	var lines []string
	if g.chance(30) {
		lines = append(lines, "---")
	}
	if g.chance(40) {
		lines = append(lines, "# "+g.pick(comments))
	}
	indent := g.pick([]string{"  ", "    "})
	for _, key := range g.uniqueKeys(1 + g.r.Intn(3)) {
		switch g.r.Intn(5) {
		case 0:
			lines = append(lines, key+":", indent+"inner: "+g.scalar(), indent+"more: "+g.scalar())
		case 1:
			lines = append(lines, key+":", "- "+g.scalar(), "- "+g.scalar())
		case 2:
			lines = append(lines, key+": |", indent+"text "+g.pick(words), indent+"# not a comment")
		default:
			line := key + ": " + g.scalar()
			if g.chance(30) {
				line += " # " + g.pick(comments)
			}
			lines = append(lines, line)
		}
		if g.chance(25) {
			lines = append(lines, "")
		}
		if g.chance(20) {
			lines = append(lines, "# "+g.pick(comments))
		}
	}
	if g.chance(30) {
		lines = append(lines, "mcp_servers:", indent+"mine:", indent+indent+"command: "+g.scalar())
	}
	// A block scalar that runs to the end of a file with no final newline cannot
	// take another member after it without gaining a newline of its own, which
	// would change its value; the engine refuses that, so the generator avoids it.
	return g.finish(lines, strings.Contains(strings.Join(lines, "\n"), ": |"))
}

func (g *docGen) json() string {
	indent := g.pick([]string{"  ", "\t", "    "})
	withComments := g.chance(50)
	keys := g.uniqueKeys(1 + g.r.Intn(3))
	lines := []string{"{"}
	if withComments {
		lines = append(lines, indent+"// "+g.pick(comments))
	}
	trailing := withComments && g.chance(40)
	for i, key := range keys {
		line := fmt.Sprintf("%s%q: %s", indent, key, g.scalar())
		if g.chance(25) {
			line = fmt.Sprintf("%s%q: [%s, %s]", indent, key, g.scalar(), g.scalar())
		}
		if i < len(keys)-1 || trailing {
			line += ","
		}
		if withComments && g.chance(30) {
			line += " // " + g.pick(comments)
		}
		lines = append(lines, line)
	}
	lines = append(lines, "}")
	return g.finish(lines, false)
}

func (g *docGen) document(format docmerge.Format) string {
	switch format {
	case docmerge.FormatTOML:
		return g.toml()
	case docmerge.FormatYAML:
		return g.yaml()
	default:
		return g.json()
	}
}

// ownedFor builds random owned servers under mcp_servers (mcpServers in JSON).
func ownedFor(r *rand.Rand, format docmerge.Format) []docmerge.OwnedKey {
	entries := map[string]any{}
	for i := 0; i <= r.Intn(3); i++ {
		entry := map[string]any{"command": "cmd" + fmt.Sprint(i), "args": []string{"-y", "pkg"}}
		if r.Intn(2) == 0 {
			entry["env"] = map[string]any{"K": "v \"quoted\" # hash"}
		}
		entries[fmt.Sprintf("gen%d", i)] = entry
	}
	name := "mcp_servers"
	if format == docmerge.FormatJSON || format == docmerge.FormatJSONC {
		name = "mcpServers"
	}
	return []docmerge.OwnedKey{{Name: name, Value: entries, Members: true}}
}

func decodeDoc(t *testing.T, format docmerge.Format, doc string) map[string]any {
	t.Helper()
	doc = strings.TrimPrefix(doc, "\xef\xbb\xbf")
	tree := map[string]any{}
	switch format {
	case docmerge.FormatTOML:
		require.NoError(t, toml.Unmarshal([]byte(doc), &tree), doc)
	case docmerge.FormatYAML:
		require.NoError(t, yaml.Unmarshal([]byte(doc), &tree), doc)
	default:
		std, err := hujson.Standardize([]byte(doc))
		require.NoError(t, err, doc)
		require.NoError(t, json.Unmarshal(std, &tree), doc)
	}
	return tree
}

func TestProperty_ApplyUnmergeRoundTrip(t *testing.T) {
	formats := map[docmerge.Format]string{
		docmerge.FormatTOML: "c.toml", docmerge.FormatYAML: "c.yaml", docmerge.FormatJSONC: "c.json",
	}
	for format, file := range formats {
		t.Run(string(format), func(t *testing.T) {
			for i := range propertyCases {
				seed := int64(i + 1)
				r := rand.New(rand.NewSource(seed))
				gen := &docGen{r: r, nl: "\n"}
				if r.Intn(5) == 0 {
					gen.nl = "\r\n"
				}
				doc := gen.document(format)
				owned := ownedFor(r, format)
				label := fmt.Sprintf("seed %d\n%q", seed, doc)

				// Act
				applied, err := docmerge.ApplyDocument(file, format, doc, owned)
				require.NoError(t, err, label)
				again, err := docmerge.ApplyDocument(file, format, applied.Body, owned)
				require.NoError(t, err, label)
				unmerged, err := docmerge.UnmergeDocument(file, format, applied.Body, applied.Claims)
				require.NoError(t, err, label)

				// Assert: idempotent, the user's content survives the merge, and
				// unmerging restores the original bytes.
				assert.Equal(t, applied.Body, again.Body, "apply is idempotent: "+label)
				assert.True(t, unmerged.Changed, label)
				assert.Equal(t, doc, unmerged.Body, label)
				before, after := decodeDoc(t, format, doc), decodeDoc(t, format, applied.Body)
				for _, key := range owned {
					for name := range key.Value.(map[string]any) {
						assert.Contains(t, after[key.Name], name, label)
					}
					removeOwned(after, key)
				}
				removeOwned(before, owned[0])
				assert.Equal(t, before, after, "preservation: "+label)
			}
		})
	}
}

// removeOwned deletes the owned entries from a decoded document, and the parent
// when that leaves it empty, to compare what is left.
func removeOwned(tree map[string]any, key docmerge.OwnedKey) {
	parent, _ := tree[key.Name].(map[string]any)
	for name := range key.Value.(map[string]any) {
		delete(parent, name)
	}
	if len(parent) == 0 {
		delete(tree, key.Name)
	}
}
