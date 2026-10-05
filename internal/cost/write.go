package cost

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// Output formats of the cost report.
const (
	FormatText     = "text"
	FormatJSON     = "json"
	FormatMarkdown = "markdown"
)

// Write prints the report in the named format.
func Write(w io.Writer, r *Report, format string) error {
	switch format {
	case "", FormatText:
		return writeText(w, r)
	case FormatJSON:
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(r) //nolint:wrapcheck // writer error
	case FormatMarkdown:
		return writeMarkdown(w, r)
	}
	return fmt.Errorf("unknown format %q (use text, json or markdown)", format)
}

func comma(n int) string {
	s := fmt.Sprintf("%d", n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

func label(it Item) string {
	name := it.Kind + " " + it.Name
	if it.Domain != "" {
		name += " (" + it.Domain + ")"
	}
	return name
}

// share renders n as a percentage of total.
func share(n, total int) string {
	if total <= 0 {
		return "-"
	}
	return fmt.Sprintf("%d%%", (n*100+total/2)/total)
}

func limitLine(name string, l *Limit) string {
	status := "within budget"
	if l.Exceeded {
		status = "OVER BUDGET"
	}
	return fmt.Sprintf("%s: %s of %s tokens - %s", name, comma(l.Actual), comma(l.Limit), status)
}

func offenderNames(items []Item, n int) string {
	var names []string
	for i, it := range items {
		if i == n {
			break
		}
		names = append(names, label(it))
	}
	return strings.Join(names, ", ")
}

func writeText(w io.Writer, r *Report) error {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Context cost - profile %q, target %s (%s, approximate)\n\n", r.Profile, r.Target, r.Tokenizer)
	fmt.Fprintf(&sb, "  always loaded  %10s tokens\n  conditional    %10s tokens\n  on demand      %10s tokens\n\n",
		comma(r.Always), comma(r.Conditional), comma(r.OnDemand))
	writeTopText(&sb, "Top always-loaded items (estimate from sources)", r.TopAlways, r.Always, func(i Item) int { return i.Always })
	writeTopText(&sb, "Top on-demand items (estimate from sources)", r.TopOnDemand, r.OnDemand, func(i Item) int { return i.OnDemand })
	for _, l := range []struct {
		name string
		l    *Limit
		top  []Item
	}{{"Always-loaded budget", r.AlwaysBudget, r.TopAlways}, {"On-demand budget", r.OnDemandBudget, r.TopOnDemand}} {
		if l.l == nil {
			continue
		}
		sb.WriteString(limitLine(l.name, l.l) + "\n")
		if l.l.Exceeded {
			fmt.Fprintf(&sb, "  biggest offenders: %s\n", offenderNames(l.top, 3))
		}
	}
	sb.WriteString("\nNotes:\n")
	for _, n := range r.Notes {
		fmt.Fprintf(&sb, "  - %s\n", n)
	}
	_, err := io.WriteString(w, sb.String())
	return err //nolint:wrapcheck // writer error
}

func writeTopText(sb *strings.Builder, title string, items []Item, total int, by func(Item) int) {
	if len(items) == 0 {
		return
	}
	fmt.Fprintf(sb, "%s:\n", title)
	for _, it := range items {
		fmt.Fprintf(sb, "  %8s  %4s  %s\n", comma(by(it)), share(by(it), total), label(it))
	}
	sb.WriteString("\n")
}

func writeMarkdown(w io.Writer, r *Report) error {
	var sb strings.Builder
	fmt.Fprintf(&sb, "## Context cost\n\nProfile `%s`, target `%s` (%s, approximate).\n\n", r.Profile, r.Target, r.Tokenizer)
	sb.WriteString("| Bucket | Tokens |\n| --- | ---: |\n")
	fmt.Fprintf(&sb, "| Always loaded | %s |\n| Conditional | %s |\n| On demand | %s |\n", comma(r.Always), comma(r.Conditional), comma(r.OnDemand))
	writeTopMarkdown(&sb, "Top always-loaded items", r.TopAlways, r.Always, func(i Item) int { return i.Always })
	writeTopMarkdown(&sb, "Top on-demand items", r.TopOnDemand, r.OnDemand, func(i Item) int { return i.OnDemand })
	for _, l := range []struct {
		name string
		l    *Limit
	}{{"Always-loaded budget", r.AlwaysBudget}, {"On-demand budget", r.OnDemandBudget}} {
		if l.l != nil {
			fmt.Fprintf(&sb, "\n**%s**\n", limitLine(l.name, l.l))
		}
	}
	sb.WriteString("\n")
	for _, n := range r.Notes {
		fmt.Fprintf(&sb, "> %s\n", n)
	}
	_, err := io.WriteString(w, sb.String())
	return err //nolint:wrapcheck // writer error
}

func writeTopMarkdown(sb *strings.Builder, title string, items []Item, total int, by func(Item) int) {
	if len(items) == 0 {
		return
	}
	fmt.Fprintf(sb, "\n### %s\n\n| Item | Tokens | Share |\n| --- | ---: | ---: |\n", title)
	for _, it := range items {
		fmt.Fprintf(sb, "| %s | %s | %s |\n", strings.ReplaceAll(label(it), "|", `\|`), comma(by(it)), share(by(it), total))
	}
}
