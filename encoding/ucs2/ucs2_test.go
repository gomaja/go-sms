// SPDX-License-Identifier: MIT

package ucs2_test

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
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

type encodePattern struct {
	name string
	in   []rune
	out  []byte
}

func TestEncode(t *testing.T) {
	patterns := []encodePattern{
		{
			"nil",
			nil,
			nil,
		},
		{
			"empty",
			[]rune(""),
			nil,
		},
		{
			"howdy",
			[]rune("你好！Howdy"),
			[]byte{
				0x4F, 0x60, 0x59, 0x7D, 0xFF, 0x01, 0x00, 0x48, 0x00, 0x6F,
				0x00, 0x77, 0x00, 0x64, 0x00, 0x79,
			},
		},
		{
			"grin",
			[]rune("😁"),
			[]byte{0xd8, 0x3d, 0xde, 0x01},
		},
		{
			"plane edges",
			[]rune{0xffff, 0x10000, 0x10ffff},
			[]byte{0xff, 0xff, 0xd8, 0x00, 0xdc, 0x00, 0xdb, 0xff, 0xdf, 0xff},
		},
		{
			"invalid runes",
			[]rune{0xd800, 0xdfff, 0x110000, -1},
			[]byte{0xff, 0xfd, 0xff, 0xfd, 0xff, 0xfd, 0xff, 0xfd},
		},
	}
	for _, p := range patterns {
		f := func(t *testing.T) {
			dst := ucs2.Encode([]rune(p.in))
			assert.Equal(t, p.out, dst)
		}
		t.Run(p.name, f)
	}
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

// ExampleEncode shows that a code point above U+FFFF is encoded as a UTF-16
// surrogate pair, taking four bytes.
func ExampleEncode() {
	fmt.Printf("% x\n", ucs2.Encode([]rune("A😁")))
	// Output: 00 41 d8 3d de 01
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
// ErrDanglingSurrogate so it can be joined to the next segment.
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
		if slices.Equal(utf16.Encode(want), units) {
			assert.Equal(t, hex.EncodeToString(src[:2*len(units)]), hex.EncodeToString(ucs2.Encode(dst)))
		}
	})
}

// FuzzEncode checks that Encode output decodes back to the runes encoded,
// with each rune UTF-16 cannot represent replaced by U+FFFD.
//
// The fuzz input is read as big-endian 32-bit runes so that surrogate code
// points, negative values and values above U+10FFFF are covered too.
func FuzzEncode(f *testing.F) {
	for _, s := range [][]rune{
		nil,
		[]rune("Howdy"),
		[]rune("你好！Howdy"),
		[]rune("a😁b"),
		{0xd800, 0xdfff, 0x10ffff, 0x110000, -1},
	} {
		raw := make([]byte, 4*len(s))
		for i, r := range s {
			binary.BigEndian.PutUint32(raw[4*i:], uint32(r))
		}
		f.Add(raw)
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		in := make([]rune, len(raw)/4)
		want := make([]rune, len(in))
		units := 0
		for i := range in {
			r := rune(binary.BigEndian.Uint32(raw[4*i:]))
			in[i] = r
			switch {
			case r < 0, r >= 0xd800 && r < 0xe000, r > 0x10ffff:
				want[i] = 0xfffd
				units++
			case r > 0xffff:
				want[i] = r
				units += 2
			default:
				want[i] = r
				units++
			}
		}
		b := ucs2.Encode(in)
		require.Len(t, b, 2*units)
		out, err := ucs2.Decode(b)
		require.NoError(t, err)
		if len(in) == 0 {
			assert.Empty(t, out)
		} else {
			assert.Equal(t, want, out)
		}
	})
}
