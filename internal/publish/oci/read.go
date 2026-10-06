package oci

import (
	"io"

	"github.com/samber/oops"
)

// readLimited reads a blob or manifest, closing rc, and refuses more than
// maxBlobBytes.
func readLimited(rc io.ReadCloser) ([]byte, error) {
	defer rc.Close() //nolint:errcheck // the content is already read or failed
	data, err := io.ReadAll(io.LimitReader(rc, maxBlobBytes+1))
	if err != nil {
		return nil, oops.Wrapf(err, "read registry content")
	}
	if len(data) > maxBlobBytes {
		return nil, oops.Errorf("registry content is larger than %d bytes", maxBlobBytes)
	}
	return data, nil
}
