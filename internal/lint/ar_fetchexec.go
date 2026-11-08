package lint

import (
	"path"
	"regexp"
	"strings"
	"sync"
)

// A download that a later command in the same block runs is a pipe to a shell written on two lines. It
// is matched by file name, not by wording, so it does not depend on how a skill phrases anything.
var (
	curlOutputRe = sync.OnceValue(func() *regexp.Regexp {
		return regexp.MustCompile(`(?:^|\s)(?:-[a-zA-Z]*o|--output)[=\s]\s*["']?([^\s"';|&]+)`)
	})
	wgetOutputRe = sync.OnceValue(func() *regexp.Regexp {
		return regexp.MustCompile(`(?:^|\s)(?:-[a-zA-Z]*O|--output-document)[=\s]\s*["']?([^\s"';|&]+)`)
	})
	redirectRe = sync.OnceValue(func() *regexp.Regexp { return regexp.MustCompile(`>\s*["']?([^\s"';|&>]+)`) })
	// A checksum or signature check before the run is what AR005 asks for.
	verifiedRe = sync.OnceValue(func() *regexp.Regexp {
		return regexp.MustCompile(`\b(?:sha(?:1|224|256|384|512)sum|shasum|b2sum|gpg\s+--verify|cosign\s+verify|minisign\s+-V)\b`)
	})
	chainRe      = sync.OnceValue(func() *regexp.Regexp { return regexp.MustCompile(`&&|;`) })
	remoteNameRe = sync.OnceValue(func() *regexp.Regexp {
		return regexp.MustCompile(`(?:^|\s)(?:-[a-zA-Z]*O|--remote-name)(?:\s|$)`)
	})
	psOutFileRe = sync.OnceValue(func() *regexp.Regexp {
		return regexp.MustCompile(`(?i)\b(?:iwr|curl|wget|invoke-webrequest)\b.*-outfile\s+["']?([^\s"';|&]+)`)
	})
	lolbinOutputRe = sync.OnceValue(func() *regexp.Regexp {
		return regexp.MustCompile(`(?i)\b(?:certutil(?:\.exe)?\b.*-urlcache|bitsadmin(?:\.exe)?\b.*/transfer)\b.*https?://\S+\s+["']?([^\s"';|&]+)`)
	})
	windowsRunRe = sync.OnceValue(func() *regexp.Regexp {
		return regexp.MustCompile(`(?i)(?:^|\s)(?:start-process|start|&|cmd(?:\.exe)?\s+/c|msiexec\s+/i|powershell(?:\.exe)?\s)|(?:^|\s)\.\\\S`)
	})
	downloaderRe = sync.OnceValue(func() *regexp.Regexp { return regexp.MustCompile(`(?:^|[\s;&|(])(curl|wget)\s`) })
	remoteURLRe  = sync.OnceValue(func() *regexp.Regexp { return regexp.MustCompile(`https?://[^\s"'<>)]+`) })
	runnerRe     = sync.OnceValue(func() *regexp.Regexp {
		return regexp.MustCompile(`(?:^|[\s;&|(])(?:sudo\s+)?(?:(?:bash|sh|zsh|source|\.|python3?|node|perl|ruby)\s+(?:-[a-zA-Z]+\s+)*)?["']?((?:\./|/|~/|\$[A-Za-z_{])[^\s"';|&]*|[^\s"';|&]+)`)
	})
)

// windowsDownload returns the file a Windows download tool writes, or "".
func windowsDownload(text string) string {
	if !remoteURLRe().MatchString(text) {
		return ""
	}
	for _, re := range []*regexp.Regexp{psOutFileRe(), lolbinOutputRe()} {
		if m := re.FindStringSubmatch(text); m != nil {
			return strings.ReplaceAll(m[1], `\`, "/")
		}
	}
	return ""
}

// runsOnWindows reports whether the segment starts file by one of PowerShell's or cmd's own ways.
func runsOnWindows(segment, file string) bool {
	return strings.Contains(strings.ReplaceAll(segment, `\`, "/"), path.Base(file)) && windowsRunRe().MatchString(segment)
}

// downloadedFile returns the file a download command writes, or "".
func downloadedFile(text string) string {
	m := downloaderRe().FindStringSubmatchIndex(text)
	if m == nil {
		return ""
	}
	tool, cmd := text[m[2]:m[3]], text[m[1]:]
	url := remoteURLRe().FindString(cmd)
	if url == "" {
		return ""
	}
	without := strings.Replace(cmd, url, " ", 1)
	flag := curlOutputRe()
	if tool == "wget" {
		flag = wgetOutputRe()
	}
	if o := flag.FindStringSubmatch(without); o != nil {
		if strings.HasPrefix(o[1], "-") || o[1] == "/dev/null" {
			return ""
		}
		return o[1]
	}
	if o := redirectRe().FindStringSubmatch(without); o != nil {
		return o[1]
	}
	if tool == "wget" || remoteNameRe().MatchString(without) {
		return path.Base(strings.SplitN(url, "?", 2)[0])
	}
	return ""
}

// executes reports whether the line runs file: through an interpreter, or by its path.
func executes(text, file string) bool {
	base := path.Base(file)
	for _, m := range runnerRe().FindAllStringSubmatch(text, -1) {
		arg := strings.TrimRight(m[1], `"'`)
		if arg != file && arg != "./"+base {
			continue
		}
		bare := strings.Fields(strings.TrimSpace(m[0]))
		if strings.HasPrefix(arg, "./") || strings.HasPrefix(arg, "/") || strings.HasPrefix(arg, "~/") ||
			strings.HasPrefix(arg, "$") || len(bare) > 1 {
			return true
		}
	}
	return false
}

// reportRuns reports AR005 for each tracked download that segment runs, and stops tracking it.
func reportRuns(r *runner, t *scanText, line int, segment string, files, windows map[string]bool) {
	for file := range files {
		switch {
		case windows[file] && runsOnWindows(segment, file):
			r.add(CodeShellExec, t.abs, line, "this runs %s, fetched in the same block with a Windows download tool", file)
			delete(files, file)
		case executes(segment, file):
			r.add(CodeShellExec, t.abs, line, "this runs %s, which the same block downloaded: a pipe to a shell written on two lines", file)
			delete(files, file)
		}
	}
}

// scanFetchThenRun reports AR005 where one shell block downloads a file and a later line runs it.
func scanFetchThenRun(r *runner, t *scanText) {
	seen := map[int]map[string]bool{}
	windows := map[string]bool{}
	for _, l := range t.lines {
		if !l.Fenced || l.Neg || !t.shellLike(l) {
			continue
		}
		if seen[l.Block] == nil {
			seen[l.Block] = map[string]bool{}
		}
		files := seen[l.Block]
		for _, segment := range chainRe().Split(l.Text, -1) {
			if verifiedRe().MatchString(segment) {
				clear(files)
				continue
			}
			reportRuns(r, t, l.No, segment, files, windows)
			if file := downloadedFile(segment); file != "" && !r.allHostsAllowed(segment) {
				files[file] = true
			}
			if file := windowsDownload(segment); file != "" && !r.allHostsAllowed(segment) {
				files[file] = true
				windows[file] = true
			}
		}
	}
}
