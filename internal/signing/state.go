package signing

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/llm"
	"github.com/Goldziher/ai-rulez/v5/internal/safefs"
)

const (
	stateFile      = "signing-state.json"
	stateSecret    = "signing-state.key"
	stateVersion   = 1
	stateMACLabel  = "ai-rulez signing state v1"
	maxStateBytes  = 1 << 20
	stateDirName   = "ai-rulez"
	stateHomeParts = ".local/state"
)

// State is the per-user rollback record: for each signer and project, the latest
// signing time of any attestation verified on this machine (see stateKey). It lives outside the
// repository (a checkout cannot plant or reset it) and is authenticated with an
// HMAC under a per-user secret, as the LLM response cache is. It makes rollback
// detectable per machine, not impossible: a fresh machine starts empty.
type State struct {
	path      string
	secret    []byte
	highwater map[string]time.Time
	heads     map[string]digestHead
	// Reset is true when the file existed but failed authentication and was
	// discarded; the caller should warn.
	Reset bool
}

// digestHead is the rollback record of a signed body that carries no signing
// time (a key-signed blob bundle): the body last accepted and the ones it
// replaced. A retired body can never be accepted again on this machine.
type digestHead struct {
	Current string   `json:"current"`
	Retired []string `json:"retired,omitempty"`
}

// maxRetired bounds the replaced digests kept per key.
const maxRetired = 64

type statePayload struct {
	Highwater map[string]time.Time  `json:"highwater"`
	Heads     map[string]digestHead `json:"heads,omitempty"`
}

type stateEnvelope struct {
	V       int             `json:"v"`
	MAC     string          `json:"mac"`
	Payload json.RawMessage `json:"payload"`
}

// StatePaths returns the state file and the HMAC secret file: the state under
// $XDG_STATE_HOME (else ~/.local/state), the secret in the user config directory
// next to the LLM cache secret. Either is "" without a home directory.
func StatePaths(env ambient.Env) (state, secret string) {
	secret = llm.UserSecretPathIn(env, stateSecret)
	if xdg := ambient.Getenv(env, "XDG_STATE_HOME"); xdg != "" && filepath.IsAbs(xdg) {
		return filepath.Join(xdg, stateDirName, stateFile), secret
	}
	home, err := ambient.OrOS(env).UserHomeDir()
	if err != nil || home == "" {
		return "", secret
	}
	return filepath.Join(home, filepath.FromSlash(stateHomeParts), stateDirName, stateFile), secret
}

// OpenState loads the state at path authenticated with the secret at secretPath
// (created on first use). A missing file is an empty state; a file that fails its
// MAC is discarded (Reset). Without usable paths it returns an error.
func OpenState(path, secretPath string) (*State, error) {
	if path == "" || secretPath == "" {
		return nil, oops.Errorf("no user directory for the signing state")
	}
	secret, err := llm.LoadSecretFile(secretPath)
	if err != nil {
		return nil, oops.Wrapf(err, "load the signing state secret")
	}
	s := &State{path: path, secret: secret, highwater: map[string]time.Time{}, heads: map[string]digestHead{}}
	data, err := safefs.ReadRegular(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return s, nil
	case err != nil:
		s.Reset = true
		return s, nil
	case len(data) > maxStateBytes:
		s.Reset = true
		return s, nil
	}
	var env stateEnvelope
	if json.Unmarshal(data, &env) != nil || env.V != stateVersion || !hmac.Equal([]byte(env.MAC), []byte(s.mac(env.Payload))) {
		s.Reset = true
		return s, nil
	}
	var p statePayload
	if json.Unmarshal(env.Payload, &p) != nil {
		s.Reset = true
		return s, nil
	}
	if p.Highwater != nil {
		s.highwater = p.Highwater
	}
	if p.Heads != nil {
		s.heads = p.Heads
	}
	return s, nil
}

func (s *State) mac(payload []byte) string {
	m := hmac.New(sha256.New, s.secret)
	m.Write([]byte(stateMACLabel))
	m.Write([]byte{0})
	m.Write(payload)
	return hex.EncodeToString(m.Sum(nil))
}

// Highwater returns the recorded time for key and whether there is one.
func (s *State) Highwater(key string) (time.Time, bool) {
	t, ok := s.highwater[key]
	return t, ok
}

// Check reports AR727 when signedAt is older than the high-water mark of key.
func (s *State) Check(key string, signedAt time.Time) error {
	if hw, ok := s.highwater[key]; ok && signedAt.Before(hw) {
		return Errorf(CodeRollback, "signed %s, but this machine has already verified an attestation from %s; if that newer one was wrong, delete %s (or pass --no-state)", signedAt.UTC().Format(time.RFC3339), hw.UTC().Format(time.RFC3339), s.path)
	}
	return nil
}

// Advance raises the high-water mark of key to signedAt and writes the state.
func (s *State) Advance(key string, signedAt time.Time) error {
	if hw, ok := s.highwater[key]; ok && !signedAt.After(hw) {
		return nil
	}
	s.highwater[key] = signedAt.UTC()
	return s.save()
}

// checkAll is Check against every key.
func (s *State) checkAll(keys []string, signedAt time.Time) error {
	for _, k := range keys {
		if err := s.Check(k, signedAt); err != nil {
			return err
		}
	}
	return nil
}

// advanceAll is Advance for every key.
func (s *State) advanceAll(keys []string, signedAt time.Time) error {
	for _, k := range keys {
		if err := s.Advance(k, signedAt); err != nil {
			return err
		}
	}
	return nil
}

// CheckDigest reports AR727 when digest is a body this machine has already
// replaced under key. It is the rollback check for an attestation with no
// signing time, where Check has nothing to compare: it covers the versions this
// machine has seen, no more.
func (s *State) CheckDigest(key, digest string) error {
	if slices.Contains(s.heads[key].Retired, digest) {
		return Errorf(CodeRollback, "%s was replaced by a newer version this machine has already verified; the signature has no signing time, so the old version cannot be told from a rollback; if the newer one was wrong, delete %s (or pass --no-state)", digest, s.path)
	}
	return nil
}

// AdvanceDigest records digest as the current body of key, retiring the one it
// replaces, and writes the state.
func (s *State) AdvanceDigest(key, digest string) error {
	head := s.heads[key]
	if head.Current == digest {
		return nil
	}
	if head.Current != "" && !slices.Contains(head.Retired, head.Current) {
		head.Retired = append(head.Retired, head.Current)
		if len(head.Retired) > maxRetired {
			head.Retired = head.Retired[len(head.Retired)-maxRetired:]
		}
	}
	head.Current = digest
	s.heads[key] = head
	return s.save()
}

func (s *State) save() error {
	payload, err := json.Marshal(statePayload{Highwater: s.highwater, Heads: s.heads})
	if err != nil {
		return oops.Wrapf(err, "encode the signing state")
	}
	data, err := json.Marshal(stateEnvelope{V: stateVersion, MAC: s.mac(payload), Payload: payload})
	if err != nil {
		return oops.Wrapf(err, "encode the signing state")
	}
	return safefs.WriteFileAtomic(s.path, data)
}
