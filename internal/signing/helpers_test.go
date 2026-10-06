package signing

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"testing"
	"time"

	protobundle "github.com/sigstore/protobuf-specs/gen/pb-go/bundle/v1"
	protocommon "github.com/sigstore/protobuf-specs/gen/pb-go/common/v1"
	protodsse "github.com/sigstore/protobuf-specs/gen/pb-go/dsse"
	protorekor "github.com/sigstore/protobuf-specs/gen/pb-go/rekor/v1"
	"github.com/sigstore/sigstore-go/pkg/root"
	"github.com/sigstore/sigstore-go/pkg/testing/ca"
	"github.com/sigstore/sigstore-go/pkg/tlog"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/Goldziher/ai-rulez/v5/internal/contentlock"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
)

// testLock returns a lock whose tree matches its entries.
func testLock(t *testing.T, mutate func(*lockfile.File)) *lockfile.File {
	t.Helper()
	f := &lockfile.File{
		Version: lockfile.Version, OutputsPinned: true,
		Item: []lockfile.Item{
			{Kind: "rule", ID: "style", Path: "rules/style.md", Digest: "sha256:" + hexRepeat("ab")},
			{Kind: "skill", ID: "deploy", Domain: "backend", Path: "domains/backend/skills/deploy", Digest: "sha256:" + hexRepeat("cd")},
		},
	}
	if mutate != nil {
		mutate(f)
	}
	f.Tree = contentlock.TreeOf(f)
	return f
}

func hexRepeat(s string) string {
	out := ""
	for range 32 {
		out += s
	}
	return out
}

var testNow = time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)

func signKeyed(t *testing.T, lock *lockfile.File, meta LockMeta) (bundle, pubPEM []byte) {
	t.Helper()
	priv, pub, err := GenerateKeyPair(nil)
	require.NoError(t, err)
	signer, err := LoadKeySigner(priv, nil)
	require.NoError(t, err)
	if meta.Now.IsZero() {
		meta.Now = testNow
	}
	if meta.Version == "" {
		meta.Version = "5.0.0"
	}
	data, err := SignLock(context.Background(), signer, lock, meta)
	require.NoError(t, err)
	return data, pub
}

func keyTrust(t *testing.T, pubPEM []byte) TrustSet {
	t.Helper()
	pub, err := ParsePublicKey(pubPEM)
	require.NoError(t, err)
	return TrustSet{Entries: []TrustEntry{{Subject: SubjectLock, Key: pub}}}
}

// noCTRoot hides the virtual root's CT logs: leaf certificates the virtual CA
// issues carry no signed certificate timestamp, which production roots require.
type noCTRoot struct{ root.TrustedMaterial }

func (noCTRoot) CTLogs() map[string]*root.TransparencyLog { return nil }

// virtualBundle signs payload with an in-process Fulcio and Rekor and returns the
// bundle JSON.
func virtualBundle(t *testing.T, vs *ca.VirtualSigstore, identity, issuer string, payload []byte, at time.Time) []byte {
	t.Helper()
	entity, err := vs.AttestAtTime(identity, issuer, payload, at, true)
	require.NoError(t, err)
	sc, err := entity.SignatureContent()
	require.NoError(t, err)
	raw := sc.EnvelopeContent().RawEnvelope()
	vc, err := entity.VerificationContent()
	require.NoError(t, err)
	tlogs, err := entity.TlogEntries()
	require.NoError(t, err)
	timestamps, err := entity.Timestamps()
	require.NoError(t, err)

	sig, err := decodeB64(raw.Signatures[0].Sig)
	require.NoError(t, err)
	pb := &protobundle.Bundle{
		MediaType: "application/vnd.dev.sigstore.bundle.v0.3+json",
		Content: &protobundle.Bundle_DsseEnvelope{DsseEnvelope: &protodsse.Envelope{
			Payload: payload, PayloadType: raw.PayloadType, Signatures: []*protodsse.Signature{{Sig: sig}},
		}},
		VerificationMaterial: &protobundle.VerificationMaterial{
			Content: &protobundle.VerificationMaterial_Certificate{Certificate: &protocommon.X509Certificate{RawBytes: vc.Certificate().Raw}},
		},
	}
	for _, e := range tlogs {
		tle := proto.Clone(e.TransparencyLogEntry()).(*protorekor.TransparencyLogEntry)
		tle.KindVersion = &protorekor.KindVersion{Kind: "dsse", Version: "0.0.1"}
		set, serr := vs.RekorSignPayload(tlog.RekorPayload{
			Body: base64.StdEncoding.EncodeToString(tle.CanonicalizedBody), IntegratedTime: tle.IntegratedTime,
			LogIndex: tle.LogIndex, LogID: hex.EncodeToString(tle.LogId.KeyId),
		})
		require.NoError(t, serr)
		tle.InclusionPromise = &protorekor.InclusionPromise{SignedEntryTimestamp: set}
		pb.VerificationMaterial.TlogEntries = append(pb.VerificationMaterial.TlogEntries, tle)
	}
	_ = timestamps
	data, err := protojson.Marshal(pb)
	require.NoError(t, err)
	return data
}
