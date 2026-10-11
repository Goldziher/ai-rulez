package plugin

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func marshalProvenance(t *testing.T, doc provenanceDocument) []byte {
	t.Helper()
	data, err := json.Marshal(doc)
	require.NoError(t, err)
	return data
}

func TestRetainObsolete(t *testing.T) {
	tests := []struct {
		name     string
		previous map[string]provenanceOutput
		kept     []Obsolete
		planned  map[string]provenanceOutput
		want     map[string]provenanceOutput
	}{
		{
			name:     "carries a previous record forward",
			previous: map[string]provenanceOutput{"skills/beta/SKILL.md": {ContentHash: "blake3:old"}},
			planned:  map[string]provenanceOutput{"skills/alpha/SKILL.md": {ContentHash: "blake3:alpha"}},
			want:     map[string]provenanceOutput{"skills/beta/SKILL.md": {ContentHash: "blake3:old"}},
		},
		{
			name:    "adds a file kept this run",
			kept:    []Obsolete{{Rel: "skills/beta/SKILL.md", ContentHash: "blake3:old"}},
			planned: map[string]provenanceOutput{"skills/alpha/SKILL.md": {ContentHash: "blake3:alpha"}},
			want:    map[string]provenanceOutput{"skills/beta/SKILL.md": {ContentHash: "blake3:old"}},
		},
		{
			name:     "drops a record the plan contains again",
			previous: map[string]provenanceOutput{"skills/beta/SKILL.md": {ContentHash: "blake3:old"}},
			planned:  map[string]provenanceOutput{"skills/beta/SKILL.md": {ContentHash: "blake3:beta"}},
			want:     nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			planned := marshalProvenance(t, provenanceDocument{
				SchemaVersion: provenanceSchema,
				SourceHash:    "blake3:source",
				Outputs:       tt.planned,
			})
			var previous []byte
			if tt.previous != nil {
				previous = marshalProvenance(t, provenanceDocument{
					SchemaVersion: provenanceSchema,
					SourceHash:    "blake3:source",
					Obsolete:      tt.previous,
				})
			}

			// Act
			got, err := RetainObsolete(planned, previous, tt.kept)

			// Assert
			require.NoError(t, err)
			if tt.want == nil {
				assert.Equal(t, planned, got, "an unchanged plan is returned byte for byte")
				return
			}
			var document provenanceDocument
			require.NoError(t, json.Unmarshal(got, &document))
			assert.Equal(t, tt.want, document.Obsolete)
		})
	}
}

func TestObsoleteFiles_KeepsRetainedRecordedFiles(t *testing.T) {
	// Arrange
	bundle := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(bundle, "a.md"), []byte("hand edited\n"), 0o600))
	previous := marshalProvenance(t, provenanceDocument{
		SchemaVersion: provenanceSchema,
		Obsolete:      map[string]provenanceOutput{"a.md": {ContentHash: hashBytes([]byte("original"))}},
	})
	planned := marshalProvenance(t, provenanceDocument{
		SchemaVersion: provenanceSchema,
		Outputs:       map[string]provenanceOutput{},
	})

	// Act
	got, err := ObsoleteFiles(bundle, previous, planned)

	// Assert
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.True(t, got[0].Edited)
	assert.Equal(t, WhyEdited, got[0].Why)
	assert.Equal(t, hashBytes([]byte("original")), got[0].ContentHash)
}

func TestObsoleteFiles_DropsRetainedRecordWhenFileIsGone(t *testing.T) {
	// Arrange
	bundle := t.TempDir()
	previous := marshalProvenance(t, provenanceDocument{
		SchemaVersion: provenanceSchema,
		Obsolete:      map[string]provenanceOutput{"a.md": {ContentHash: hashBytes([]byte("original"))}},
	})
	planned := marshalProvenance(t, provenanceDocument{
		SchemaVersion: provenanceSchema,
		Outputs:       map[string]provenanceOutput{},
	})

	// Act
	got, err := ObsoleteFiles(bundle, previous, planned)

	// Assert
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestObsoleteFiles_ForgetsRetainedRecordOncePlannedAgain(t *testing.T) {
	// Arrange
	bundle := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(bundle, "a.md"), []byte("hand edited\n"), 0o600))
	previous := marshalProvenance(t, provenanceDocument{
		SchemaVersion: provenanceSchema,
		Obsolete:      map[string]provenanceOutput{"a.md": {ContentHash: hashBytes([]byte("original"))}},
	})
	planned := marshalProvenance(t, provenanceDocument{
		SchemaVersion: provenanceSchema,
		Outputs:       map[string]provenanceOutput{"a.md": {ContentHash: hashBytes([]byte("reconciled"))}},
	})

	// Act
	got, err := ObsoleteFiles(bundle, previous, planned)

	// Assert
	require.NoError(t, err)
	assert.Empty(t, got, "a reconciled path is planned again, not obsolete")
}
