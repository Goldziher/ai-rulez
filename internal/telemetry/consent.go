package telemetry

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/safefs"
	"github.com/samber/oops"
)

// ConsentFileName is the per-user consent record inside the user config
// directory ($XDG_CONFIG_HOME/ai-rulez, else ~/.config/ai-rulez).
const ConsentFileName = "telemetry-consent.json"

// ConsentVersion is the record's format version. A record with another version
// is not honored: consent is never carried across a format change by guessing.
const ConsentVersion = 1

// Consent is the stored record that `telemetry enable` writes. It says which
// endpoint and which set of exported fields the user agreed to, so that changing
// either (a new endpoint, a widened allowlist after an upgrade, opting in to
// sessions or paths) invalidates it instead of silently sending more.
//
// The record lives only in the user config directory. A repository cannot supply,
// edit or point to one: nothing in a project's config or its checkout is ever read
// as consent.
type Consent struct {
	Version  int    `json:"version"`
	Endpoint string `json:"endpoint"`
	Protocol string `json:"protocol"`
	// Scope is what the user consented to export.
	Scope ConsentScope `json:"scope"`
	// GrantedAt is the RFC 3339 UTC time of `telemetry enable`.
	GrantedAt string `json:"granted_at"`
	// AIRulezVersion is the release that wrote the record.
	AIRulezVersion string `json:"ai_rulez_version,omitempty"`
}

// ConsentScope is the field set of a consent: the opt-in gates and a hash of the
// exported attribute names, so a release that adds an attribute to the allowlist
// does not export it under an older consent.
type ConsentScope struct {
	IncludePaths   bool `json:"include_paths,omitempty"`
	IncludeSession bool `json:"include_session,omitempty"`
	// Fields are the attribute names exported at grant time, in allowlist order.
	Fields []string `json:"fields"`
	// FieldsHash is the sha256 of Fields.
	FieldsHash string `json:"fields_hash"`
}

// ConsentPath is where the consent record lives, "" without a home directory.
func ConsentPath(getenv func(string) string) string {
	dir := config.UserConfigDir(getenv, "")
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, ConsentFileName)
}

// NormalizeEndpoint is the form two endpoints are compared in: lower-case scheme
// and host, no trailing slash. "" when the endpoint is not a URL with a host.
func NormalizeEndpoint(endpoint string) string {
	parsed, err := url.Parse(strings.TrimRight(strings.TrimSpace(endpoint), "/"))
	if err != nil || parsed.Host == "" {
		return ""
	}
	return strings.ToLower(parsed.Scheme) + "://" + strings.ToLower(parsed.Host) + parsed.Path
}

// ScopeFor builds the consent scope for the given opt-in gates: the attribute
// names the encoder would export and their hash.
func ScopeFor(includePaths, includeSession bool) ConsentScope {
	fields, _ := (&Encoder{IncludePaths: includePaths, IncludeSession: includeSession}).Fields()
	sum := sha256.Sum256([]byte(strings.Join(fields, "\n")))
	return ConsentScope{IncludePaths: includePaths, IncludeSession: includeSession, Fields: fields, FieldsHash: hex.EncodeToString(sum[:])}
}

// NewConsent builds the record for an endpoint, protocol and gates.
func NewConsent(endpoint, protocol string, includePaths, includeSession bool, now time.Time, version string) Consent {
	return Consent{
		Version: ConsentVersion, Endpoint: NormalizeEndpoint(endpoint), Protocol: protocol,
		Scope: ScopeFor(includePaths, includeSession), GrantedAt: formatOrderTime(now), AIRulezVersion: version,
	}
}

// Check reports why a record does not cover the effective settings, "" when it
// does: the endpoint, protocol and exported field set must all match.
func (c *Consent) Check(endpoint, protocol string, includePaths, includeSession bool) string {
	switch {
	case c.Version != ConsentVersion:
		return fmt.Sprintf("the consent record has version %d, this release reads %d", c.Version, ConsentVersion)
	case NormalizeEndpoint(endpoint) == "" || c.Endpoint != NormalizeEndpoint(endpoint):
		return "the endpoint changed since consent was given"
	case c.Protocol != protocol:
		return "the protocol changed since consent was given"
	case c.Scope.FieldsHash != ScopeFor(includePaths, includeSession).FieldsHash:
		return "the exported fields changed since consent was given (an opt-in or the allowlist differs)"
	}
	return ""
}

// ErrConsentUnsafe means the record file is writable by others, so it cannot be
// trusted to have been written by the user.
var ErrConsentUnsafe = errors.New("consent record is group- or world-writable")

// LoadConsent reads the record at path. A missing file is (nil, nil). A file that
// cannot be trusted (a symlink, loose permissions, bad JSON, an unknown field) is
// an error: the caller treats it as no consent and reports why.
func LoadConsent(path string) (*Consent, error) {
	if path == "" {
		return nil, nil //nolint:nilnil // no home directory: no record
	}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil //nolint:nilnil // no record is the normal off state
	}
	if err != nil {
		return nil, oops.Wrapf(err, "stat consent record")
	}
	// Windows reports a fixed 0666 for every file, so the mode says nothing there; its
	// user config directory is private to the account.
	if runtime.GOOS != "windows" && info.Mode().IsRegular() && info.Mode().Perm()&0o022 != 0 {
		return nil, oops.With("path", path).Wrapf(ErrConsentUnsafe, "read consent record")
	}
	data, err := safefs.ReadRegular(path)
	if err != nil {
		return nil, oops.With("path", path).Wrapf(err, "read consent record")
	}
	var c Consent
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&c); err != nil {
		return nil, oops.With("path", path).Wrapf(err, "parse consent record")
	}
	return &c, nil
}

// SaveConsent writes the record atomically with mode 0600.
func SaveConsent(path string, c *Consent) error {
	if path == "" {
		return oops.Hint("Set HOME or XDG_CONFIG_HOME.").Errorf("no user config directory to store the consent record")
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return oops.Wrapf(err, "encode consent record")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return oops.Wrapf(err, "create user config directory")
	}
	return safefs.WriteFileAtomic(path, append(data, '\n')) //nolint:wrapcheck // safefs errors carry the path
}

// RemoveConsent deletes the record; a missing one is not an error. It reports
// whether a record was removed.
func RemoveConsent(path string) (bool, error) {
	if path == "" {
		return false, nil
	}
	err := os.Remove(path)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, os.ErrNotExist):
		return false, nil
	}
	return false, oops.With("path", path).Wrapf(err, "remove consent record")
}
