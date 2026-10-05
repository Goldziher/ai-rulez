package importer

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/lint"
)

// A credential is never carried into config.toml: every string of an MCP server
// (arguments, URL, environment, headers) is checked and a literal secret is
// replaced by a ${VAR} reference. The value is never printed.

var (
	credentialName = regexp.MustCompile(`(?i)(key|token|secret|password|passwd|credential|auth|bearer)`)
	// envRef is the only form treated as an existing reference.
	envRef = regexp.MustCompile(`^\$\{?[A-Z_][A-Z0-9_]*\}?$`)
	// connectionString is scheme://user:password@host.
	connectionString = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.-]*://[^/\s:@]*:[^/\s@]+@`)
	authScheme       = regexp.MustCompile(`^(?i)(bearer|basic|token|digest|apikey)\s+(.*)$`)
	urlParts         = regexp.MustCompile(`^([A-Za-z][A-Za-z0-9+.-]*://)([^/?#]*)([^?#]*)(?:\?([^#]*))?(#.*)?$`)
	refUserinfo      = regexp.MustCompile(`^\$\{[A-Z_][A-Z0-9_]*\}(?::\$\{[A-Z_][A-Z0-9_]*\})?$`)
	nonAlnum         = regexp.MustCompile(`[^A-Za-z0-9]+`)
	envAssignKey     = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

func isRef(v string) bool { return envRef.MatchString(v) }

func looksSecret(v string) bool {
	if _, ok := lint.DetectSecret(v); ok {
		return true
	}
	return connectionString.MatchString(v)
}

func envVarName(key string) string {
	name := strings.Trim(strings.ToUpper(nonAlnum.ReplaceAllString(key, "_")), "_")
	if name == "" || (name[0] >= '0' && name[0] <= '9') {
		name = "MCP_" + name
	}
	return strings.TrimSuffix(name, "_")
}

func refFor(key string) string { return "${" + envVarName(key) + "}" }

// credentialScan redacts the strings of one MCP server and records a
// needs-action finding for each one it replaced.
type credentialScan struct {
	plan   *Plan
	file   string
	field  string
	server string
}

func (c credentialScan) redacted(field, ref string) {
	c.plan.add(newFinding(StatusNeedsAction, c.file, field, "",
		"literal credential replaced by a "+ref+" reference; export the variable before generating"))
}

// redactValue returns v with a literal secret replaced by a reference to key.
// header values keep a leading auth scheme (Bearer, Basic, ...).
func redactValue(key, v string, header bool) (string, bool) {
	if v == "" || isRef(v) {
		return v, false
	}
	prefix, rest := "", v
	if header {
		if m := authScheme.FindStringSubmatch(v); m != nil {
			prefix, rest = m[1]+" ", m[2]
			if isRef(rest) {
				return v, false
			}
		}
	}
	if credentialName.MatchString(key) || looksSecret(rest) {
		return prefix + refFor(key), true
	}
	return v, false
}

// values redacts a map of environment variables or headers.
func (c credentialScan) values(suffix string, in map[string]string, header bool) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	keys := make([]string, 0, len(in))
	for k := range in {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v, hit := redactValue(k, in[k], header)
		out[k] = v
		if hit {
			c.redacted(c.field+suffix+"."+k, refFor(k))
		}
	}
	return out
}

// args redacts a command line: `--api-key value`, `--token=value`, `KEY=value`
// (docker -e), URLs with credentials and bare secrets.
func (c credentialScan) args(in []string) []string {
	if len(in) == 0 {
		return in
	}
	out := make([]string, 0, len(in))
	for i := 0; i < len(in); i++ {
		a := in[i]
		field := fmt.Sprintf("%s.args[%d]", c.field, i)
		switch {
		case strings.HasPrefix(a, "-"):
			name, val, hasVal := strings.Cut(a, "=")
			flag := strings.TrimLeft(name, "-")
			if hasVal {
				if v, hit := redactValue(flag, val, false); hit {
					out = append(out, name+"="+v)
					c.redacted(field, refFor(flag))
					continue
				}
				out = append(out, name+"="+c.url(field, val))
				continue
			}
			out = append(out, a)
			if credentialName.MatchString(flag) && i+1 < len(in) && !strings.HasPrefix(in[i+1], "-") && !isRef(in[i+1]) {
				i++
				out = append(out, refFor(flag))
				c.redacted(fmt.Sprintf("%s.args[%d]", c.field, i), refFor(flag))
			}
		default:
			if k, v, ok := strings.Cut(a, "="); ok && envAssignKey.MatchString(k) {
				if nv, hit := redactValue(k, v, false); hit {
					out = append(out, k+"="+nv)
					c.redacted(field, refFor(k))
					continue
				}
			}
			if looksSecret(a) && !strings.Contains(a, "://") {
				ref := refFor(c.server + "_ARG" + fmt.Sprint(i))
				out = append(out, ref)
				c.redacted(field, ref)
				continue
			}
			out = append(out, c.url(field, a))
		}
	}
	return out
}

// url redacts the userinfo and credential query parameters of a URL. Anything
// that is not a URL is returned unchanged.
func (c credentialScan) url(field, raw string) string {
	m := urlParts.FindStringSubmatch(raw)
	if m == nil {
		return raw
	}
	scheme, authority, rest, query, frag := m[1], m[2], m[3], m[4], m[5]
	changed := false
	if at := strings.LastIndex(authority, "@"); at >= 0 {
		if userinfo := authority[:at]; !refUserinfo.MatchString(userinfo) {
			ref := refFor(c.server + "_URL_USERINFO")
			authority = ref + authority[at:]
			c.redacted(field, ref)
			changed = true
		}
	}
	if query != "" {
		params := strings.Split(query, "&")
		for i, p := range params {
			k, v, ok := strings.Cut(p, "=")
			if !ok {
				continue
			}
			if nv, hit := redactValue(k, v, false); hit {
				params[i] = k + "=" + nv
				c.redacted(field, refFor(k))
				changed = true
			}
		}
		query = strings.Join(params, "&")
	}
	if !changed {
		return raw
	}
	out := scheme + authority + rest
	if query != "" {
		out += "?" + query
	}
	return out + frag
}
