package signing_test

import (
	. "github.com/Goldziher/ai-rulez/v5/internal/signing"
	"testing"

	protobundle "github.com/sigstore/protobuf-specs/gen/pb-go/bundle/v1"
	protocommon "github.com/sigstore/protobuf-specs/gen/pb-go/common/v1"
	protorekor "github.com/sigstore/protobuf-specs/gen/pb-go/rekor/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
)

// withLogEntry returns bundle with one transparency log entry added, as
// `ai-rulez sign --tlog` writes. The entry's proof is not checkable without a
// trusted root, which is the case under test.
func withLogEntry(t *testing.T, data []byte) []byte {
	t.Helper()
	pb := &protobundle.Bundle{}
	require.NoError(t, protojson.Unmarshal(data, pb))
	pb.MediaType = "application/vnd.dev.sigstore.bundle+json;version=0.1"
	pb.VerificationMaterial.TlogEntries = append(pb.VerificationMaterial.TlogEntries, &protorekor.TransparencyLogEntry{
		LogIndex:          1,
		LogId:             &protocommon.LogId{KeyId: []byte("log")},
		KindVersion:       &protorekor.KindVersion{Kind: "dsse", Version: "0.0.1"},
		IntegratedTime:    testNow.Unix(),
		CanonicalizedBody: []byte(`{"apiVersion":"0.0.1","kind":"dsse","spec":{"envelopeHash":{"algorithm":"sha256","value":"00"},"payloadHash":{"algorithm":"sha256","value":"00"},"signatures":[{"signature":"AA==","verifier":"AA=="}]}}`),
		InclusionPromise:  &protorekor.InclusionPromise{SignedEntryTimestamp: []byte("x")},
	})
	out, err := protojson.Marshal(pb)
	require.NoError(t, err)
	return out
}

func TestTLogOptionalAcceptsALoggedKeyBundleItCannotCheck(t *testing.T) {
	lock := testLock(t, nil)
	data, pub := signKeyed(t, lock, LockMeta{Now: testNow})
	logged := withLogEntry(t, data)
	trust := keyTrust(t, pub)
	tests := []struct {
		name     string
		mode     TLogMode
		wantCode string
	}{
		{"optional verifies the key and ignores the unverifiable log", TLogOptional, ""},
		{"required still needs a root for the proof", TLogRequired, CodeRootUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := LockPolicy{Verifier: Verifier{Keys: trust.Keys(SubjectLock), TLog: tt.mode}, Trust: trust, Now: testNow}

			rep, err := VerifyLock(logged, lock, p)

			if tt.wantCode != "" {
				require.Error(t, err)
				assert.Equal(t, tt.wantCode, CodeOf(err))
				return
			}
			require.NoError(t, err)
			assert.True(t, rep.Result.Weak, "an unchecked log entry proves no time")
			assert.True(t, rep.Result.SignedAt.IsZero())
		})
	}
}
