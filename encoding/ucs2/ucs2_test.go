// SPDX-License-Identifier: MIT

package ucs2_test

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"slices"
	"testing"
	"unicode/utf16"

	"github.com/gomaja/go-sms/encoding/ucs2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type decodePattern struct {
	name string
	in   []byte
	out  []rune
	err  error
}

func TestDecode(t *testing.T) {
	patterns := []decodePattern{
		{
			"nil",
			nil,
			nil,
			nil,
		},
		{
			"empty",
			[]byte(""),
			nil,
			nil,
		},
		{
			"odd",
			[]byte{1, 2, 3, 4, 5},
			nil,
			ucs2.ErrInvalidLength,
		},
		{
			"howdy",
			[]byte{
				0x4F, 0x60, 0x59, 0x7D, 0xFF, 0x01, 0x00, 0x48, 0x00, 0x6F,
				0x00, 0x77, 0x00, 0x64, 0x00, 0x79,
			},
			[]rune("你好！Howdy"),
			nil,
		},
		{
			"grin",
			[]byte{0xd8, 0x3d, 0xde, 0x01},
			[]rune("😁"),
			nil,
		},
		{
			"dangling surrogate",
			[]byte{
				0x00, 0x48, 0x00, 0x6F, 0x00, 0x77, 0x00, 0x64, 0x00, 0x79,
				0xd8, 0x3d,
			},
			[]rune("Howdy"),
			ucs2.ErrDanglingSurrogate([]byte{0xD8, 0x3D}),
		},
		{
			"high surrogate before bmp",
			[]byte{0xd8, 0x00, 0x00, 0x41},
			[]rune{0xfffd, 'A'},
			nil,
		},
		{
			"low surrogate before bmp",
			[]byte{0xdc, 0x00, 0x00, 0x41},
			[]rune{0xfffd, 'A'},
			nil,
		},
		{
			"leading low surrogate",
			[]byte{0xde, 0x01, 0x00, 0x42},
			[]rune{0xfffd, 'B'},
			nil,
		},
		{
			"high surrogate before pair",
			[]byte{0xd8, 0x3d, 0xd8, 0x3d, 0xde, 0x01},
			[]rune{0xfffd, 0x1f601},
			nil,
		},
		{
			"trailing low surrogate",
			[]byte{0x00, 0x41, 0xde, 0x01},
			[]rune{'A', 0xfffd},
			nil,
		},
		{
			"low surrogates",
			[]byte{0xdc, 0x00, 0xdf, 0xff},
			[]rune{0xfffd, 0xfffd},
			nil,
		},
		{
			"reversed pair",
			[]byte{0xde, 0x01, 0xd8, 0x3d},
			[]rune{0xfffd},
			ucs2.ErrDanglingSurrogate([]byte{0xd8, 0x3d}),
		},
		{
			"high surrogates",
			[]byte{0xdb, 0xff, 0xd8, 0x3d},
			[]rune{0xfffd},
			ucs2.ErrDanglingSurrogate([]byte{0xd8, 0x3d}),
		},
		{
			"only high surrogate",
			[]byte{0xdb, 0xff},
			[]rune{},
			ucs2.ErrDanglingSurrogate([]byte{0xdb, 0xff}),
		},
		{
			"surrogate range edges",
			[]byte{0xd7, 0xff, 0xdb, 0xff, 0xdc, 0x00, 0xe0, 0x00},
			[]rune{0xd7ff, 0x10fc00, 0xe000},
			nil,
		},
	}
	for _, p := range patterns {
		f := func(t *testing.T) {
			dst, err := ucs2.Decode(p.in)
			assert.Equal(t, p.err, err)
			assert.Equal(t, p.out, dst)
		}
		t.Run(p.name, f)
	}
}

// TestEncode checks that Encode codes each rune in 16 bits, as the UCS2
// alphabet of 3GPP TS 23.038 Section 6.2.3 does, and returns no octets and
// an error for the first rune it cannot code: an ErrUnencodable for a
// character above U+FFFF, and an ErrInvalidRune for a rune that is not a
// character. Neither is replaced by U+FFFD or by a surrogate pair.
func TestEncode(t *testing.T) {
	patterns := []struct {
		name string
		in   []rune
		encodeResult
	}{
		{"nil", nil, encodeResult{nil, nil}},
		{"empty", []rune{}, encodeResult{nil, nil}},
		{"howdy", []rune("你好！Howdy"), encodeResult{[]byte{
			0x4F, 0x60, 0x59, 0x7D, 0xFF, 0x01, 0x00, 0x48, 0x00, 0x6F,
			0x00, 0x77, 0x00, 0x64, 0x00, 0x79,
		}, nil}},
		{"U+0000", []rune{0}, encodeResult{[]byte{0x00, 0x00}, nil}},
		{"U+D7FF", []rune{0xd7ff}, encodeResult{[]byte{0xd7, 0xff}, nil}},
		{"U+E000", []rune{0xe000}, encodeResult{[]byte{0xe0, 0x00}, nil}},
		{"U+FFFD", []rune{0xfffd}, encodeResult{[]byte{0xff, 0xfd}, nil}},
		{"U+FFFF", []rune{0xffff}, encodeResult{[]byte{0xff, 0xff}, nil}},
		{"bmp edges", []rune{0, 0xd7ff, 0xe000, 0xfffd, 0xffff}, encodeResult{
			[]byte{0x00, 0x00, 0xd7, 0xff, 0xe0, 0x00, 0xff, 0xfd, 0xff, 0xff}, nil}},
		{"U+10000", []rune{0x10000}, encodeResult{nil, ucs2.ErrUnencodable{Index: 0, Rune: 0x10000}}},
		{"U+1F600", []rune{0x1f600}, encodeResult{nil, ucs2.ErrUnencodable{Index: 0, Rune: 0x1f600}}},
		{"U+10FFFF", []rune{0x10ffff}, encodeResult{nil, ucs2.ErrUnencodable{Index: 0, Rune: 0x10ffff}}},
		{"U+D800", []rune{0xd800}, encodeResult{nil, ucs2.ErrInvalidRune{Index: 0, Rune: 0xd800}}},
		{"U+DBFF", []rune{0xdbff}, encodeResult{nil, ucs2.ErrInvalidRune{Index: 0, Rune: 0xdbff}}},
		{"U+DC00", []rune{0xdc00}, encodeResult{nil, ucs2.ErrInvalidRune{Index: 0, Rune: 0xdc00}}},
		{"U+DFFF", []rune{0xdfff}, encodeResult{nil, ucs2.ErrInvalidRune{Index: 0, Rune: 0xdfff}}},
		{"U+110000", []rune{0x110000}, encodeResult{nil, ucs2.ErrInvalidRune{Index: 0, Rune: 0x110000}}},
		{"-1", []rune{-1}, encodeResult{nil, ucs2.ErrInvalidRune{Index: 0, Rune: -1}}},
		{"MaxInt32", []rune{math.MaxInt32}, encodeResult{nil, ucs2.ErrInvalidRune{Index: 0, Rune: math.MaxInt32}}},
		{"MinInt32", []rune{math.MinInt32}, encodeResult{nil, ucs2.ErrInvalidRune{Index: 0, Rune: math.MinInt32}}},
		{"valid prefix", []rune("A😀"), encodeResult{nil, ucs2.ErrUnencodable{Index: 1, Rune: 0x1f600}}},
		{"first of two unencodable", []rune("ab😀😁"), encodeResult{nil, ucs2.ErrUnencodable{Index: 2, Rune: 0x1f600}}},
		{"unencodable then invalid", []rune{'A', 0x1f600, 0xd800}, encodeResult{nil, ucs2.ErrUnencodable{Index: 1, Rune: 0x1f600}}},
		{"invalid then unencodable", []rune{'A', 0xd800, 0x1f600}, encodeResult{nil, ucs2.ErrInvalidRune{Index: 1, Rune: 0xd800}}},
		{"first of two invalid", []rune{'A', 'B', 0x110000, -1}, encodeResult{nil, ucs2.ErrInvalidRune{Index: 2, Rune: 0x110000}}},
		{"pair of surrogates", []rune{0xd83d, 0xde01}, encodeResult{nil, ucs2.ErrInvalidRune{Index: 0, Rune: 0xd83d}}},
	}
	for _, p := range patterns {
		t.Run(p.name, func(t *testing.T) {
			out, err := ucs2.Encode(p.in)
			assert.Equal(t, p.err, err)
			if p.out == nil {
				assert.Nil(t, out)
			} else {
				assert.Equal(t, hex.EncodeToString(p.out), hex.EncodeToString(out))
				// sized before it is written, so no larger than it needs
				assert.Equal(t, len(out), cap(out))
			}
		})
	}
}

// TestEncodeRoundTrip checks that Decode gives back the runes Encode coded,
// and that Encode codes them as EncodeUTF16 does, as UTF-16 codes a
// character up to U+FFFF in one 16-bit unit too (RFC 2781 Section 2.1).
func TestEncodeRoundTrip(t *testing.T) {
	for _, in := range [][]rune{
		[]rune("你好！Howdy"),
		[]rune("Привет, мир"),
		{0, 0xd7ff, 0xe000, 0xfffd, 0xffff},
	} {
		b, err := ucs2.Encode(in)
		require.NoError(t, err)
		out, err := ucs2.Decode(b)
		require.NoError(t, err)
		assert.Equal(t, in, out)
		u, err := ucs2.EncodeUTF16(in)
		require.NoError(t, err)
		assert.Equal(t, u, b)
	}
}

// TestErrUnencodable checks that an ErrUnencodable can be found with
// errors.As, also when wrapped, and is not an ErrInvalidRune, and that its
// message names the character and its index.
func TestErrUnencodable(t *testing.T) {
	_, err := ucs2.Encode([]rune("ab😀"))
	var ue ucs2.ErrUnencodable
	require.True(t, errors.As(fmt.Errorf("wrapped: %w", err), &ue), "%v", err)
	assert.Equal(t, 2, ue.Index)
	assert.Equal(t, rune(0x1f600), ue.Rune)
	var ir ucs2.ErrInvalidRune
	assert.False(t, errors.As(err, &ir))

	_, err = ucs2.Encode([]rune{'a', 0xdfff})
	require.True(t, errors.As(fmt.Errorf("wrapped: %w", err), &ir), "%v", err)
	assert.Equal(t, 1, ir.Index)
	assert.Equal(t, rune(0xdfff), ir.Rune)
	assert.False(t, errors.As(err, &ue))

	assert.Equal(t, "ucs2: '😀' (U+1F600) at index 2 has no UCS2 encoding, as it is above U+FFFF",
		ucs2.ErrUnencodable{Index: 2, Rune: 0x1f600}.Error())
	assert.Equal(t, "ucs2: '\\U0010ffff' (U+10FFFF) at index 0 has no UCS2 encoding, as it is above U+FFFF",
		ucs2.ErrUnencodable{Index: 0, Rune: 0x10ffff}.Error())
}

func TestErrDanglingSurrogate(t *testing.T) {
	patterns := [][]byte{
		{0xd8, 0x00},
		{0xd8, 0xa0},
		{0xd8, 0x0a},
		{0xd8, 0x9a},
		{0xd8, 0xa9},
		{0xd8, 0xff},
	}
	for _, p := range patterns {
		f := func(t *testing.T) {
			err := ucs2.ErrDanglingSurrogate(p)
			expected := fmt.Sprintf("ucs2: dangling surrogate: %#v", p)
			s := err.Error()
			assert.Equal(t, expected, s)
		}
		t.Run(fmt.Sprintf("%x", p), f)
	}
}

// ExampleEncode shows that UCS2 codes each character in 16 bits, so it has
// no encoding for a character above U+FFFF, which EncodeUTF16 codes instead.
func ExampleEncode() {
	b, err := ucs2.Encode([]rune("Aé€"))
	fmt.Printf("% x %v\n", b, err)
	b, err = ucs2.Encode([]rune("A😁"))
	fmt.Printf("%v %v\n", b, err)
	// Output:
	// 00 41 00 e9 20 ac <nil>
	// [] ucs2: '😁' (U+1F601) at index 1 has no UCS2 encoding, as it is above U+FFFF
}

// TestErrDanglingSurrogateCopy checks that the error does not share memory
// with the decoded array. sms.Decode prepends the error to the next segment
// with append, which would otherwise write over whatever follows the array.
func TestErrDanglingSurrogateCopy(t *testing.T) {
	buf := []byte{0x00, 0x41, 0xd8, 0x3d, 0xaa, 0xbb}
	_, err := ucs2.Decode(buf[:4])
	ds, ok := err.(ucs2.ErrDanglingSurrogate)
	require.True(t, ok, "unexpected error %v", err)
	ud := append([]byte(ds), 0xde, 0x01)
	assert.Equal(t, []byte{0xd8, 0x3d, 0xde, 0x01}, ud)
	assert.Equal(t, []byte{0x00, 0x41, 0xd8, 0x3d, 0xaa, 0xbb}, buf)
}

// FuzzDecode checks Decode against the standard library UTF-16 decoder, which
// replaces each unpaired surrogate with U+FFFD and keeps the unit after it.
// The one difference is a trailing high surrogate, which Decode returns as an
// ErrDanglingSurrogate so it can be joined to the next segment. Well-formed
// input must also re-encode unchanged with EncodeUTF16, and with Encode if it
// holds no surrogate pair.
func FuzzDecode(f *testing.F) {
	seeds := [][]byte{
		nil,
		{1, 2, 3, 4, 5},
		{
			0x4F, 0x60, 0x59, 0x7D, 0xFF, 0x01, 0x00, 0x48, 0x00, 0x6F,
			0x00, 0x77, 0x00, 0x64, 0x00, 0x79,
		},
		{0xd8, 0x3d, 0xde, 0x01},
		{0x00, 0x48, 0xd8, 0x3d},
		{0xd8, 0x00, 0x00, 0x41},
		{0xdc, 0x00, 0x00, 0x41},
		{0xd8, 0x3d, 0xd8, 0x3d, 0xde, 0x01},
		{0x00, 0x41, 0xde, 0x01},
		{0xde, 0x01, 0xd8, 0x3d},
	}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, src []byte) {
		dst, err := ucs2.Decode(src)
		if len(src)%2 != 0 {
			require.Equal(t, ucs2.ErrInvalidLength, err)
			require.Nil(t, dst)
			return
		}
		units := make([]uint16, len(src)/2)
		for i := range units {
			units[i] = binary.BigEndian.Uint16(src[2*i:])
		}
		n := len(units)
		if n > 0 && units[n-1] >= 0xd800 && units[n-1] < 0xdc00 {
			require.Equal(t, ucs2.ErrDanglingSurrogate(src[len(src)-2:]), err)
			units = units[:n-1]
		} else {
			require.NoError(t, err)
		}
		want := utf16.Decode(units)
		if len(want) == 0 {
			assert.Empty(t, dst)
		} else {
			assert.Equal(t, want, dst)
		}
		// Well-formed UTF-16 must survive a decode and re-encode unchanged.
		// If it has no surrogate it is UCS2, which a strict re-encode keeps
		// too, and otherwise the strict re-encode refuses the first rune that
		// a surrogate pair coded.
		if !slices.Equal(utf16.Encode(want), units) {
			return
		}
		b, err := ucs2.EncodeUTF16(dst)
		require.NoError(t, err)
		assert.Equal(t, hex.EncodeToString(src[:2*len(units)]), hex.EncodeToString(b))
		b, err = ucs2.Encode(dst)
		if i := slices.IndexFunc(dst, func(r rune) bool { return r > 0xffff }); i >= 0 {
			require.Equal(t, ucs2.ErrUnencodable{Index: i, Rune: dst[i]}, err)
			assert.Nil(t, b)
			return
		}
		require.NoError(t, err)
		assert.Equal(t, hex.EncodeToString(src[:2*len(units)]), hex.EncodeToString(b))
	})
}

// FuzzEncode checks Encode against the oracle utf16Units: it codes each rune
// in two octets, its value, Decode gives the runes back, and if a rune is not
// a character that one 16-bit unit holds it returns no octets and an error
// holding the first such rune and its index, an ErrInvalidRune for a rune
// that is not a Unicode scalar value and an ErrUnencodable for a character
// above U+FFFF.
func FuzzEncode(f *testing.F) {
	for _, s := range fuzzRuneSeeds {
		f.Add(fuzzSeed(s))
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		in := fuzzRunes(raw)
		got, err := ucs2.Encode(in)
		for i, r := range in {
			units, ok := utf16Units(r)
			switch {
			case !ok:
				require.Equal(t, ucs2.ErrInvalidRune{Index: i, Rune: r}, err)
				require.Nil(t, got)
				return
			case len(units) != 1:
				require.Equal(t, ucs2.ErrUnencodable{Index: i, Rune: r}, err)
				require.Nil(t, got)
				return
			}
		}
		require.NoError(t, err)
		if len(in) == 0 {
			require.Nil(t, got)
			return
		}
		require.Len(t, got, 2*len(in))
		for i, r := range in {
			require.Equal(t, []byte{byte(r >> 8), byte(r)}, got[2*i:2*i+2], "rune %d", i)
		}
		out, err := ucs2.Decode(got)
		require.NoError(t, err)
		require.Equal(t, in, out)
		u, err := ucs2.EncodeUTF16(in)
		require.NoError(t, err)
		require.Equal(t, u, got)
	})
}

// encodeResult is what an encoder returns for a slice of runes.
type encodeResult struct {
	out []byte
	err error
}

// TestEncodeUTF16 checks EncodeUTF16 against RFC 2781 Section 2.1, and that
// it returns an ErrInvalidRune for the first rune that is not a Unicode
// scalar value, with no octets, rather than coding U+FFFD in its place.
func TestEncodeUTF16(t *testing.T) {
	patterns := []struct {
		name string
		in   []rune
		encodeResult
	}{
		{"nil", nil, encodeResult{nil, nil}},
		{"empty", []rune{}, encodeResult{nil, nil}},
		{"howdy", []rune("你好！Howdy"), encodeResult{[]byte{
			0x4F, 0x60, 0x59, 0x7D, 0xFF, 0x01, 0x00, 0x48, 0x00, 0x6F,
			0x00, 0x77, 0x00, 0x64, 0x00, 0x79,
		}, nil}},
		{"U+0000", []rune{0}, encodeResult{[]byte{0x00, 0x00}, nil}},
		{"U+D7FF", []rune{0xd7ff}, encodeResult{[]byte{0xd7, 0xff}, nil}},
		{"U+E000", []rune{0xe000}, encodeResult{[]byte{0xe0, 0x00}, nil}},
		{"U+FFFD", []rune{0xfffd}, encodeResult{[]byte{0xff, 0xfd}, nil}},
		{"U+FFFF", []rune{0xffff}, encodeResult{[]byte{0xff, 0xff}, nil}},
		{"U+10000", []rune{0x10000}, encodeResult{[]byte{0xd8, 0x00, 0xdc, 0x00}, nil}},
		{"U+103FF", []rune{0x103ff}, encodeResult{[]byte{0xd8, 0x00, 0xdf, 0xff}, nil}},
		{"U+10400", []rune{0x10400}, encodeResult{[]byte{0xd8, 0x01, 0xdc, 0x00}, nil}},
		{"U+1F600", []rune{0x1f600}, encodeResult{[]byte{0xd8, 0x3d, 0xde, 0x00}, nil}},
		{"U+10FFFF", []rune{0x10ffff}, encodeResult{[]byte{0xdb, 0xff, 0xdf, 0xff}, nil}},
		{"grin", []rune("😁"), encodeResult{[]byte{0xd8, 0x3d, 0xde, 0x01}, nil}},
		{"mixed", []rune("A😀é"), encodeResult{[]byte{0x00, 0x41, 0xd8, 0x3d, 0xde, 0x00, 0x00, 0xe9}, nil}},
		{"plane edges", []rune{0xffff, 0x10000, 0x10ffff}, encodeResult{
			[]byte{0xff, 0xff, 0xd8, 0x00, 0xdc, 0x00, 0xdb, 0xff, 0xdf, 0xff}, nil}},
		{"U+D800", []rune{0xd800}, encodeResult{nil, ucs2.ErrInvalidRune{Index: 0, Rune: 0xd800}}},
		{"U+DBFF", []rune{0xdbff}, encodeResult{nil, ucs2.ErrInvalidRune{Index: 0, Rune: 0xdbff}}},
		{"U+DC00", []rune{0xdc00}, encodeResult{nil, ucs2.ErrInvalidRune{Index: 0, Rune: 0xdc00}}},
		{"U+DFFF", []rune{0xdfff}, encodeResult{nil, ucs2.ErrInvalidRune{Index: 0, Rune: 0xdfff}}},
		{"U+110000", []rune{0x110000}, encodeResult{nil, ucs2.ErrInvalidRune{Index: 0, Rune: 0x110000}}},
		{"-1", []rune{-1}, encodeResult{nil, ucs2.ErrInvalidRune{Index: 0, Rune: -1}}},
		{"MaxInt32", []rune{math.MaxInt32}, encodeResult{nil, ucs2.ErrInvalidRune{Index: 0, Rune: math.MaxInt32}}},
		{"MinInt32", []rune{math.MinInt32}, encodeResult{nil, ucs2.ErrInvalidRune{Index: 0, Rune: math.MinInt32}}},
		{"valid prefix", []rune{'A', 0x1f600, 0xdc00}, encodeResult{nil, ucs2.ErrInvalidRune{Index: 2, Rune: 0xdc00}}},
		{"first of two", []rune{'A', 0xd800, 0xdc00}, encodeResult{nil, ucs2.ErrInvalidRune{Index: 1, Rune: 0xd800}}},
		{"first of two out of range", []rune{'A', 'B', 0x110000, -1}, encodeResult{nil, ucs2.ErrInvalidRune{Index: 2, Rune: 0x110000}}},
		{"pair of surrogates", []rune{0xd83d, 0xde01}, encodeResult{nil, ucs2.ErrInvalidRune{Index: 0, Rune: 0xd83d}}},
	}
	for _, p := range patterns {
		t.Run(p.name, func(t *testing.T) {
			out, err := ucs2.EncodeUTF16(p.in)
			assert.Equal(t, p.err, err)
			if p.out == nil {
				assert.Nil(t, out)
			} else {
				assert.Equal(t, hex.EncodeToString(p.out), hex.EncodeToString(out))
				// sized before it is written, so no larger than it needs
				assert.Equal(t, len(out), cap(out))
			}
		})
	}
}

// TestEncodeUTF16RoundTrip checks that Decode gives back the runes
// EncodeUTF16 coded, as RFC 2781 Section 2.2 decodes what Section 2.1
// encodes.
func TestEncodeUTF16RoundTrip(t *testing.T) {
	for _, in := range [][]rune{
		[]rune("你好！Howdy"),
		[]rune("a😁b😂c"),
		{0, 0xd7ff, 0xe000, 0xfffd, 0xffff, 0x10000, 0x10ffff},
	} {
		b, err := ucs2.EncodeUTF16(in)
		require.NoError(t, err)
		out, err := ucs2.Decode(b)
		require.NoError(t, err)
		assert.Equal(t, in, out)
	}
}

// TestErrInvalidRune checks that an ErrInvalidRune can be found with
// errors.As, also when wrapped, and that its message names the rune and its
// index, giving a negative value as it is rather than as a code point.
func TestErrInvalidRune(t *testing.T) {
	_, err := ucs2.EncodeUTF16([]rune{'a', 'b', 0xdc00})
	var ir ucs2.ErrInvalidRune
	require.True(t, errors.As(fmt.Errorf("wrapped: %w", err), &ir), "%v", err)
	assert.Equal(t, 2, ir.Index)
	assert.Equal(t, rune(0xdc00), ir.Rune)

	for _, p := range []struct {
		err ucs2.ErrInvalidRune
		msg string
	}{
		{ucs2.ErrInvalidRune{Index: 2, Rune: 0xdc00}, "ucs2: rune U+DC00 at index 2 is not a Unicode scalar value"},
		{ucs2.ErrInvalidRune{Index: 0, Rune: 0x110000}, "ucs2: rune U+110000 at index 0 is not a Unicode scalar value"},
		{ucs2.ErrInvalidRune{Index: 7, Rune: -1}, "ucs2: rune -1 at index 7 is not a Unicode scalar value"},
		{ucs2.ErrInvalidRune{Index: 1, Rune: math.MinInt32}, "ucs2: rune -2147483648 at index 1 is not a Unicode scalar value"},
	} {
		assert.Equal(t, p.msg, p.err.Error())
	}
}

// ExampleEncodeUTF16 shows a character above U+FFFF coded as a surrogate
// pair, and a surrogate code point, which is not a character, refused.
func ExampleEncodeUTF16() {
	b, err := ucs2.EncodeUTF16([]rune("A😁"))
	fmt.Printf("% x %v\n", b, err)
	b, err = ucs2.EncodeUTF16([]rune{'A', 0xd83d})
	fmt.Printf("%v %v\n", b, err)
	// Output:
	// 00 41 d8 3d de 01 <nil>
	// [] ucs2: rune U+D83D at index 1 is not a Unicode scalar value
}

// fuzzRunes reads fuzz input as runes, five bytes each: a selector and a
// big-endian 32-bit value. The selector folds the value into a range, so that
// runes of the BMP, of the supplementary planes and around each boundary of
// the Unicode scalar values are as frequent as arbitrary 32-bit values, most
// of which are above U+10FFFF or negative.
func fuzzRunes(raw []byte) []rune {
	edges := []rune{0, 0xd800, 0xdc00, 0xe000, 0x10000, 0x110000}
	in := make([]rune, 0, len(raw)/5)
	for ; len(raw) >= 5; raw = raw[5:] {
		v := binary.BigEndian.Uint32(raw[1:])
		var r rune
		switch raw[0] % 4 {
		case 0: // the BMP, surrogates included
			r = rune(v % 0x10000)
		case 1: // the supplementary planes
			r = rune(0x10000 + v%0x100000)
		case 2: // within 128 either side of a boundary
			r = edges[v%uint32(len(edges))] + rune(int8(v>>8))
		default: // any 32-bit value
			r = rune(int32(v))
		}
		in = append(in, r)
	}
	return in
}

// fuzzSeed returns the fuzz input that fuzzRunes reads as the runes.
func fuzzSeed(rs []rune) []byte {
	raw := make([]byte, 0, 5*len(rs))
	for _, r := range rs {
		raw = append(raw, 3)
		raw = binary.BigEndian.AppendUint32(raw, uint32(r))
	}
	return raw
}

// utf16Units is the oracle of the fuzz tests: the UTF-16 code units of r,
// worked out from the rules of RFC 2781 Section 2 with arithmetic rather than
// the bit operations of the package, or false if r is not a Unicode scalar
// value.
func utf16Units(r rune) ([]uint16, bool) {
	switch {
	case r < 0:
		return nil, false
	case r <= 0xd7ff:
		return []uint16{uint16(r)}, true
	case r <= 0xdfff: // the surrogates have no characters
		return nil, false
	case r <= 0xffff:
		return []uint16{uint16(r)}, true
	case r <= 0x10ffff:
		u := int64(r) - 0x10000
		return []uint16{uint16(0xd800 + u/0x400), uint16(0xdc00 + u%0x400)}, true
	default:
		return nil, false
	}
}

// fuzzRuneSeeds are the runes of the seeds of the encoder fuzz tests.
var fuzzRuneSeeds = [][]rune{
	nil,
	[]rune("Howdy"),
	[]rune("你好！Howdy"),
	[]rune("a😁b"),
	{0, 0xd7ff, 0xe000, 0xfffd, 0xffff, 0x10000, 0x10ffff},
	{'a', 0xd800, 0xdfff},
	{'a', 0x1f600, 0x110000, -1},
	{math.MaxInt32, math.MinInt32},
}

// FuzzEncodeUTF16 checks EncodeUTF16 against the oracle utf16Units and the
// standard library: it codes the runes as RFC 2781 Section 2.1 does, two
// octets for a rune up to U+FFFF and four above it, Decode gives them back,
// and if a rune is not a Unicode scalar value it returns no octets and an
// ErrInvalidRune holding the first such rune and its index.
func FuzzEncodeUTF16(f *testing.F) {
	for _, s := range fuzzRuneSeeds {
		f.Add(fuzzSeed(s))
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		in := fuzzRunes(raw)
		got, err := ucs2.EncodeUTF16(in)
		var want []byte
		size := 0
		for i, r := range in {
			units, ok := utf16Units(r)
			if !ok {
				require.Equal(t, ucs2.ErrInvalidRune{Index: i, Rune: r}, err)
				require.Nil(t, got)
				return
			}
			for _, u := range units {
				want = binary.BigEndian.AppendUint16(want, u)
			}
			if r > 0xffff {
				size += 4
			} else {
				size += 2
			}
		}
		require.NoError(t, err)
		require.Equal(t, want, got)
		require.Len(t, got, size)
		require.Equal(t, len(got), cap(got))
		if len(in) == 0 {
			require.Nil(t, got)
			return
		}
		std := utf16.Encode(in)
		require.Len(t, got, 2*len(std))
		for i, u := range std {
			require.Equal(t, u, binary.BigEndian.Uint16(got[2*i:]), "unit %d", i)
		}
		out, err := ucs2.Decode(got)
		require.NoError(t, err)
		require.Equal(t, in, out)
	})
}
