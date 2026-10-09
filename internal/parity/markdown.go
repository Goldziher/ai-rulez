package parity

import (
	"fmt"
	"sort"
	"strings"
)

// Markers delimit the generated block of docs/mcp-server.md.
const (
	DocsBegin = "<!-- parity:begin -->"
	DocsEnd   = "<!-- parity:end -->"
)

// Markdown renders the table as the three tables of docs/mcp-server.md: the
// capabilities both sides have, those only the command line has, and those only
// MCP has. The output is deterministic.
func Markdown(caps []Capability) string {
	var paired, cliOnly, mcpOnly []Capability
	sorted := Sorted(caps)
	for i := range sorted {
		c := sorted[i]
		switch c.Kind {
		case Paired:
			paired = append(paired, c)
		case CLIOnly:
			cliOnly = append(cliOnly, c)
		case MCPOnly:
			mcpOnly = append(mcpOnly, c)
		}
	}
	var b strings.Builder
	b.WriteString("**On both sides.** The command and the tool run the same library code. Rows marked *compared* are also run on one fixture by the parity test, which checks the command's `--format json` document and the tool's `structuredContent` field for field.\n\n")
	b.WriteString("| Command | MCP tool | Server | Compared |\n| ------- | -------- | ------ | -------- |\n")
	for i := range paired {
		c := &paired[i]
		compared := ""
		if c.Behavior != "" {
			compared = "yes"
		}
		fmt.Fprintf(&b, "| %s | `%s` | %s | %s |\n", commandCell(c), c.Tool, c.ServerOrDefault(), compared)
	}
	b.WriteString("\n**Only on the command line**, deliberately. These write trust, spend money, use the network or credentials, prompt, or run for the length of a process.\n\n")
	b.WriteString("| Command | Why it is not a tool |\n| ------- | -------------------- |\n")
	for i := range cliOnly {
		c := &cliOnly[i]
		fmt.Fprintf(&b, "| %s | %s |\n", commandCell(c), escape(c.Reason))
	}
	b.WriteString("\n**Only on MCP**, deliberately.\n\n")
	b.WriteString("| MCP tool | Server | Why it has no command |\n| -------- | ------ | --------------------- |\n")
	for i := range mcpOnly {
		c := &mcpOnly[i]
		fmt.Fprintf(&b, "| `%s` | %s | %s |\n", c.Tool, c.ServerOrDefault(), escape(c.Reason))
	}
	return b.String()
}

func commandCell(c *Capability) string {
	paths := append([]string(nil), c.CLI...)
	sort.Strings(paths)
	cells := make([]string, len(paths))
	for i, p := range paths {
		cells[i] = "`" + p + "`"
		if c.Mode != "" {
			cells[i] = "`" + p + " --" + c.Mode + "`"
		}
	}
	return strings.Join(cells, ", ")
}

func escape(s string) string { return strings.ReplaceAll(s, "|", "\\|") }
