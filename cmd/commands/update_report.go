package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/samber/oops"
)

func finishUpdate(rep *updateReport, code int) int {
	sort.Slice(rep.Updates, func(i, j int) bool {
		if rep.Updates[i].Kind != rep.Updates[j].Kind {
			return rep.Updates[i].Kind < rep.Updates[j].Kind
		}
		return rep.Updates[i].Name < rep.Updates[j].Name
	})
	if updateFormat == formatJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(rep); err != nil {
			fmtError(oops.Wrapf(err, "write the report"))
			return 1
		}
		return code
	}
	writeUpdateText(rep)
	return code
}

// writeUpdateItem prints one update and reports whether its scan refused it.
func writeUpdateItem(u *updateItem) (refused bool) {
	from := "(unlocked)"
	if u.From != nil {
		from = u.From.Tag
	}
	fmt.Printf("%s %s: %s -> %s (%s)\n", u.Kind, u.Name, from, u.To.Tag, shortSHA(u.To.Commit))
	if u.Released != "" {
		fmt.Printf("  released %s (%s)\n", u.Released, u.ReleasedFrom)
	}
	for _, h := range u.Held {
		fmt.Printf("  %s\n", safeText(h.String()))
	}
	for _, f := range u.Files {
		fmt.Printf("  %s  %s\n", f.Change, f.Path)
	}
	if u.DigestOld != u.DigestNew {
		fmt.Printf("  tree %s -> %s\n", u.DigestOld, u.DigestNew)
	}
	if u.Scan != nil {
		writeScanText(*u)
		refused = u.Scan.Refused
	}
	fmt.Println("  run `ai-rulez generate`, then `ai-rulez lock` (it refreshes the output pins and the served-skill pins, which stay stale until then)")
	return refused
}

func writeUpdateText(rep *updateReport) {
	verb := "updated"
	if rep.DryRun {
		verb = "would update"
	}
	refused := 0
	for i := range rep.Updates {
		if writeUpdateItem(&rep.Updates[i]) {
			refused++
		}
	}
	for _, m := range rep.Major {
		switch {
		case m.Written:
			fmt.Printf("%s %s: wrote version = %q to config.toml (was %q; latest %s)\n", m.Kind, m.Name, m.To, m.From, m.Latest)
		default:
			fmt.Printf("%s %s: newer major %s: version = %q (now %q); `update --major --write-config` applies it\n", m.Kind, m.Name, m.Latest, m.To, m.From)
		}
	}
	for i := range rep.Blocked {
		r := &rep.Blocked[i]
		fmt.Fprintf(os.Stderr, "refused %s %s: %s %s\n", r.Kind, r.Name, r.Code, r.Note)
	}
	for i := range rep.Unchanged {
		r := &rep.Unchanged[i]
		if r.Downgrade {
			fmt.Printf("%s %s: %s\n", r.Kind, r.Name, r.Note)
		}
		for _, h := range r.Held {
			fmt.Printf("%s %s: %s\n", r.Kind, r.Name, safeText(h.String()))
		}
	}
	for _, n := range rep.Notes {
		fmt.Println(n)
	}
	switch {
	case len(rep.Blocked) > 0:
		fmt.Fprintln(os.Stderr, "nothing was written")
	case refused > 0:
		fmt.Fprintf(os.Stderr, "refused %d source(s): the security scan of the new tree has error findings; review them, then pass --accept-findings. Nothing was written\n", refused)
	case len(rep.Updates) > 0:
		fmt.Printf("%s %d source(s)\n", verb, len(rep.Updates))
	case len(rep.Major) == 0:
		fmt.Println("everything is up to date within its constraints")
	}
}

// writeScanText prints the scan result of one update; the text of a finding is
// untrusted content, so it is printed escaped.
func writeScanText(u updateItem) {
	sc := u.Scan
	switch {
	case sc.Errors == 0 && sc.Warnings == 0:
		fmt.Println("  scan: 0 findings")
	default:
		fmt.Printf("  scan: %d error(s), %d warning(s)\n", sc.Errors, sc.Warnings)
	}
	for _, f := range sc.Findings {
		fmt.Printf("    %s %s %s:%d %s\n", f.Code, f.Severity, safeText(f.File), f.Line, safeText(f.Message))
	}
	if sc.Note != "" {
		fmt.Printf("  scan note: %s\n", safeText(sc.Note))
	}
	if sc.Accepted {
		fmt.Println("  scan findings accepted with --accept-findings")
	}
}
