// Package obscure encodes embedded data that must not appear as plain text in
// the source tree or the binary. The malware patterns of the lint rules and
// their test payloads are such data: anti-malware scanners match the payload
// strings and report the binary, or a checkout, as malware.
//
// The encoding is not protection: it is a repeating-key XOR wrapped in base64,
// enough that no payload string survives verbatim. Edit an encoded file with
// `go run ./scripts/obscure decode FILE`, then `encode`.
package obscure

import (
	"bytes"
	"encoding/base64"

	"github.com/samber/oops"
)

const (
	key       = "ai-rulez/obscure"
	lineWidth = 76
)

// Encode returns plain XORed with the key, in base64 wrapped at 76 columns.
func Encode(plain []byte) []byte {
	enc := base64.StdEncoding.EncodeToString(xor(plain))
	var out bytes.Buffer
	for len(enc) > lineWidth {
		out.WriteString(enc[:lineWidth])
		out.WriteByte('\n')
		enc = enc[lineWidth:]
	}
	out.WriteString(enc)
	out.WriteByte('\n')
	return out.Bytes()
}

// Decode reverses Encode; whitespace between the base64 lines is ignored.
func Decode(enc []byte) ([]byte, error) {
	compact := bytes.Join(bytes.Fields(enc), nil)
	raw := make([]byte, base64.StdEncoding.DecodedLen(len(compact)))
	n, err := base64.StdEncoding.Decode(raw, compact)
	if err != nil {
		return nil, oops.Wrapf(err, "decode obscured data")
	}
	return xor(raw[:n]), nil
}

func xor(in []byte) []byte {
	out := make([]byte, len(in))
	for i, b := range in {
		out[i] = b ^ key[i%len(key)]
	}
	return out
}
