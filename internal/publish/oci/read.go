package oci

import (
	"errors"
	"io"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/safefs"
)

// readLimited reads a blob or manifest, closing rc, and refuses more than
// maxBlobBytes.
func readLimited(rc io.ReadCloser) ([]byte, error) {
	defer rc.Close() //nolint:errcheck // the content is already read or failed
	data, err := safefs.ReadLimited(rc, maxBlobBytes)
	if errors.Is(err, safefs.ErrTooLarge) {
		return nil, oops.Errorf("registry content is larger than %d bytes", maxBlobBytes)
	}
	if err != nil {
		return nil, oops.Wrapf(err, "read registry content")
	}
	return data, nil
}
