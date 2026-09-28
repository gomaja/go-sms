// SPDX-License-Identifier: MIT

package bcd_test

import (
	"fmt"
	"testing"

	"github.com/gomaja/go-sms/encoding/bcd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type decodePattern struct {
	name string
	in   byte
	out  int
	err  error
}

func TestDecode(t *testing.T) {
	patterns := []decodePattern{
		{
			"zero",
			0x0,
			0,
			nil,
		},
		{
			"thirteen",
			0x31,
			13,
			nil,
		},
		{
			"thirty one",
			0x13,
			31,
			nil,
		},
		{
			"ninety nine",
			0x99,
			99,
			nil,
		},
		{
			"a9",
			0xa9,
			0,
			bcd.ErrInvalidOctet(0xa9),
		},
		{
			"9a",
			0x9a,
			0,
			bcd.ErrInvalidOctet(0x9a),
		},
		{
			"ff",
			0xff,
			0,
			bcd.ErrInvalidOctet(0xff),
		},
	}
	for _, p := range patterns {
		f := func(t *testing.T) {
			i, err := bcd.Decode(p.in)
			assert.Equal(t, p.err, err)
			assert.Equal(t, p.out, i)
		}
		t.Run(p.name, f)
	}
}

func TestDecodeSigned(t *testing.T) {
	patterns := []decodePattern{
		{
			"zero",
			0x0,
			0,
			nil,
		},
		{
			"thirteen",
			0x31,
			13,
			nil,
		},
		{
			"thirty one",
			0x13,
			31,
			nil,
		},
		{
			"seventy nine",
			0x97,
			79,
			nil,
		},
		{
			"negative zero",
			0x08,
			0,
			nil,
		},
		{
			"negative one",
			0x18,
			-1,
			nil,
		},
		{
			"negative 19",
			0x99,
			-19,
			nil,
		},
		{
			"a9",
			0xa9,
			0,
			bcd.ErrInvalidOctet(0xa9),
		},
		{
			"negative 29",
			0x9a,
			-29,
			nil,
		},
		{
			"negative 79",
			0x9f,
			-79,
			nil,
		},
		{
			"ff",
			0xff,
			0,
			bcd.ErrInvalidOctet(0xff),
		},
	}
	for _, p := range patterns {
		f := func(t *testing.T) {
			i, err := bcd.DecodeSigned(p.in)
			assert.Equal(t, p.err, err)
			assert.Equal(t, p.out, i)
		}
		t.Run(p.name, f)
	}
}

type encodePattern struct {
	name string
	in   int
	out  byte
	err  error
}

func TestEncode(t *testing.T) {
	patterns := []encodePattern{
		{
			"zero",
			0,
			0x0,
			nil,
		},
		{
			"thirteen",
			13,
			0x31,
			nil,
		},
		{
			"thirty one",
			31,
			0x13,
			nil,
		},
		{
			"ninety nine",
			99,
			0x99,
			nil,
		},
		{
			"negative",
			-1,
			0,
			bcd.ErrInvalidInteger(-1),
		},
		{
			"hundred",
			100,
			0,
			bcd.ErrInvalidInteger(100),
		},
		{
			"a9",
			0xa9,
			0,
			bcd.ErrInvalidInteger(0xa9),
		},
		{
			"9a",
			0x9a,
			0,
			bcd.ErrInvalidInteger(0x9a),
		},
		{
			"ff",
			0xff,
			0,
			bcd.ErrInvalidInteger(0xff),
		},
	}
	for _, p := range patterns {
		f := func(t *testing.T) {
			b, err := bcd.Encode(p.in)
			assert.Equal(t, p.err, err)
			assert.Equal(t, p.out, b)
		}
		t.Run(p.name, f)
	}
}

func TestEncodeSigned(t *testing.T) {
	patterns := []encodePattern{
		{
			"zero",
			0,
			0x0,
			nil,
		},
		{
			"thirteen",
			13,
			0x31,
			nil,
		},
		{
			"thirty one",
			31,
			0x13,
			nil,
		},
		{
			"seventy nine",
			79,
			0x97,
			nil,
		},
		{
			"negative one",
			-1,
			0x18,
			nil,
		},
		{
			"negative 19",
			-19,
			0x99,
			nil,
		},
		{
			"a9",
			0xa9,
			0,
			bcd.ErrInvalidInteger(0xa9),
		},
		{
			"negative 29",
			-29,
			0x9a,
			nil,
		},
		{
			"negative 79",
			-79,
			0x9f,
			nil,
		},
		{
			"ff",
			0xff,
			0,
			bcd.ErrInvalidInteger(0xff),
		},
	}
	for _, p := range patterns {
		f := func(t *testing.T) {
			b, err := bcd.EncodeSigned(p.in)
			assert.Equal(t, p.err, err)
			assert.Equal(t, p.out, b)
		}
		t.Run(p.name, f)
	}
}

// TestErrInvalidOctet tests that the errors can be stringified.
//
// It is fragile, as it compares the strings exactly, but its main purpose is
// to confirm the Error function doesn't recurse, as that is bad.
func TestErrInvalidOctet(t *testing.T) {
	patterns := []byte{0x00, 0xa0, 0x0a, 0x9a, 0xa9, 0xff}
	for _, p := range patterns {
		f := func(t *testing.T) {
			err := bcd.ErrInvalidOctet(p)
			expected := fmt.Sprintf("bcd: invalid octet: 0x%02x", p)
			s := err.Error()
			assert.Equal(t, expected, s)
		}
		t.Run(fmt.Sprintf("%x", p), f)
	}
}

// TestErrInvalidInteger tests that the errors can be stringified.
//
// It is fragile, as it compares the strings exactly, but its main purpose is
// to confirm the Error function doesn't recurse, as that is bad.
func TestErrInvalidInteger(t *testing.T) {
	patterns := []int{0, 20, -80, 100}
	for _, p := range patterns {
		f := func(t *testing.T) {
			err := bcd.ErrInvalidInteger(p)
			expected := fmt.Sprintf("bcd: invalid integer: %d", p)
			s := err.Error()
			assert.Equal(t, expected, s)
		}
		t.Run(fmt.Sprintf("%x", p), f)
	}
}

// ExampleEncode shows the semi-octet order, where the most significant digit
// is stored in the lowest nibble.
func ExampleEncode() {
	b, _ := bcd.Encode(12)
	fmt.Printf("0x%02x\n", b)
	// Output: 0x21
}

// FuzzDecode checks that Decode accepts exactly the octets with two nibbles
// in 0..9, and that Encode gives back the same octet.
func FuzzDecode(f *testing.F) {
	for _, b := range []byte{0x00, 0x31, 0x13, 0x99, 0xa9, 0x9a, 0xff} {
		f.Add(b)
	}
	f.Fuzz(func(t *testing.T, b byte) {
		i, err := bcd.Decode(b)
		if b&0x0f > 9 || b>>4 > 9 {
			require.Equal(t, bcd.ErrInvalidOctet(b), err)
			return
		}
		require.NoError(t, err)
		require.Equal(t, int(b&0x0f)*10+int(b>>4), i)
		e, err := bcd.Encode(i)
		require.NoError(t, err)
		assert.Equal(t, b, e)
	})
}

// FuzzDecodeSigned checks that DecodeSigned accepts exactly the octets with
// an upper nibble in 0..9, and that EncodeSigned gives back the same octet,
// except that negative zero is encoded as zero.
func FuzzDecodeSigned(f *testing.F) {
	for _, b := range []byte{0x00, 0x31, 0x97, 0x08, 0x18, 0x9a, 0x9f, 0xa9, 0xff} {
		f.Add(b)
	}
	f.Fuzz(func(t *testing.T, b byte) {
		i, err := bcd.DecodeSigned(b)
		if b>>4 > 9 {
			require.Equal(t, bcd.ErrInvalidOctet(b), err)
			return
		}
		require.NoError(t, err)
		require.GreaterOrEqual(t, i, -79)
		require.LessOrEqual(t, i, 79)
		e, err := bcd.EncodeSigned(i)
		require.NoError(t, err)
		if b == 0x08 {
			assert.Equal(t, byte(0x00), e)
		} else {
			assert.Equal(t, b, e)
		}
	})
}

// FuzzEncode checks the range accepted by Encode and EncodeSigned, and that
// Decode and DecodeSigned give back the integer encoded.
func FuzzEncode(f *testing.F) {
	for _, i := range []int{0, 13, 31, 79, 99, 100, -1, -79, -80} {
		f.Add(i)
	}
	f.Fuzz(func(t *testing.T, i int) {
		b, err := bcd.Encode(i)
		if i < 0 || i > 99 {
			assert.Equal(t, bcd.ErrInvalidInteger(i), err)
		} else {
			require.NoError(t, err)
			d, err := bcd.Decode(b)
			require.NoError(t, err)
			assert.Equal(t, i, d)
		}
		b, err = bcd.EncodeSigned(i)
		if i < -79 || i > 79 {
			assert.Equal(t, bcd.ErrInvalidInteger(i), err)
		} else {
			require.NoError(t, err)
			d, err := bcd.DecodeSigned(b)
			require.NoError(t, err)
			assert.Equal(t, i, d)
		}
	})
}
