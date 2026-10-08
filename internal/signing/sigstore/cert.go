package signing

import (
	"crypto/x509"
	"encoding/hex"

	"github.com/sigstore/sigstore-go/pkg/fulcio/certificate"
)

func hexOf(b []byte) string { return hex.EncodeToString(b) }

func summarize(cert *x509.Certificate) (SignerInfo, error) {
	s, err := certificate.SummarizeCertificate(cert)
	if err != nil {
		return SignerInfo{}, wrap(CodeInvalid, err, "the signing certificate cannot be read")
	}
	return SignerInfo{Kind: KindKeyless, Identity: s.SubjectAlternativeName, Issuer: s.Issuer}, nil
}
