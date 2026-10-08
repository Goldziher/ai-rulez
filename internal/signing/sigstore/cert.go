package sigstore

import (
	"crypto/x509"
	"encoding/hex"

	"github.com/sigstore/sigstore-go/pkg/fulcio/certificate"

	"github.com/Goldziher/ai-rulez/v5/internal/signing"
)

func hexOf(b []byte) string { return hex.EncodeToString(b) }

func summarize(cert *x509.Certificate) (signing.SignerInfo, error) {
	s, err := certificate.SummarizeCertificate(cert)
	if err != nil {
		return signing.SignerInfo{}, signing.Wrap(signing.CodeInvalid, err, "the signing certificate cannot be read")
	}
	return signing.SignerInfo{Kind: signing.KindKeyless, Identity: s.SubjectAlternativeName, Issuer: s.Issuer}, nil
}
