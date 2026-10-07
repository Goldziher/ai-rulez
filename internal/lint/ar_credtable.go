package lint

import (
	"fmt"
	"regexp"
	"strings"
)

// CodeUnknownDotdir reads a hidden directory of the home folder that no credential family names.
const CodeUnknownDotdir = "AR027"

func registerArCredtable(s *ruleSet) {
	s.addRules(RuleInfo{CodeUnknownDotdir, "unknown-dotdir-read", SeverityOff, "a read command targets a hidden directory of the home folder that is not in the credential table or a known benign list (off by default; a catch-all for locations the table does not know)"})
}

// Credential tiers. critical and high report as AR006's warning, the rest as info;
// copying or uploading a credential raises any tier to an error.
const (
	tierCritical = "critical"
	tierHigh     = "high"
	tierMedium   = "medium"
	tierLow      = "low"
	tierInfo     = "info"
)

// Access methods.
const (
	methodRead     = "read"
	methodCopy     = "copy"
	methodRedirect = "redirect"
	methodDD       = "dd"
	methodExfil    = "exfil"
)

// credFamily is one family of credential locations.
type credFamily struct {
	id    string
	tier  string
	label string
	re    *regexp.Regexp
	// methods limits the access methods that matter (nil means all).
	methods []string
	// skip reports a match that is not a credential (a public key, a sample file).
	skip func(m []string) bool
}

const home = `(?:~|\$HOME|\$\{HOME\}|/home/[^/\s"']+|/Users/[^/\s"']+)`

func hp(rest string) *regexp.Regexp { return regexp.MustCompile(home + `/` + rest) }
func ap(rest string) *regexp.Regexp { return regexp.MustCompile(`(?:^|[\s"'=<@(])` + rest) }

func notPub(m []string) bool { return len(m) > 1 && m[len(m)-1] == ".pub" }

// credentialTable re-derives the location families from the public conventions of
// each tool (where ssh, aws, gpg, kubectl, docker, ... keep secrets), not from any
// particular scanner's list.
var credentialTable = []credFamily{
	{id: "ssh-private-key", tier: tierCritical, label: "an SSH key or config", re: hp(`\.ssh/(?:id_[A-Za-z0-9_]+|known_hosts|authorized_keys|config)(\.pub)?`), skip: notPub},
	{id: "ssh-private-key", tier: tierCritical, label: "an SSH private key", re: regexp.MustCompile(`(?:^|[\s"'/])id_(?:rsa|ed25519|ecdsa|dsa)(\.pub)?\b`), skip: notPub},
	{id: "env-file", tier: tierCritical, label: "a .env file", re: regexp.MustCompile(`(?:^|[\s"'=/@<])\.env(?:rc)?(\.[A-Za-z]+)?(?:[\s"';|)&>]|$)`), methods: []string{methodRead},
		skip: func(m []string) bool {
			switch strings.ToLower(strings.TrimPrefix(m[len(m)-1], ".")) {
			case "example", "sample", "template", "dist", "defaults", "schema", "test":
				return true
			}
			return false
		}},
	{id: "aws-credentials", tier: tierCritical, label: "AWS credentials", re: hp(`\.aws(?:/(?:credentials|config|sso/\S*)|/?(?:[\s"';|)&>]|$))`)},
	{id: "etc-shadow", tier: tierCritical, label: "the system password hashes", re: ap(`/etc/(?:shadow|gshadow|master\.passwd)\b`)},
	{id: "git-credentials", tier: tierCritical, label: "stored git credentials", re: hp(`\.git-credentials\b`)},
	{id: "netrc", tier: tierCritical, label: ".netrc", re: hp(`[._]netrc\b`)},
	{id: "gnupg", tier: tierCritical, label: "the GnuPG keyring", re: hp(`\.gnupg(?:/|\b)`)},
	{id: "kube-config", tier: tierCritical, label: "the kubeconfig", re: hp(`\.kube/(?:config|cache)\b`)},
	{id: "vault-token", tier: tierCritical, label: "the Vault token", re: hp(`\.vault-token\b`)},
	{id: "terraform-creds", tier: tierCritical, label: "Terraform credentials", re: hp(`\.terraform\.d/credentials\S*`)},
	{id: "keyring", tier: tierCritical, label: "the GNOME keyring", re: hp(`\.local/share/keyrings\b`)},
	{id: "npmrc", tier: tierCritical, label: ".npmrc", re: hp(`\.npmrc\b`)},
	{id: "pypirc", tier: tierCritical, label: ".pypirc", re: hp(`\.pypirc\b`)},
	{id: "gem-credentials", tier: tierCritical, label: "RubyGems credentials", re: hp(`\.gem/credentials\b`)},
	{id: "ssl-private", tier: tierCritical, label: "TLS private keys", re: ap(`/etc/ssl/private\b`)},
	{id: "ssh-host-key", tier: tierCritical, label: "an SSH host key", re: ap(`/etc/ssh/ssh_host_\w+_key\b`)},
	{id: "pgpass", tier: tierCritical, label: ".pgpass", re: hp(`\.pgpass\b`)},
	{id: "mysql-cnf", tier: tierCritical, label: ".my.cnf", re: hp(`\.my\.cnf\b`)},
	{id: "azure", tier: tierHigh, label: "Azure credentials", re: hp(`\.azure/`)},
	{id: "gcloud", tier: tierHigh, label: "gcloud credentials", re: hp(`\.config/gcloud\b`)},
	{id: "docker-config", tier: tierHigh, label: "Docker registry credentials", re: hp(`\.docker/config\.json\b`)},
	{id: "gh-cli", tier: tierHigh, label: "the GitHub CLI token", re: hp(`\.config/gh/hosts\.ya?ml\b`)},
	{id: "password-store", tier: tierHigh, label: "the pass password store", re: hp(`\.password-store\b`)},
	{id: "macos-keychain", tier: tierHigh, label: "a macOS keychain", re: regexp.MustCompile(`(?:~|\$HOME|\$\{HOME\})?/Library/Keychains\b`)},
	{id: "terraformrc", tier: tierHigh, label: ".terraformrc", re: hp(`\.terraformrc\b`)},
	{id: "cargo-credentials", tier: tierHigh, label: "Cargo registry credentials", re: hp(`\.cargo/credentials(?:\.toml)?\b`)},
	{id: "op-cli", tier: tierHigh, label: "1Password CLI data", re: hp(`(?:\.config/op|\.op)(?:/|\b)`)},
	{id: "age-keys", tier: tierHigh, label: "age keys", re: hp(`(?:\.config/age/keys\.txt|\.age/\S*)`)},
	{id: "etc-passwd", tier: tierMedium, label: "the user database", re: ap(`/etc/(?:passwd|sudoers(?:\.d/\S*)?)\b`)},
	{id: "shell-history", tier: tierLow, label: "shell history", re: hp(`\.(?:bash|zsh|python|node|psql|mysql)_history\b|` + `\.local/share/fish/fish_history\b`)},
	{id: "openvpn", tier: tierLow, label: "OpenVPN profiles", re: regexp.MustCompile(`(?:^|[\s"'=])(?:/etc/openvpn|~/\.openvpn)\b`)},
	{id: "auth-log", tier: tierInfo, label: "the authentication log", re: ap(`/var/log/(?:auth\.log|secure)\b`)},
}

// credentialStems are literals that every credentialTable pattern contains;
// a line without one cannot match any family, so the table is skipped for it.
var credentialStems = []string{
	".ssh", "id_", ".env", ".aws", "/etc/", ".git-credentials", "netrc", ".gnupg", ".kube", ".vault-token", ".terraform", "keyrings",
	".npmrc", ".pypirc", ".gem/", ".pgpass", ".my.cnf", ".azure", ".config/", ".docker", ".password-store", "Keychains", ".cargo",
	".op", ".age", "_history", "openvpn", "/var/log",
}

var (
	readVerbRe     = regexp.MustCompile(`(?i)(?:^|[\s(])(?:cat|head|tail|less|more|bat|tac|strings|base64|xxd|od|hexdump|grep|egrep|awk|sed|get-content|type)\s`)
	copyVerbRe     = regexp.MustCompile(`(?:^|[\s(])(?:cp|ln|install|mv)\s`)
	exfilVerbRe    = regexp.MustCompile(`(?:^|[\s(])(?:scp|rsync)\s`)
	ddRe           = regexp.MustCompile(`\bdd\b[^|]*\bif=\s*$`)
	redirectInRe   = regexp.MustCompile(`<\s*$`)
	keychainCmdRe  = regexp.MustCompile(`\bsecurity\s+find-(?:generic|internet)-password\b`)
	credSegSplitRe = regexp.MustCompile(`&&|\|\||[;|]|\$\(|\x60`)
)

// accessMethod reports how the text before a credential path reaches it.
func accessMethod(prefix string) string {
	if loc := credSegSplitRe.FindAllStringIndex(prefix, -1); len(loc) > 0 {
		prefix = prefix[loc[len(loc)-1][1]:]
	}
	switch {
	case ddRe.MatchString(prefix):
		return methodDD
	case redirectInRe.MatchString(prefix):
		return methodRedirect
	case exfilVerbRe.MatchString(prefix):
		return methodExfil
	case copyVerbRe.MatchString(prefix):
		return methodCopy
	case readVerbRe.MatchString(prefix):
		return methodRead
	}
	return ""
}

func (f credFamily) wants(method string) bool {
	if f.methods == nil {
		return true
	}
	for _, m := range f.methods {
		if m == method {
			return true
		}
	}
	return false
}

// credHit is one credential access found on a line.
type credHit struct {
	sev          Severity
	label, where string
	method       string
}

// detectCredentialAccess finds the most severe credential access on a line.
func detectCredentialAccess(line string) (credHit, bool) {
	var best credHit
	if !containsAnyStem(line, false, credentialStems) {
		return best, false
	}
	for _, fam := range credentialTable {
		for _, loc := range fam.re.FindAllStringSubmatchIndex(line, -1) {
			m := make([]string, len(loc)/2)
			for i := range m {
				if loc[2*i] >= 0 {
					m[i] = line[loc[2*i]:loc[2*i+1]]
				}
			}
			if fam.skip != nil && fam.skip(m) {
				continue
			}
			method := accessMethod(line[:prefixEnd(line, loc[0])])
			if method == "" || !fam.wants(method) {
				continue
			}
			sev := credentialSeverity(fam.tier, method)
			if best.sev == "" || sev.rank() > best.sev.rank() {
				best = credHit{sev: sev, label: fam.label, where: strings.TrimSpace(m[0]), method: method}
			}
		}
	}
	return best, best.sev != ""
}

// scanCredentialAccess reports commands that read, copy or upload a credential
// location. Merely naming a path (ls ~/.ssh, "the .env file holds secrets") is
// not an access and is not reported.
func (r *runner) scanCredentialAccess(abs string, no int, line string) {
	if keychainCmdRe.MatchString(line) {
		r.add(CodeShellAccess, abs, no, "reads a password from the macOS keychain (security find-*-password)")
		return
	}
	if hit, ok := detectCredentialAccess(line); ok {
		r.addSev(hit.sev, CodeShellAccess, abs, no, "%s", fmt.Sprintf("%s %s (%s)", methodVerb(hit.method), hit.label, hit.where))
	}
	r.scanUnknownDotdir(abs, no, line)
}

// prefixEnd is where the command text before a path match ends: a leading
// delimiter (space, quote, '=', '<', '@', '(') the pattern consumed still belongs
// to the command.
func prefixEnd(line string, start int) int {
	if start < len(line) && strings.IndexByte("=<@ \t\"'(", line[start]) >= 0 {
		return start + 1
	}
	return start
}

func credentialSeverity(tier, method string) Severity {
	if method == methodCopy || method == methodExfil {
		return SeverityError
	}
	switch tier {
	case tierCritical, tierHigh:
		return SeverityWarning
	}
	return SeverityInfo
}

func methodVerb(method string) string {
	switch method {
	case methodCopy:
		return "copies"
	case methodExfil:
		return "uploads"
	case methodDD:
		return "dumps"
	case methodRedirect:
		return "feeds into a command"
	}
	return "reads"
}

var (
	dotdirRe     = regexp.MustCompile(home + `/\.([A-Za-z0-9][A-Za-z0-9_-]*)/`)
	benignDotdir = map[string]bool{
		"claude": true, presetCursor: true, presetCodex: true, keyAgents: true, "cache": true, "local": true, "config": true, "gemini": true,
		"windsurf": true, "vscode": true, cmdNPM: true, "nvm": true, "rustup": true, "pyenv": true, "rbenv": true, "bundle": true, "m2": true,
		"gradle": true, "oh-my-zsh": true, "zsh": true, "tmux": true, "fzf": true, "cargo": true, "bun": true, "deno": true, "volta": true,
		"sdkman": true, "asdf": true, "mise": true, "rtx": true, "pnpm": true, "yarn": true, "ai-rulez": true, "basemind": true, "copilot": true,
		"kiro": true, "continue": true, "cline": true, "roo": true, "amp": true, "opencode": true, "junie": true, "trae": true,
	}
	tableDotdirs = regexp.MustCompile(`^(?:ssh|aws|gnupg|kube|docker|azure|terraform\.d|gem|op|age|password-store|openvpn|vault-token)$`)
)

func (r *runner) scanUnknownDotdir(abs string, no int, line string) {
	if r.sev[CodeUnknownDotdir] == SeverityOff {
		return
	}
	for _, loc := range dotdirRe.FindAllStringSubmatchIndex(line, -1) {
		dir := line[loc[2]:loc[3]]
		if benignDotdir[dir] || tableDotdirs.MatchString(dir) {
			continue
		}
		if accessMethod(line[:loc[0]]) == methodRead {
			r.add(CodeUnknownDotdir, abs, no, "reads ~/.%s/, a hidden directory that is not a known tool directory; check that it holds no credentials", dir)
			return
		}
	}
}
