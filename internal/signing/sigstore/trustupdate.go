package signing

import (
	"path/filepath"

	"github.com/samber/oops"
	"github.com/sigstore/sigstore-go/pkg/root"
	"github.com/sigstore/sigstore-go/pkg/tuf"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/safefs"
)

// UpdateTrustedRoot fetches the public-good Sigstore trusted root over TUF (the
// only network step of verification; `ai-rulez trust update`) and caches it for
// offline use. It returns the cached file's path.
func UpdateTrustedRoot(env ambient.Env) (string, error) {
	dir, err := config.CacheDirIn(env, "sigstore")
	if err != nil {
		return "", oops.Wrap(err)
	}
	opts := tuf.DefaultOptions()
	opts.CachePath = filepath.Join(dir, "tuf")
	tr, err := root.FetchTrustedRootWithOptions(opts)
	if err != nil {
		return "", oops.Wrapf(err, "fetch the Sigstore trusted root")
	}
	data, err := tr.MarshalJSON()
	if err != nil {
		return "", oops.Wrapf(err, "encode the trusted root")
	}
	path := filepath.Join(dir, TrustedRootFile)
	if err := safefs.WriteFileAtomic(path, data); err != nil {
		return "", oops.Wrap(err)
	}
	return path, nil
}
