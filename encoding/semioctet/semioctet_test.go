// SPDX-License-Identifier: MIT

package semioctet_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/gomaja/go-sms/encoding/semioctet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecode(t *testing.T) {
	patterns := []struct {
		name         string
		inDst        []byte
		inSrc        []byte
		out          []byte
		outReadCount int
		err          error
	}{
		{
			"nil",
			nil,
			nil,
			nil,
			0,
			nil,
		},
		{
			"nil dst",
			nil,
			[]byte{0x10, 0x32, 0x54, 0x76},
			nil,
			0,
			nil,
		},
		{
			"nil src",
			make([]byte, 3),
			nil,
			[]byte{},
			0,
			nil,
		},
		{
			"empty dst",
			[]byte{},
			[]byte{0x10, 0x32, 0x54, 0x76},
			[]byte{},
			0,
			nil,
		},
		{
			"empty src",
			make([]byte, 3),
			[]byte{},
			[]byte{},
			0,
			nil,
		},
		{
			"limit src",
			make([]byte, 8),
			[]byte{0x10, 0x32, 0x54, 0x76},
			[]byte("01234567"),
			4,
			nil,
		},
		{
			"fill limit src",
			make([]byte, 8),
			[]byte{0x10, 0x32, 0x54, 0xf6},
			[]byte("0123456"),
			4,
			nil,
		},
		{
			"fill limit even dst",
			make([]byte, 6),
			[]byte{0x10, 0x32, 0x54, 0xf6, 0x98},
			[]byte("012345"),
			3,
			nil,
		},
		{
			"no fill limit even dst",
			make([]byte, 6),
			[]byte{0x10, 0x32, 0x54, 0x76, 0x98},
			[]byte("012345"),
			3,
			nil,
		},
		{
			"fill limit odd dst",
			make([]byte, 5),
			[]byte{0x10, 0x32, 0xf4, 0x76},
			[]byte("01234"),
			3,
			nil,
		},
		{
			"alphabet",
			make([]byte, 15),
			[]byte{0x10, 0x32, 0x54, 0x76, 0x98, 0xba, 0xdc, 0xfe},
			[]byte("0123456789*#abc"),
			8,
			nil,
		},
		{
			"no fill limit even dst",
			make([]byte, 6),
			[]byte{0x10, 0x32, 0x54, 0x76, 0x98},
			[]byte("012345"),
			3,
			nil,
		},
		{
			"no fill limit odd dst",
			make([]byte, 5),
			[]byte{0x10, 0x32, 0x54, 0x76},
			nil,
			3,
			semioctet.ErrMissingFill,
		},
		{
			"skip inter fill",
			make([]byte, 10),
			[]byte{0x10, 0x32, 0xF4, 0x76, 0xF8},
			[]byte("01234678"),
			5,
			nil,
		},
	}
	for _, p := range patterns {
		f := func(t *testing.T) {
			dst, count, err := semioctet.Decode(p.inDst, p.inSrc)
			assert.Equal(t, p.err, err)
			assert.Equal(t, p.outReadCount, count)
			assert.Equal(t, p.out, dst)
		}
		t.Run(p.name, f)
	}
}

func TestEncode(t *testing.T) {
	patterns := []struct {
		name string
		in   []byte
		out  []byte
		err  error
	}{
		{
			"nil",
			nil,
			nil,
			nil,
		},
		{
			"empty src",
			[]byte{},
			[]byte{},
			nil,
		},
		{
			"even src",
			[]byte("01234567"),
			[]byte{0x10, 0x32, 0x54, 0x76},
			nil,
		},
		{
			"odd src",
			[]byte("0123456"),
			[]byte{0x10, 0x32, 0x54, 0xf6},
			nil,
		},
		{
			"alphabet",
			[]byte("0123456789*#abc"),
			[]byte{0x10, 0x32, 0x54, 0x76, 0x98, 0xba, 0xdc, 0xfe},
			nil,
		},
		{
			"invalid digit",
			[]byte("012345D6789"),
			nil,
			semioctet.ErrInvalidDigit('D'),
		},
	}
	for _, p := range patterns {
		f := func(t *testing.T) {
			dst, err := semioctet.Encode(p.in)
			assert.Equal(t, p.err, err)
			assert.Equal(t, p.out, dst)
		}
		t.Run(p.name, f)
	}
}

// FuzzDecode checks Decode against a reference model of 3GPP TS 23.040
// Section 9.1.2.3: semi-octets are read low nibble first, each '1111' is
// skipped, and reading stops after the octet that fills dst. If that octet
// fills dst with its low nibble then its high nibble must be fill.
func FuzzDecode(f *testing.F) {
	f.Add([]byte{}, uint8(3))
	f.Add([]byte{0x10, 0x32, 0x54, 0x76}, uint8(8))
	f.Add([]byte{0x10, 0x32, 0x54, 0xf6}, uint8(8))
	f.Add([]byte{0x10, 0x32, 0x54, 0xf6, 0x98}, uint8(6))
	f.Add([]byte{0x10, 0x32, 0x54, 0x76}, uint8(5))
	f.Add([]byte{0x10, 0x32, 0xF4, 0x76, 0xF8}, uint8(10))
	f.Add([]byte{0x10, 0x32, 0x54, 0x76, 0x98, 0xba, 0xdc, 0xfe}, uint8(15))
	f.Add([]byte{0x1f, 0x32, 0xff, 0x54}, uint8(4))
	f.Add([]byte{0x1f, 0x32, 0xff, 0x54}, uint8(5))
	f.Fuzz(func(t *testing.T, src []byte, n uint8) {
		dst := make([]byte, int(n)%48)
		out, ri, err := semioctet.Decode(dst, src)

		var want []byte
		var wantErr error
		wantRi := 0
		for wantRi < len(src) && len(want) < len(dst) {
			o := src[wantRi]
			wantRi++
			for _, d := range []byte{o & 0x0f, o >> 4} {
				if d != 0x0f {
					want = append(want, "0123456789*#abc"[d])
				}
			}
			if len(want) > len(dst) {
				want = nil
				wantErr = semioctet.ErrMissingFill
				break
			}
		}
		require.Equal(t, wantErr, err)
		require.Equal(t, wantRi, ri)
		require.Equal(t, string(want), string(out))
		if err != nil {
			return
		}

		// The digits must encode and decode back unchanged.
		enc, err := semioctet.Encode(out)
		require.NoError(t, err)
		require.Len(t, enc, (len(out)+1)/2)
		back, bn, err := semioctet.Decode(make([]byte, len(out)), enc)
		require.NoError(t, err)
		assert.Equal(t, len(enc), bn)
		assert.Equal(t, string(out), string(back))
	})
}

// FuzzEncode checks that Encode rejects the first byte that is not a
// semi-octet digit, and that anything it encodes decodes back unchanged.
func FuzzEncode(f *testing.F) {
	for _, s := range []string{"", "01234567", "0123456", "0123456789*#abc", "012345D6789"} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, src []byte) {
		enc, err := semioctet.Encode(src)
		for _, c := range src {
			if !strings.ContainsRune("0123456789*#abc", rune(c)) {
				require.Equal(t, semioctet.ErrInvalidDigit(c), err)
				require.Nil(t, enc)
				return
			}
		}
		require.NoError(t, err)
		require.Len(t, enc, (len(src)+1)/2)
		if len(src)%2 == 1 {
			assert.Equal(t, byte(0xf0), enc[len(enc)-1]&0xf0, "fill")
		}
		back, n, err := semioctet.Decode(make([]byte, len(src)), enc)
		require.NoError(t, err)
		assert.Equal(t, len(enc), n)
		assert.Equal(t, string(src), string(back))
	})
}

// TestErrInvalidDigit tests that the errors can be stringified.
// It is fragile, as it compares the strings exactly, but its main purpose is
// to confirm the Error function doesn't recurse, as that is bad.
func TestErrInvalidOctet(t *testing.T) {
	patterns := []byte{0x00, 0xa0, 0x0a, 0x9a, 0xa9, 0xff}
	for _, p := range patterns {
		f := func(t *testing.T) {
			err := semioctet.ErrInvalidDigit(p)
			expected := fmt.Sprintf("semioctet: invalid digit: '%c' - 0x%x", byte(p), int(p))
			s := err.Error()
			assert.Equal(t, expected, s)
		}
		t.Run(fmt.Sprintf("%x", p), f)
	}
}
