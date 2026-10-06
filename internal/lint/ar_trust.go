package lint

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

// Codes for the provenance and coverage rules.
const (
	CodePublisherMismatch = "AR032"
	CodeAuthorityClaim    = "AR033"
	CodeLowAnalyzability  = "AR034"
)

func init() {
	registerRules(
		RuleInfo{CodePublisherMismatch, "publisher-mismatch", SeverityWarning, "an installed skill's name or description credits a publisher that is not the owner of the repository it was installed from"},
		RuleInfo{CodeAuthorityClaim, "authority-claim", SeverityInfo, "an installed skill's description claims to be official, verified or trusted, but its source owner is not a known organization"},
		RuleInfo{CodeLowAnalyzability, "low-analyzability", SeverityInfo, "most of a skill directory is binary, archived or oversize, so the scan did not read it"},
	)
	registerRunCheck(checkInstalledTrust, AnalyzerSecurity)
	registerRunCheck(checkAnalyzability, AnalyzerSecurity)
}

var (
	claimRe     = regexp.MustCompile(`\b(?:(?:made|created|published|maintained|authored|developed|built|provided|owned|supported)\s+by|by)\s+(@[A-Za-z0-9_-]+|[A-Z][a-z][\w&.-]*(?:\s+[A-Z][\w&.-]*){0,3})`)
	authorityRe = regexp.MustCompile(`(?i)\b(official|verified|trusted|authorized|endorsed|certified)\b`)
	orgSuffixRe = regexp.MustCompile(`(?i)\b(?:corp|corporation|inc|team|labs|co|ltd|llc|technologies|software|group)\b\.?`)
	// ownerRe pulls the owner out of a git source: github.com/owner/repo,
	// https://host/owner/repo.git, git@host:owner/repo or owner/repo.
	ownerRe = regexp.MustCompile(`^(?:(?:https?|ssh|git)://(?:[^@/]+@)?[^/]+/|git@[^:]+:|[a-z0-9.-]+\.[a-z]{2,}/)?([A-Za-z0-9_.-]+)/[A-Za-z0-9_.-]+`)

	// trustedOrgs are organizations an "official" claim is believable for.
	trustedOrgs = []string{
		"anthropics", "anthropic", "openai", "google", "googlecloudplatform", "google-gemini", "microsoft", "github", "vercel", "cloudflare",
		"aws", "awslabs", "aws-samples", "stripe", "supabase", "hashicorp", cmdDocker, "kubernetes", "mozilla", "jetbrains", "atlassian",
		"slackapi", "figma", "getsentry", "datadog", "elastic", "mongodb", "redis", "huggingface", "langchain-ai", "modelcontextprotocol",
		"agentskills", presetCursor, "openai-codex", "vercel-labs", "netlify", "shopify", "twilio", "notionhq", "linear", "pulumi",
	}
)

func normalizeOrg(s string) string {
	s = strings.ToLower(orgSuffixRe.ReplaceAllString(s, ""))
	var b strings.Builder
	for _, c := range s {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') {
			b.WriteRune(c)
		}
	}
	return b.String()
}

// sourceOwner is the owner of a git source, or "" for a local path.
func sourceOwner(source string) string {
	source = strings.TrimSpace(source)
	if strings.HasPrefix(source, ".") || strings.HasPrefix(source, "/") || strings.HasPrefix(source, "~") || source == "" {
		return ""
	}
	if m := ownerRe.FindStringSubmatch(source); m != nil {
		return m[1]
	}
	return ""
}

// claimedPublisher returns the publisher the text credits itself to.
func claimedPublisher(text string) string {
	if m := claimRe.FindStringSubmatch(text); m != nil {
		return strings.TrimPrefix(strings.TrimSpace(m[1]), "@")
	}
	return ""
}

// publisherMatches is a loose comparison: equal, or one contains the other.
func publisherMatches(claimed, owner string) bool {
	a, b := normalizeOrg(claimed), normalizeOrg(owner)
	if a == "" || b == "" {
		return true
	}
	return strings.Contains(a, b) || strings.Contains(b, a)
}

func checkInstalledTrust(r *runner) { //nolint:gocyclo // linear checks over a documented schema; splitting them hides the rules
	cfgPath := r.configFilePath()
	if cfgPath == "" || len(r.cfg.InstalledSkills) == 0 {
		return
	}
	lines := r.fileLines(cfgPath)
	for _, s := range r.cfg.InstalledSkills {
		owner := sourceOwner(s.Source)
		if owner == "" {
			continue // a local source: nothing to compare with
		}
		var it *item
		for i := range r.items {
			if c := &r.items[i]; !c.owned && !c.isDoc && c.kind == kindSkill && (itemID(c.kind, c.cf) == s.Name || strings.EqualFold(itemID(c.kind, c.cf), s.Name)) {
				it = c
				break
			}
		}
		if it == nil {
			continue
		}
		text := itemID(it.kind, it.cf) + ". " + r.description(it)
		at := lineContaining(lines, `"`+s.Name+`"`)
		if claimed := claimedPublisher(text); claimed != "" && !publisherMatches(claimed, owner) {
			r.add(CodePublisherMismatch, cfgPath, at, "installed skill %q credits %q, but it is installed from %s/...; check that the skill is what it says it is", s.Name, claimed, owner)
		}
		if authorityRe.MatchString(text) && !inSet(trustedOrgs, strings.ToLower(owner)) {
			r.add(CodeAuthorityClaim, cfgPath, at, "installed skill %q describes itself as %q, but its source owner %q is not a known organization", s.Name, strings.ToLower(authorityRe.FindString(text)), owner)
		}
	}
}

// Thresholds for AR034.
const (
	minAuditableRatio = 0.70
	maxAuditableBytes = 1 << 20
	maxWalkedFiles    = 5000
)

var opaqueExt = map[string]bool{
	".wasm": true, ".zip": true, ".tar": true, ".gz": true, ".tgz": true, ".bz2": true, ".xz": true, ".7z": true, ".rar": true, ".jar": true,
	".exe": true, ".dll": true, ".so": true, ".dylib": true, ".bin": true, ".o": true, ".a": true, ".class": true, ".pyc": true, ".whl": true,
	".pdf": true, ".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true, ".ico": true, ".mp4": true, ".mov": true, ".mp3": true,
	".wav": true, ".ttf": true, ".otf": true, ".woff": true, ".woff2": true, ".sqlite": true, ".db": true, ".onnx": true, ".pt": true,
}

var skipDirs = map[string]bool{".git": true, "node_modules": true, "__pycache__": true, ".venv": true}

func checkAnalyzability(r *runner) { //nolint:gocyclo // linear checks over a documented schema; splitting them hides the rules
	for i := range r.items {
		it := &r.items[i]
		if !it.owned || it.isDoc || it.kind != kindSkill || it.itemDir == "" {
			continue
		}
		total, auditable := 0, 0
		type opaque struct {
			rel  string
			size int
		}
		var opaques []opaque
		walked := 0
		//nolint:errcheck // an unreadable entry is simply not counted
		_ = filepath.WalkDir(it.itemDir, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil //nolint:nilerr // best effort
			}
			if d.IsDir() {
				if p != it.itemDir && skipDirs[d.Name()] {
					return filepath.SkipDir
				}
				return nil
			}
			walked++
			if walked > maxWalkedFiles {
				return filepath.SkipAll
			}
			info, ierr := d.Info()
			if ierr != nil || !info.Mode().IsRegular() {
				return nil //nolint:nilerr // an unreadable entry is simply not counted
			}
			size := int(info.Size())
			total += size
			if size <= maxAuditableBytes && !opaqueExt[strings.ToLower(filepath.Ext(p))] && looksLikeText(p) {
				auditable += size
				return nil
			}
			rel, _ := filepath.Rel(it.itemDir, p) //nolint:errcheck // display only
			opaques = append(opaques, opaque{filepath.ToSlash(rel), size})
			return nil
		})
		if total == 0 || float64(auditable)/float64(total) >= minAuditableRatio {
			continue
		}
		sort.Slice(opaques, func(a, b int) bool {
			if opaques[a].size != opaques[b].size {
				return opaques[a].size > opaques[b].size
			}
			return opaques[a].rel < opaques[b].rel
		})
		var names []string
		for j, o := range opaques {
			if j == 3 {
				names = append(names, "...")
				break
			}
			names = append(names, o.rel)
		}
		r.add(CodeLowAnalyzability, it.abs, r.docs[it.abs].lineOf(keyName, 1),
			"only %d%% of the %d bytes in this skill directory could be scanned; the scan did not read %s", auditable*100/total, total, strings.Join(names, ", "))
	}
}

// looksLikeText reports whether a file starts like UTF-8 text (no NUL byte).
func looksLikeText(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close() //nolint:errcheck // read-only
	buf := make([]byte, 8192)
	n, _ := f.Read(buf) //nolint:errcheck // a short or empty read is fine
	buf = buf[:n]
	if strings.IndexByte(string(buf), 0) >= 0 {
		return false
	}
	// a read can cut a multi-byte rune at the end
	for len(buf) > 0 && !utf8.Valid(buf) && len(buf) > n-4 {
		buf = buf[:len(buf)-1]
	}
	return utf8.Valid(buf)
}
