package evals

import (
	"os"

	"github.com/Goldziher/ai-rulez/v5/internal/llm"
)

// ResultsKeyFile is the per-user key that signs eval results, beside the LLM
// cache secret in the user config directory (never in the repository).
const ResultsKeyFile = "eval-results.key"

// UserKey returns the per-user MAC key for the results store, creating it on
// first use (`eval run` is the writer). It returns nil when no key can be
// created; without it every stored result counts as unverified and is re-run,
// never trusted.
func UserKey() []byte {
	key, err := llm.LoadSecretFile(llm.UserSecretPath(ResultsKeyFile))
	if err != nil {
		return nil
	}
	return key
}

// ExistingUserKey is UserKey for readers: it never creates the key, so a machine
// that never ran `eval run` has none and every recorded result is unverified.
func ExistingUserKey() []byte {
	path := llm.UserSecretPath(ResultsKeyFile)
	if path == "" {
		return nil
	}
	if _, err := os.Lstat(path); err != nil {
		return nil
	}
	key, err := llm.LoadSecretFile(path)
	if err != nil {
		return nil
	}
	return key
}
