package skillsearch

import (
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testIndex(t *testing.T, dtype string) *Index {
	t.Helper()
	m := Manifest{DocTemplateVersion: DocTemplateVersion, Provider: "test@local", Model: "m", Dims: 3, DType: dtype, Fields: []string{"name"}}
	items := []ManifestItem{{ID: "b", ItemDigest: "sha256:b", TextDigest: "sha256:tb"}, {ID: "a", Domain: "d", ItemDigest: "sha256:a", TextDigest: "sha256:ta"}}
	x, err := NewIndex(m, items, [][]float32{{1, 2, 3}, {0.5, -1, 0.25}})
	require.NoError(t, err)
	return x
}

func TestFloat16_RoundTrip(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in   float32
		want float32
		tol  float64
	}{
		{0, 0, 0}, {1, 1, 0}, {-2.5, -2.5, 0}, {0.333333, 0.333333, 3e-4}, {65504, 65504, 0},
		{1e-7, 1e-7, 1e-7}, {6.1e-5, 6.1e-5, 1e-7},
	}
	for _, tt := range tests {
		got := float16ToFloat32(float32ToFloat16(tt.in))
		assert.InDelta(t, tt.want, got, tt.tol+1e-9, "%v", tt.in)
	}
	assert.True(t, math.IsInf(float64(float16ToFloat32(float32ToFloat16(1e9))), 1), "overflow is infinite, which the index rejects")
	assert.True(t, math.IsNaN(float64(float16ToFloat32(float32ToFloat16(float32(math.NaN()))))))
	// round to nearest even: 1 + 2^-11 is halfway between 1 and 1+2^-10 and rounds to 1
	assert.Equal(t, float32(1), float16ToFloat32(float32ToFloat16(1+1.0/2048)))
}

func TestIndex_WriteLoadRoundTrip(t *testing.T) {
	t.Parallel()
	for _, dtype := range []string{DTypeFloat32, DTypeFloat16} {
		t.Run(dtype, func(t *testing.T) {
			// Arrange
			x := testIndex(t, dtype)
			require.NoError(t, x.Quantize())
			dir := t.TempDir()

			// Act
			require.NoError(t, WriteIndex(dir, x))
			got, err := LoadIndex(dir)

			// Assert
			require.NoError(t, err)
			assert.Equal(t, x.Manifest, got.Manifest)
			assert.Equal(t, 2, got.Len())
			assert.Equal(t, 1, got.Row("d", "a"), "rows are sorted by domain then id")
			assert.Equal(t, 0, got.Row("", "b"))
			assert.Equal(t, -1, got.Row("", "missing"))
			for i := range x.vecs {
				assert.Equal(t, x.vecs[i], got.vecs[i], "the loaded vectors are bit-identical to the quantized ones")
			}
		})
	}
}

func TestIndex_BytesAreDeterministic(t *testing.T) {
	t.Parallel()
	a, b := testIndex(t, DTypeFloat16), testIndex(t, DTypeFloat16)
	ma, va, err := a.Bytes()
	require.NoError(t, err)
	mb, vb, err := b.Bytes()
	require.NoError(t, err)
	assert.Equal(t, ma, mb)
	assert.Equal(t, va, vb)
	assert.Len(t, va, 2*3*2, "float16 stores two bytes per component")
}

func TestIndex_Float16IsHalfTheSize(t *testing.T) {
	t.Parallel()
	_, v32, err := testIndex(t, DTypeFloat32).Bytes()
	require.NoError(t, err)
	_, v16, err := testIndex(t, DTypeFloat16).Bytes()
	require.NoError(t, err)
	assert.Equal(t, len(v32)/2, len(v16))
}

func TestLoadIndex_RejectsDamage(t *testing.T) {
	t.Parallel()
	write := func(t *testing.T) string {
		dir := t.TempDir()
		require.NoError(t, WriteIndex(dir, testIndex(t, DTypeFloat32)))
		return dir
	}
	tests := []struct {
		name   string
		damage func(t *testing.T, dir string)
	}{
		{"vectors truncated", func(t *testing.T, dir string) {
			p := filepath.Join(dir, VectorsFile)
			raw, _ := os.ReadFile(p)
			require.NoError(t, os.WriteFile(p, raw[:len(raw)-4], 0o600))
		}},
		{"vectors flipped bit", func(t *testing.T, dir string) {
			p := filepath.Join(dir, VectorsFile)
			raw, _ := os.ReadFile(p)
			raw[0] ^= 1
			require.NoError(t, os.WriteFile(p, raw, 0o600))
		}},
		{"vectors missing", func(t *testing.T, dir string) { require.NoError(t, os.Remove(filepath.Join(dir, VectorsFile))) }},
		{"manifest not json", func(t *testing.T, dir string) {
			require.NoError(t, os.WriteFile(filepath.Join(dir, ManifestFile), []byte("{"), 0o600))
		}},
		{"manifest wrong version", func(t *testing.T, dir string) {
			p := filepath.Join(dir, ManifestFile)
			raw, _ := os.ReadFile(p)
			require.NoError(t, os.WriteFile(p, []byte(string(raw[:15])+"9"+string(raw[16:])), 0o600))
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := write(t)
			tt.damage(t, dir)

			_, err := LoadIndex(dir)

			require.ErrorIs(t, err, ErrNoIndex)
		})
	}
	_, err := LoadIndex(filepath.Join(t.TempDir(), "absent"))
	require.ErrorIs(t, err, ErrNoIndex)
}

func TestLoadIndex_RejectsNaNInVectors(t *testing.T) {
	t.Parallel()
	x := testIndex(t, DTypeFloat32)
	x.vecs[0][0] = float32(math.NaN())
	dir := t.TempDir()
	require.NoError(t, WriteIndex(dir, x), "the writer does not look; the loader does")

	_, err := LoadIndex(dir)

	require.ErrorIs(t, err, ErrNoIndex)
}

func TestNewIndex_RejectsBadVectors(t *testing.T) {
	t.Parallel()
	m := Manifest{Dims: 2, DType: DTypeFloat32}
	items := []ManifestItem{{ID: "a"}}
	for name, v := range map[string][]float32{"ragged": {1}, "nan": {1, float32(math.NaN())}, "inf": {1, float32(math.Inf(1))}} {
		_, err := NewIndex(m, items, [][]float32{v})
		assert.Error(t, err, name)
	}
	_, err := NewIndex(m, []ManifestItem{{ID: "a"}, {ID: "a"}}, [][]float32{{1, 0}, {0, 1}})
	assert.Error(t, err, "duplicate ids")
}

func TestIndex_Float16RejectsOverflow(t *testing.T) {
	t.Parallel()
	x := testIndex(t, DTypeFloat16)
	x.vecs[0][0] = 1e9
	_, _, err := x.Bytes()
	assert.Error(t, err)
}

func TestIndex_CheckClassifiesItems(t *testing.T) {
	t.Parallel()
	// Arrange: an index of three skills, then one edited, one added, one removed
	items := refundCatalog()
	cfg := Config{}
	emb := refundEmbedder()
	res, err := Build(t.Context(), items, &BuildOptions{Config: cfg, Embedder: emb})
	require.NoError(t, err)
	now := append([]Item{}, items...)
	now[0].Doc.Description = "Something else entirely"                        // text changed: stale
	now[1].Digest = "sha256:new"                                              // digest changed, text same: drifted
	now = append(now[:3], Item{ID: "new-skill", Doc: Doc{Name: "new-skill"}}) // dispute-charge kept, git-workflow removed

	// Act
	st := res.Index.Check(now, cfg)

	// Assert
	assert.Equal(t, []string{"issue-refund"}, st.Stale)
	assert.Equal(t, []string{"new-skill"}, st.Missing)
	assert.Equal(t, []string{"git-workflow"}, st.Orphaned)
	assert.Equal(t, 2, st.Current)
	assert.Equal(t, 1, st.Drifted)
	assert.False(t, st.Fresh())
}

func TestManifest_Reason(t *testing.T) {
	t.Parallel()
	x := testIndex(t, DTypeFloat32)
	x.Manifest.Fields = Config{}.Resolved().Fields
	tests := []struct {
		name            string
		provider, model string
		cfg             Config
		want            string
	}{
		{"same", "test@local", "m", Config{}, ""},
		{"other model", "test@local", "n", Config{}, "model changed"},
		{"other provider", "x@y", "m", Config{}, "provider changed"},
		{"other fields", "test@local", "m", Config{Fields: []string{"name"}}, "fields changed"},
		{"body toggled", "test@local", "m", Config{IndexBody: true}, "index_body"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := x.Manifest.Reason(tt.provider, tt.model, tt.cfg)
			if tt.want == "" {
				assert.Empty(t, got)
			} else {
				assert.Contains(t, got, tt.want)
			}
		})
	}
	x.Manifest.DocTemplateVersion = 99
	assert.Contains(t, x.Manifest.Reason("test@local", "m", Config{}), "template")
}

// Known IEEE 754 binary16 encodings, including every rounding boundary of the subnormal range.
func TestFloat32ToFloat16_MatchesKnownHalfValues(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   float32
		want uint16
	}{
		{"zero", 0, 0x0000},
		{"negative zero", float32(math.Copysign(0, -1)), 0x8000},
		{"one", 1, 0x3c00},
		{"minus two", -2, 0xc000},
		{"one third rounds to nearest", 1.0 / 3, 0x3555},
		{"largest finite half", 65504, 0x7bff},
		{"just below the overflow tie", 65519.99, 0x7bff},
		{"overflow tie rounds to even, which is infinity", 65520, 0x7c00},
		{"smallest normal", 6.103515625e-05, 0x0400},
		{"largest subnormal", 6.097555160522461e-05, 0x03ff},
		{"smallest subnormal", 5.960464477539063e-08, 0x0001},
		{"2^-25 is a tie and rounds to even zero", 2.9802322387695312e-08, 0x0000},
		{"just above 2^-25 rounds up to the smallest subnormal", math.Nextafter32(2.9802322387695312e-08, 1), 0x0001},
		{"just below 2^-25 underflows to zero", math.Nextafter32(2.9802322387695312e-08, 0), 0x0000},
		{"1.5 * 2^-25", 4.470348358154297e-08, 0x0001},
		{"negative smallest subnormal", -5.960464477539063e-08, 0x8001},
		{"far below the half range", 1e-30, 0x0000},
		{"infinity", float32(math.Inf(1)), 0x7c00},
		{"negative infinity", float32(math.Inf(-1)), 0xfc00},
		{"nan", float32(math.NaN()), 0x7e00},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			got := float32ToFloat16(tt.in)

			// Assert
			assert.Equal(t, tt.want, got, "float32ToFloat16(%v) = %#04x, want %#04x", tt.in, got, tt.want)
		})
	}
}

// Every finite half decodes and encodes back to itself.
func TestFloat16_RoundTripsEveryFiniteHalf(t *testing.T) {
	t.Parallel()
	for h := uint32(0); h <= 0xffff; h++ {
		if h&0x7c00 == 0x7c00 { // inf and NaN
			continue
		}
		f := float16ToFloat32(uint16(h))
		if got := float32ToFloat16(f); got != uint16(h) {
			t.Fatalf("half %#04x decodes to %v and encodes back to %#04x", h, f, got)
		}
	}
}
