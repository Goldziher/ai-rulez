package okfbridge

import (
	"path"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/okf"
)

// rewriteExportLinks finishes the pieces of an export: relative links between
// exported source files become bundle-absolute links to the exported concept
// files (/rules/b.md). A relative link to anything that is not part of the export
// is left as written and reported in res.Notes. Fenced code and inline code are
// never touched. Bundle-absolute and external links are kept.
func rewriteExportLinks(pieces []piece, res *ExportResult) []okf.File {
	bundleOf := map[string]string{}
	for i := range pieces {
		if pieces[i].src != "" {
			bundleOf[pieces[i].src] = pieces[i].file.Path
		}
	}
	files := make([]okf.File, 0, len(pieces))
	for i := range pieces {
		pc := pieces[i]
		if pc.head == nil {
			files = append(files, pc.file)
			continue
		}
		body := pc.body
		if pc.src != "" {
			dir := path.Dir(pc.src)
			body = okf.RewriteLinks(pc.body, func(dest string, _ int) (string, bool) {
				p, suffix, internal := okf.SplitDest(dest)
				if !internal || strings.HasPrefix(p, "/") {
					return "", false
				}
				target, _, inside := okf.ResolveLink(dir, dest)
				if !inside {
					res.Notes = append(res.Notes, outsideNote(pc.file.Path, dest))
					return "", false
				}
				if bundlePath, ok := bundleOf[target]; ok {
					return okf.JoinDest("/"+bundlePath, suffix), true
				}
				res.Notes = append(res.Notes, outsideNote(pc.file.Path, dest))
				return "", false
			})
		}
		pc.file.Data = append(append([]byte{}, pc.head...), body...)
		files = append(files, pc.file)
	}
	return files
}

func outsideNote(concept, dest string) string {
	return "link " + dest + " in " + concept + " points at a file that is not part of the export; left unchanged"
}
