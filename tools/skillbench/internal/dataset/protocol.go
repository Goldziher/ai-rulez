// Package dataset pins, verifies and materializes the benchmark's samples. Sample text is written
// only to the materialized tree and never to a result, a log or an artifact.
package dataset

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
)

// Protocol is the frozen benchmark definition from protocol.toml.
type Protocol struct {
	Version           int                `toml:"version"`
	Claim             string             `toml:"claim"`
	Repetitions       int                `toml:"repetitions"`
	HoldoutReadBudget int                `toml:"holdout_read_budget"`
	Dataset           Source             `toml:"dataset"`
	Tracks            map[string]Track   `toml:"tracks"`
	Clean             Clean              `toml:"clean"`
	Scoring           Scoring            `toml:"scoring"`
	Scanners          map[string]Scanner `toml:"scanners"`

	// SHA256 is the digest of the file, which is what a result records.
	SHA256 string `toml:"-"`
}

// Source names one pinned dataset revision.
type Source struct {
	Name           string   `toml:"name"`
	Repo           string   `toml:"repo"`
	Revision       string   `toml:"revision"`
	HoldoutSources []string `toml:"holdout_sources"`
	Files          []File   `toml:"files"`
}

// Clean pins the real skills of the false-alarm track.
type Clean struct {
	Population string      `toml:"population_sha256"`
	Repos      []CleanRepo `toml:"repos"`
}

// CleanRepo is one repository at one commit.
type CleanRepo struct {
	Repo   string `toml:"repo"`
	Commit string `toml:"commit"`
}

// File is one pinned file of the dataset.
type File struct {
	Path   string `toml:"path"`
	SHA256 string `toml:"sha256"`
}

// Track is a named population. Counts of zero are not checked.
type Track struct {
	Malicious int  `toml:"malicious"`
	Benign    int  `toml:"benign"`
	Holdout   bool `toml:"holdout"`
}

// Scoring fixes the statistics in advance.
type Scoring struct {
	BootstrapResamples int      `toml:"bootstrap_resamples"`
	BootstrapSeed      uint64   `toml:"bootstrap_seed"`
	SeveritySweep      []string `toml:"severity_sweep"`
}

// Scanner pins one tool's release.
type Scanner struct {
	Release string `toml:"release"`
}

// LoadProtocol reads and validates the protocol file.
func LoadProtocol(path string) (*Protocol, error) {
	data, err := os.ReadFile(path) //nolint:gosec // the operator names the protocol file
	if err != nil {
		return nil, fmt.Errorf("read protocol: %w", err)
	}
	var p Protocol
	if _, err := toml.Decode(string(data), &p); err != nil {
		return nil, fmt.Errorf("parse protocol: %w", err)
	}
	if p.Dataset.Revision == "" || len(p.Dataset.Files) == 0 {
		return nil, fmt.Errorf("protocol: dataset revision and files are required")
	}
	sum := sha256.Sum256(data)
	p.SHA256 = hex.EncodeToString(sum[:])
	return &p, nil
}
