package contentlock

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
)

// KindVerifier pins one verifier declaration: a flat or inline [[verifiers]]
// entry of config.toml, or one table of .ai-rulez/verifiers/*.toml. The id is
// the verifier id; Path is the declaration file (empty for config.toml).
// Weakening a severity, widening an exclude or deleting a verifier changes the
// pin, so it shows in review. Verifiers that arrive through an include are
// covered by the include's own pin.
const KindVerifier = "verifier"

// collectVerifiers pins the project's own verifier declarations and the
// [verifiers_settings] that govern them.
func (c *collector) collectVerifiers() error {
	for i := range c.cfg.Verifiers {
		v := &c.cfg.Verifiers[i]
		if err := c.addVerifier(v.Name, "", v); err != nil {
			return err
		}
	}
	if err := c.collectVerifierFiles(); err != nil {
		return err
	}
	if s := c.cfg.VerifiersSettings; s != nil {
		data, err := canonicalJSON(s)
		if err != nil {
			return err
		}
		digest, err := TreeDigest(KindSettings, []Leaf{{Path: "verifiers-settings.json", Mode: ModeRegular, Data: data}})
		if err != nil {
			return err
		}
		c.items = append(c.items, lockfile.Item{Kind: KindSettings, ID: "verifiers-settings", Digest: digest})
	}
	return nil
}

func (c *collector) addVerifier(id, path string, decl any) error {
	data, err := canonicalJSON(decl)
	if err != nil {
		return err
	}
	digest, err := TreeDigest(KindVerifier, []Leaf{{Path: "verifier.json", Mode: ModeRegular, Data: data}})
	if err != nil {
		return err
	}
	c.items = append(c.items, lockfile.Item{Kind: KindVerifier, ID: id, Path: path, Digest: digest})
	return nil
}

// collectVerifierFiles pins each [[verifiers]] table of the declaration files in
// the configuration directory. The tables are read generically (never
// validated), so a malformed or unknown key still changes the pin; a file that
// cannot be read or parsed is a problem a check fails on.
func (c *collector) collectVerifierFiles() error {
	if c.cfg.ConfigDir == "" {
		return nil
	}
	dir := filepath.Join(c.cfg.ConfigDir, config.VerifiersDirName)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return oops.With("dir", dir).Wrapf(err, "read the verifiers directory for the lock")
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".toml") {
			continue
		}
		rel := config.VerifiersDirName + "/" + e.Name()
		path := filepath.Join(dir, e.Name())
		if info, statErr := os.Lstat(path); statErr != nil || !info.Mode().IsRegular() {
			c.problems = append(c.problems, "verifier file "+rel+" is not a regular file, so it cannot be pinned")
			continue
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			c.problems = append(c.problems, "verifier file "+rel+" cannot be read: "+readErr.Error())
			continue
		}
		var doc struct {
			Verifiers []map[string]any `toml:"verifiers"`
		}
		if err := toml.Unmarshal(data, &doc); err != nil {
			c.problems = append(c.problems, "verifier file "+rel+" is not valid TOML, so it cannot be pinned")
			continue
		}
		for i, table := range doc.Verifiers {
			id, _ := table["id"].(string)
			if id == "" {
				id = rel + "#" + strconv.Itoa(i+1)
			}
			if err := c.addVerifier(id, rel, table); err != nil {
				return err
			}
		}
	}
	return nil
}
