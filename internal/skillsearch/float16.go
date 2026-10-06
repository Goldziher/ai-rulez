package skillsearch

import "math"

// float32ToFloat16 converts to IEEE 754 half precision, rounding to nearest even.
// Values beyond the half range become infinities, which the index rejects at
// write time; NaN stays NaN.
func float32ToFloat16(f float32) uint16 {
	b := math.Float32bits(f)
	sign := uint16(b>>16) & 0x8000
	exp := int32(b>>23) & 0xff
	mant := b & 0x7fffff
	switch {
	case exp == 0xff: // inf or NaN
		if mant != 0 {
			return sign | 0x7e00
		}
		return sign | 0x7c00
	case exp > 142: // overflow
		return sign | 0x7c00
	case exp < 103: // underflow to zero
		return sign
	case exp < 113: // subnormal half
		shift := uint32(126 - exp)
		m := mant | 0x800000
		half := uint16(m >> shift)
		rem := m & (1<<shift - 1)
		mid := uint32(1) << (shift - 1)
		if rem > mid || (rem == mid && half&1 == 1) {
			half++
		}
		return sign | half
	}
	half := sign | uint16((exp-112)<<10) | uint16(mant>>13)
	rem := mant & 0x1fff
	if rem > 0x1000 || (rem == 0x1000 && half&1 == 1) {
		half++ // a carry into the exponent is correct
	}
	return half
}

func float16ToFloat32(h uint16) float32 {
	sign := uint32(h&0x8000) << 16
	exp := uint32(h>>10) & 0x1f
	mant := uint32(h & 0x3ff)
	switch exp {
	case 0:
		if mant == 0 {
			return math.Float32frombits(sign)
		}
		// subnormal: normalise
		e := uint32(113)
		for mant&0x400 == 0 {
			mant <<= 1
			e--
		}
		mant &= 0x3ff
		return math.Float32frombits(sign | e<<23 | mant<<13)
	case 0x1f:
		return math.Float32frombits(sign | 0x7f800000 | mant<<13)
	}
	return math.Float32frombits(sign | (exp+112)<<23 | mant<<13)
}
