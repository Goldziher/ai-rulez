package signing

import "encoding/base64"

func decodeB64(s string) ([]byte, error) { return base64.StdEncoding.DecodeString(s) }
