package lint

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// Codes for the capability analysis.
const (
	CodeCapabilityRisk = "AR030"
	CodeCrossItemChain = "AR031"
)

func init() {
	registerRules(
		RuleInfo{CodeCapabilityRisk, "capability-profile-risk", SeverityWarning, "the commands an item runs combine capabilities that are risky together: destructive with network, many network commands, interpreter with network"},
		RuleInfo{CodeCrossItemChain, "cross-item-exfil-chain", SeverityWarning, "items of one bundle split a dangerous capability between them: one reads credentials, another has network; stealth beside a high-risk item"},
	)
	registerRunCheck(checkCapabilities, AnalyzerSecurity)
}

// Command tiers.
const (
	tierRead        = iota // read-only
	tierMutate             // mutating
	tierDestructive        // destructive
	tierNetwork            // network
	tierPrivilege          // privilege
	tierStealth            // stealth
	tierInterp             // interpreter
	tierCount
)

var tierNames = [tierCount]string{"read-only", "mutating", "destructive", "network", "privilege", "stealth", "interpreter"}

// commandTiers classifies commands by basename. Commands not listed are unknown
// and do not count. Derived from each tool's documented purpose.
var commandTiers = buildTiers(map[int]string{
	tierRead:        "cat ls head tail grep egrep fgrep rg ag find wc sort uniq cut tr diff cmp jq yq echo printf pwd which whereis type stat file date basename dirname realpath readlink du df ps top uname whoami id hostname env printenv tree less more column awk sed xxd od hexdump strings nl tee test true false sleep seq",
	tierMutate:      "mkdir touch cp mv ln git npm pnpm yarn pip pip3 uv go cargo make cmake gradle mvn bundle gem composer dotnet docker podman kubectl helm terraform ansible tar zip unzip gzip gunzip bzip2 xz patch install chmod",
	tierDestructive: "dd mkfs fdisk parted wipefs kill killall pkill truncate reboot shutdown halt poweroff",
	tierNetwork:     "curl wget ssh scp sftp rsync nc ncat netcat socat nmap dig nslookup host telnet ftp http xh httpie aria2c lynx mtr traceroute",
	tierPrivilege:   "sudo su doas chown chgrp mount umount systemctl service useradd usermod userdel groupadd passwd crontab modprobe insmod iptables ufw setcap visudo launchctl",
	tierStealth:     "shred chattr",
	tierInterp:      "python python3 node ruby perl lua php bun deno tsx ts-node pwsh powershell osascript Rscript npx bunx uvx",
})

func buildTiers(in map[int]string) map[string]int {
	out := map[string]int{}
	for tier, names := range in {
		for _, n := range strings.Fields(names) {
			out[n] = tier
		}
	}
	return out
}

// capProfile counts the commands of each tier an item runs.
type capProfile struct {
	counts   [tierCount]int
	credRead bool
	stealth  bool
}

func (p capProfile) describe() string {
	var parts []string
	for tier := tierDestructive; tier < tierCount; tier++ {
		if p.counts[tier] > 0 {
			parts = append(parts, fmt.Sprintf("%s:%d", tierNames[tier], p.counts[tier]))
		}
	}
	return strings.Join(parts, " ")
}

// classify adds the commands of one shell line to the profile.
func (p *capProfile) classify(line string) {
	for _, seg := range segSplitRe.Split(stripShellComment(line), -1) {
		words := shellWords(strings.TrimSpace(seg))
		if len(words) > 0 && words[0] == cmdSudo {
			p.counts[tierPrivilege]++
		}
		cmd := commandWord(words)
		tier, known := commandTiers[cmd]
		if cmd == "rm" {
			tier, known = tierMutate, true
			for _, a := range words {
				if strings.HasPrefix(a, "-") && !strings.HasPrefix(a, "--") && strings.ContainsAny(a, "rRf") {
					tier = tierDestructive
				}
			}
		}
		if known {
			p.counts[tier]++
		}
	}
	if _, ok := detectCredentialAccess(line); ok {
		p.credRead = true
	}
	for _, re := range stealthRes {
		if re.MatchString(line) {
			p.stealth = true
			p.counts[tierStealth]++
			break
		}
	}
}

// itemProfile scans the shell the item ships: fenced shell blocks of its
// SKILL.md and markdown resources, and its shell scripts.
func (r *runner) itemProfile(it *item) capProfile {
	var p capProfile
	scan := func(abs, raw string) {
		t := newScanText(r, abs, raw)
		if !t.md && !isShellScript(t) {
			return
		}
		for _, l := range t.logicalLines() {
			if t.shellLike(l) {
				p.classify(l.Text)
			}
		}
	}
	if d, ok := r.docs[it.abs]; ok {
		scan(it.abs, strings.Join(d.lines, "\n"))
	}
	for i := range r.items {
		if other := &r.items[i]; other.isDoc && other.itemDir == it.itemDir {
			if d, ok := r.docs[other.abs]; ok {
				scan(other.abs, strings.Join(d.lines, "\n"))
			}
		}
	}
	for _, res := range it.cf.Resources {
		if res.Kind == config.SkillKindScripts && !strings.HasSuffix(strings.ToLower(res.RelPath), ".md") {
			scan(filepath.Join(it.itemDir, filepath.FromSlash(res.RelPath)), string(res.Content))
		}
	}
	return p
}

// maxNetworkCommands is how many network commands an item may run before AR030
// calls it network-heavy, unless [lint.capability] max_network_commands sets it.
const maxNetworkCommands = DefaultMaxNetworkCommands

// DefaultMaxNetworkCommands is the built-in network-command limit of AR030.
const DefaultMaxNetworkCommands = 5

func (r *runner) maxNetwork() int {
	if c := r.lc.Capability; c != nil && c.MaxNetworkCommands != nil && *c.MaxNetworkCommands >= 0 {
		return *c.MaxNetworkCommands
	}
	return maxNetworkCommands
}

// highRiskCodes are the findings that make an item "high risk" for AR031.
var highRiskCodes = map[string]bool{CodeShellExec: true, CodeExfilCommand: true, CodeCredentialTaint: true, CodeStealthCommand: true, CodeSecretDetected: true}

type capItem struct {
	it   *item
	id   string
	p    capProfile
	high bool
	line int
}

func checkCapabilities(r *runner) {
	byDomain := map[string][]capItem{}
	var domains []string
	for i := range r.items {
		it := &r.items[i]
		if !it.owned || it.isDoc || it.kind != kindSkill || it.itemDir == "" {
			continue
		}
		ci := capItem{it: it, id: itemID(it.kind, it.cf), p: r.itemProfile(it), line: r.docs[it.abs].lineOf(keyName, 1)}
		ci.high = r.itemHasFindings(it, highRiskCodes)
		r.reportProfile(ci)
		if _, seen := byDomain[it.domain]; !seen {
			domains = append(domains, it.domain)
		}
		byDomain[it.domain] = append(byDomain[it.domain], ci)
	}
	sort.Strings(domains)
	for _, d := range domains {
		r.crossItem(d, byDomain[d])
	}
}

func (r *runner) itemHasFindings(it *item, codes map[string]bool) bool {
	prefix := r.display(it.itemDir) + "/"
	for _, f := range r.findings {
		if codes[f.Code] && strings.HasPrefix(f.File, prefix) && f.Severity.AtLeast(SeverityWarning) {
			return true
		}
	}
	return false
}

func (r *runner) reportProfile(ci capItem) {
	c := ci.p.counts
	switch {
	case c[tierDestructive] > 0 && c[tierNetwork] > 0:
		r.add(CodeCapabilityRisk, ci.it.abs, ci.line, "%s runs destructive and network commands together (%s); split them or confirm the combination is intended", ci.id, ci.p.describe())
	case c[tierInterp] > 0 && c[tierNetwork] > 0:
		r.addSev(SeverityInfo, CodeCapabilityRisk, ci.it.abs, ci.line, "%s runs an interpreter and network commands (%s); code it generates could reach the network", ci.id, ci.p.describe())
	case c[tierNetwork] > r.maxNetwork():
		r.addSev(SeverityInfo, CodeCapabilityRisk, ci.it.abs, ci.line, "%s runs %d network commands (%s), more than %d", ci.id, c[tierNetwork], ci.p.describe(), r.maxNetwork())
	}
}

func (r *runner) crossItem(domain string, items []capItem) { //nolint:gocyclo // linear checks over a documented schema; splitting them hides the rules
	if len(items) < 2 {
		return
	}
	type rule struct {
		name string
		sev  Severity
		a, b func(capItem) bool
		why  string
	}
	rules := []rule{
		{"exfil", SeverityWarning, func(x capItem) bool { return x.p.credRead && x.p.counts[tierNetwork] == 0 }, func(x capItem) bool { return x.p.counts[tierNetwork] > 0 && !x.p.credRead },
			"%q reads credentials and has no network access, %q has network access and reads none: run together they form an exfiltration chain"},
		{"stealth", SeverityWarning, func(x capItem) bool { return x.p.stealth }, func(x capItem) bool { return x.high },
			"%q erases history and %q carries a high-risk finding: the pair hides what the second does"},
		{"privilege", SeverityInfo, func(x capItem) bool { return x.p.counts[tierPrivilege] > 0 && x.p.counts[tierNetwork] == 0 }, func(x capItem) bool { return x.p.counts[tierNetwork] > 0 && x.p.counts[tierPrivilege] == 0 },
			"%q uses privileged commands and has no network access, %q has network access: together they can fetch and install with elevated rights"},
		{"credential-interpreter", SeverityInfo, func(x capItem) bool { return x.p.credRead && x.p.counts[tierNetwork] == 0 }, func(x capItem) bool { return x.p.counts[tierInterp] > 0 && !x.p.credRead },
			"%q reads credentials and %q runs an interpreter: code the interpreter runs could use what the first read"},
	}
	for _, ru := range rules {
		var first *[2]capItem
		pairs := 0
		for _, a := range items {
			for _, b := range items {
				if a.it == b.it || !ru.a(a) || !ru.b(b) {
					continue
				}
				if first == nil {
					first = &[2]capItem{a, b}
				}
				pairs++
			}
		}
		if first == nil {
			continue
		}
		where := "domain " + domain
		if domain == "" {
			where = "the root bundle"
		}
		msg := fmt.Sprintf(ru.why, first[0].id, first[1].id) + " (" + where
		if pairs > 1 {
			msg += fmt.Sprintf(", and %d more pair(s)", pairs-1)
		}
		r.addSev(ru.sev, CodeCrossItemChain, first[0].it.abs, first[0].line, "%s)", msg)
	}
}
