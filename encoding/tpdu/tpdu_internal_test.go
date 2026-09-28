// SPDX-License-Identifier: MIT

package tpdu

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecodeUserData(t *testing.T) {
	patterns := []struct {
		name   string
		inPDU  TPDU
		inSrc  []byte
		outUD  UserData
		outUDH UserDataHeader
		n      int
		err    error
	}{
		{"nil",
			TPDU{},
			nil,
			nil,
			nil,
			0,
			NewDecodeError("udl", 0, ErrUnderflow),
		},
		{"empty",
			TPDU{},
			[]byte{0},
			nil,
			nil,
			1,
			nil,
		},
		{"7bit",
			TPDU{},
			[]byte{0x07, 0xED, 0xF2, 0x7C, 0x1E, 0x3E, 0x97, 0x01},
			[]byte("message"),
			nil,
			8,
			nil,
		},
		// TS 23.040 9.2.2.1: "Any unused bits shall be set to zero by the
		// sending entity and shall be ignored by the receiving entity."
		{"non-zero spare septet 7bit",
			TPDU{},
			[]byte{0x07, 0xED, 0xF2, 0x7C, 0x1E, 0x3E, 0x97, 0xf1},
			[]byte("message"),
			nil,
			8,
			nil,
		},
		{"sm underflow",
			TPDU{},
			[]byte{0x07, 0xED, 0xF2, 0x7C, 0x1E, 0x3E, 0x97},
			nil,
			nil,
			0,
			NewDecodeError("sm", 1, ErrUnderflow),
		},
		{"7bit udh",
			TPDU{FirstOctet: 0x40},
			[]byte{
				0x0e, 0x05, 0x00, 0x03, 0x01, 0x02, 0x03, 0xda, 0xe5, 0xf9,
				0x3c, 0x7c, 0x2e, 0x03,
			},
			[]byte("message"),
			UserDataHeader([]InformationElement{{ID: 0, Data: []byte{1, 2, 3}}}),
			14,
			nil,
		},
		{"8bit",
			TPDU{DCS: 0xf4},
			[]byte{0x07, 0xED, 0xF2, 0x7C, 0x1E, 0x3E, 0x97, 0x01},
			[]byte{0xED, 0xF2, 0x7C, 0x1E, 0x3E, 0x97, 0x01},
			nil,
			8,
			nil,
		},
		{"ucs2",
			TPDU{DCS: 0xe0},
			[]byte{
				0x0e, 0x00, 0x6D, 0x00, 0x65, 0x00, 0x73, 0x00, 0x73, 0x00,
				0x61, 0x00, 0x67, 0x00, 0x65,
			},
			[]byte{
				0x00, 0x6D, 0x00, 0x65, 0x00, 0x73, 0x00, 0x73, 0x00, 0x61,
				0x00, 0x67, 0x00, 0x65,
			},
			nil,
			15,
			nil,
		},
		{"odd ucs2",
			TPDU{DCS: 0xe0},
			[]byte{
				0x0d, 0x00, 0x6D, 0x00, 0x65, 0x00, 0x73, 0x00, 0x73, 0x00,
				0x61, 0x00, 0x67, 0x00,
			},
			nil,
			nil,
			0,
			NewDecodeError("sm", 1, ErrOddUCS2Length),
		},
		{"udh only",
			TPDU{FirstOctet: 0x40},
			[]byte{0x06, 0x05, 0x01, 0x03, 0x01, 0x02, 0x03},
			nil,
			UserDataHeader([]InformationElement{{ID: 1, Data: []byte{1, 2, 3}}}),
			7,
			nil,
		},
		{"reserved dcs",
			TPDU{DCS: 0xaa},
			[]byte{0x07, 0xED, 0xF2, 0x7C, 0x1E, 0x3E, 0x97, 0x01},
			[]byte("message"),
			nil,
			8,
			nil,
		},
		{"trailing octet left to caller",
			TPDU{FirstOctet: 0x40, DCS: 0xf4},
			[]byte{0x06, 0x05, 0x01, 0x03, 0x01, 0x02, 0x03, 0x04},
			nil,
			UserDataHeader([]InformationElement{{ID: 1, Data: []byte{1, 2, 3}}}),
			7,
			nil,
		},
		{"trailing octets after udl 0 left to caller",
			TPDU{},
			[]byte{0x00, 0x41},
			nil,
			nil,
			1,
			nil,
		},
		{"short udh",
			TPDU{FirstOctet: 0x40},
			[]byte{0x05, 0x05, 0x01, 0x03, 0x01, 0x02},
			nil,
			nil,
			0,
			NewDecodeError("udh.ie", 2, ErrUnderflow),
		},
		{"ignored udh",
			TPDU{FirstOctet: 0x40},
			[]byte{0x05, 0x04, 0x01, 0x03, 0x01, 0x02},
			nil,
			UserDataHeader{},
			6,
			nil,
		},
	}
	for _, p := range patterns {
		f := func(t *testing.T) {
			n, err := p.inPDU.decodeUserData(p.inSrc)
			require.Equal(t, p.err, err)
			assert.Equal(t, p.n, n)
			assert.Equal(t, p.outUDH, p.inPDU.UDH)
			assert.Equal(t, p.outUD, p.inPDU.UD)
		}
		t.Run(p.name, f)
	}
}

func TestDecode7BitRejectsNegativeShortMessageLength(t *testing.T) {
	patterns := []struct {
		name string
		sml  int
		udhl int
		src  []byte
	}{
		{"direct negative length with empty source", -1, 0, nil},
		{"direct negative length with non-empty source", -1, 0, []byte{0x00}},
		{"UDH-adjusted negative length with empty source", 1, 1, nil},
		{"UDH-adjusted negative length with non-empty source", 1, 1, []byte{0x00, 0x00}},
	}
	for _, p := range patterns {
		t.Run(p.name, func(t *testing.T) {
			sm, err := decode7Bit(p.sml, p.udhl, p.src)
			require.Equal(t, ErrUnderflow, err)
			assert.Nil(t, sm)
		})
	}
}

func TestDecode7BitHandlesSurplusSeptets(t *testing.T) {
	patterns := []struct {
		name  string
		sml   int
		src   []byte
		outSM []byte
		err   error
	}{
		{"drops single trailing zero septet", 0, []byte{0x00}, []byte{}, nil},
		{"drops trailing zero after message septets", 2, []byte{0xcf, 0x25, 0x00}, []byte("OK"), nil}, // "OK\x00" packed
		{"drops single non-zero surplus septet", 0, []byte{0x01}, []byte{}, nil},
		{"drops non-zero surplus septet after message septets", 2, []byte{0xcf, 0x25, 0xfe}, []byte("OK"), nil},
		{"rejects multiple surplus septets", 0, []byte{0x00, 0x00}, nil, ErrOverlength},
	}
	for _, p := range patterns {
		t.Run(p.name, func(t *testing.T) {
			sm, err := decode7Bit(p.sml, 0, p.src)
			require.Equal(t, p.err, err)
			assert.Equal(t, p.outSM, sm)
		})
	}
}

func TestEncodeUserData(t *testing.T) {
	patterns := []struct {
		name string
		in   TPDU
		out  []byte
		err  error
	}{
		{"empty 7bit",
			TPDU{},
			[]byte{0},
			nil,
		},
		{"empty 8bit",
			TPDU{DCS: 0xf4},
			[]byte{0},
			nil,
		},
		{"empty ucs2",
			TPDU{DCS: 0xe0},
			[]byte{0},
			nil,
		},
		{"7bit",
			TPDU{UD: []byte("message")},
			[]byte{0x07, 0xED, 0xF2, 0x7C, 0x1E, 0x3E, 0x97, 0x01},
			nil,
		},
		{"7bit udh",
			TPDU{
				FirstOctet: 0x40,
				UDH: UserDataHeader(
					[]InformationElement{{ID: 0, Data: []byte{1, 2, 3}}}),
				UD: []byte("message"),
			},
			[]byte{
				0x0e, 0x05, 0x00, 0x03, 0x01, 0x02, 0x03, 0xda, 0xe5, 0xf9,
				0x3c, 0x7c, 0x2e, 0x03,
			},
			nil,
		},
		{"8bit udh",
			TPDU{
				FirstOctet: 0x40,
				DCS:        0xf4,
				UDH: UserDataHeader(
					[]InformationElement{{ID: 0, Data: []byte{1, 2, 3}}}),
				UD: []byte("message"),
			},
			[]byte{
				0x0d, 0x05, 0x00, 0x03, 0x01, 0x02, 0x03, 0x6d, 0x65, 0x73,
				0x73, 0x61, 0x67, 0x65,
			},
			nil,
		},
		{"ucs2 udh",
			TPDU{
				FirstOctet: 0x40,
				DCS:        0xe0,
				UDH: UserDataHeader(
					[]InformationElement{{ID: 0, Data: []byte{1, 2, 3}}}),
				UD: []byte{
					0x00, 0x6D, 0x00, 0x65, 0x00, 0x73, 0x00, 0x73, 0x00, 0x61,
					0x00, 0x67, 0x00, 0x65,
				},
			},
			[]byte{
				0x14, 0x05, 0x00, 0x03, 0x01, 0x02, 0x03, 0x00, 0x6D, 0x00,
				0x65, 0x00, 0x73, 0x00, 0x73, 0x00, 0x61, 0x00, 0x67, 0x00,
				0x65,
			},
			nil,
		},
		{"8bit",
			TPDU{
				DCS: 0xf4,
				UD:  []byte{0xED, 0xF2, 0x7C, 0x1E, 0x3E, 0x97, 0x01}},
			[]byte{0x07, 0xED, 0xF2, 0x7C, 0x1E, 0x3E, 0x97, 0x01},
			nil,
		},
		{"ucs2",
			TPDU{
				DCS: 0xe0,
				UD: []byte{
					0x00, 0x6D, 0x00, 0x65, 0x00, 0x73, 0x00, 0x73, 0x00, 0x61,
					0x00, 0x67, 0x00, 0x65,
				},
			},
			[]byte{
				0x0e, 0x00, 0x6D, 0x00, 0x65, 0x00, 0x73, 0x00, 0x73, 0x00,
				0x61, 0x00, 0x67, 0x00, 0x65,
			},
			nil,
		},
		{"odd ucs2",
			TPDU{
				DCS: 0xe0,
				UD: []byte{
					0x00, 0x6D, 0x00, 0x65, 0x00, 0x73, 0x00, 0x73, 0x00, 0x61,
					0x00, 0x67, 0x00,
				},
			},
			nil,
			NewEncodeError("sm", ErrOddUCS2Length),
		},
		{"udh only",
			TPDU{
				FirstOctet: 0x40,
				UDH: UserDataHeader(
					[]InformationElement{{ID: 1, Data: []byte{1, 2, 3}}})},
			// TS 23.040 9.2.3.16: the header and its fill bits are 7
			// septets, which take 7 octets.
			[]byte{0x07, 0x05, 0x01, 0x03, 0x01, 0x02, 0x03, 0x00},
			nil,
		},
		{"reserved dcs",
			TPDU{DCS: 0x80, UD: []byte("message")},
			[]byte{0x07, 0xED, 0xF2, 0x7C, 0x1E, 0x3E, 0x97, 0x01},
			nil,
		},
	}
	for _, p := range patterns {
		f := func(t *testing.T) {
			d, err := p.in.encodeUserData()
			require.Equal(t, p.err, err)
			assert.Equal(t, p.out, d)
		}
		t.Run(p.name, f)
	}
}

func TestChunk(t *testing.T) {
	patterns := []struct {
		name  string
		in    []byte
		alpha Alphabet
		bs    int
		out   [][]byte
	}{
		{
			"7bit",
			[]byte{1, 2, 0x1b, 4, 5, 6, 7, 8},
			Alpha7Bit,
			3,
			[][]byte{
				{0x01, 0x02},
				{0x1b, 0x04, 0x05},
				{0x06, 0x07, 0x08},
			},
		},
		{
			"8bit",
			[]byte{1, 2, 0x1b, 4, 5, 6, 7, 8},
			Alpha8Bit,
			3,
			[][]byte{
				{0x01, 0x02, 0x1b},
				{0x04, 0x05, 0x06},
				{0x07, 0x08},
			},
		},
		{
			"ucs2",
			[]byte{1, 2, 0xd8, 4, 5, 6, 7, 8, 9, 10},
			AlphaUCS2,
			4,
			[][]byte{
				{0x01, 0x02},
				{0xd8, 0x04, 0x05, 0x06},
				{0x07, 0x08, 0x09, 0x0a},
			},
		},
	}
	for _, p := range patterns {
		f := func(t *testing.T) {
			out := chunk(p.in, p.alpha, p.bs)
			assert.Equal(t, p.out, out)
		}
		t.Run(p.name, f)
	}
	// a block too small for a character of the alphabet is refused, rather
	// than looping or panicking.
	msg := []byte{0x1b, 0x65, 0xd8, 0x3d, 0xde, 0x01}
	for _, bs := range []int{-1, 0, 1} {
		assert.Nil(t, chunk(msg, Alpha7Bit, bs), "7bit %d", bs)
	}
	for _, bs := range []int{-1, 0, 1, 2, 3} {
		assert.Nil(t, chunk(msg, AlphaUCS2, bs), "ucs2 %d", bs)
	}
	for _, bs := range []int{-1, 0} {
		assert.Nil(t, chunk(msg, Alpha8Bit, bs), "8bit %d", bs)
	}
	assert.Len(t, chunk(msg, Alpha8Bit, 1), 6)
}

func TestChunk7Bit(t *testing.T) {
	patterns := []struct {
		name string
		in   []byte
		bs   int
		out  [][]byte
	}{
		{
			"nil",
			nil,
			2,
			nil,
		},
		{
			"empty",
			[]byte{},
			2,
			nil,
		},
		{
			"integral",
			[]byte{1, 2, 3, 4},
			2,
			[][]byte{
				{1, 2},
				{3, 4},
			},
		},
		{
			"residual",
			[]byte{1, 2, 3, 4},
			3,
			[][]byte{
				{1, 2, 3},
				{4},
			},
		},
		{
			"three",
			[]byte{1, 2, 3, 4, 5, 6, 7, 8},
			3,
			[][]byte{
				{1, 2, 3},
				{4, 5, 6},
				{7, 8},
			},
		},
		{
			"escaped",
			[]byte{1, 2, 0x1b, 4, 5, 6, 7, 8},
			3,
			[][]byte{
				{1, 2},
				{0x1b, 4, 5},
				{6, 7, 8},
			},
		},
		{
			"double escaped",
			[]byte{1, 0x1b, 0x1b, 4, 5, 6, 7, 8},
			3,
			[][]byte{
				{1, 0x1b, 0x1b},
				{4, 5, 6},
				{7, 8},
			},
		},
		{
			// TS 23.038 6.2.1.1: ESC ESC is itself a sequence, so the
			// third ESC starts the sequence ESC 'e'.
			"escape run odd",
			[]byte{'a', 0x1b, 0x1b, 0x1b, 'e', 'b'},
			4,
			[][]byte{
				{'a', 0x1b, 0x1b},
				{0x1b, 'e', 'b'},
			},
		},
		{
			"escape run of five",
			[]byte{0x1b, 0x1b, 0x1b, 0x1b, 0x1b, 'e', 'b'},
			5,
			[][]byte{
				{0x1b, 0x1b, 0x1b, 0x1b},
				{0x1b, 'e', 'b'},
			},
		},
		{
			"escape run even",
			[]byte{'a', 0x1b, 0x1b, 0x1b, 0x1b, 'b'},
			5,
			[][]byte{
				{'a', 0x1b, 0x1b, 0x1b, 0x1b},
				{'b'},
			},
		},
		{
			"escapes in blocks of 2",
			[]byte{0x1b, 0x1b, 0x1b, 'e', 'a', 0x1b, 'e'},
			2,
			[][]byte{
				{0x1b, 0x1b},
				{0x1b, 'e'},
				{'a'},
				{0x1b, 'e'},
			},
		},
		{
			"escape at block start",
			[]byte{'a', 'b', 0x1b, 'e', 'c', 'd'},
			2,
			[][]byte{
				{'a', 'b'},
				{0x1b, 'e'},
				{'c', 'd'},
			},
		},
	}
	for _, p := range patterns {
		f := func(t *testing.T) {
			out := chunk7Bit(p.in, p.bs)
			assert.Equal(t, p.out, out)
		}
		t.Run(p.name, f)
	}
	// Below the minimum of 2, which Segment never uses, an escape cannot be
	// kept with the next septet, but every chunk still holds a septet.
	assert.Equal(t, [][]byte{{0x1b}, {0x1b}, {0x1b}}, chunk7Bit([]byte{0x1b, 0x1b, 0x1b}, 1))
}

func TestChunk8Bit(t *testing.T) {
	patterns := []struct {
		name string
		in   []byte
		bs   int
		out  [][]byte
	}{
		{
			"nil",
			nil,
			2,
			nil,
		},
		{
			"empty",
			[]byte{},
			2,
			nil,
		},
		{
			"integral",
			[]byte{1, 2, 3, 4},
			2,
			[][]byte{
				{1, 2},
				{3, 4},
			},
		},
		{
			"residual",
			[]byte{1, 2, 3, 4},
			3,
			[][]byte{
				{1, 2, 3},
				{4},
			},
		},
		{
			"three",
			[]byte{1, 2, 3, 4, 5, 6, 7, 8},
			3,
			[][]byte{
				{1, 2, 3},
				{4, 5, 6},
				{7, 8},
			},
		},
	}
	for _, p := range patterns {
		f := func(t *testing.T) {
			out := chunk8Bit(p.in, p.bs)
			assert.Equal(t, p.out, out)
		}
		t.Run(p.name, f)
	}
}

func TestChunkUCS2(t *testing.T) {
	patterns := []struct {
		name string
		in   []byte
		bs   int
		out  [][]byte
	}{
		{
			"nil",
			nil,
			2,
			nil,
		},
		{
			"empty",
			[]byte{},
			2,
			nil,
		},
		{
			"integral",
			[]byte{1, 2, 3, 4},
			2,
			[][]byte{
				{1, 2},
				{3, 4},
			},
		},
		{
			"odd bs",
			[]byte{1, 2, 3, 4},
			3,
			[][]byte{
				{1, 2},
				{3, 4},
			},
		},
		{
			"three",
			[]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
			4,
			[][]byte{
				{1, 2, 3, 4},
				{5, 6, 7, 8},
				{9, 10},
			},
		},
		{
			"surrogate",
			[]byte{1, 2, 0xd8, 4, 5, 6, 7, 8, 9, 10},
			4,
			[][]byte{
				{1, 2},
				{0xd8, 4, 5, 6},
				{7, 8, 9, 10},
			},
		},
		{
			"odd msg",
			[]byte{1, 2, 3, 4, 5, 6, 7, 8, 9},
			4,
			[][]byte{
				{1, 2, 3, 4},
				{5, 6, 7, 8},
				{9},
			},
		},
		{
			// a block of 2 cannot hold a surrogate pair, so it is split
			// rather than looping.
			"surrogate in blocks of 2",
			[]byte{0xd8, 0x3d, 0xde, 0x01, 0x00, 0x61},
			2,
			[][]byte{
				{0xd8, 0x3d},
				{0xde, 0x01},
				{0x00, 0x61},
			},
		},
		{
			"surrogate pairs",
			[]byte{0xd8, 0x3d, 0xde, 0x01, 0xd8, 0x3d, 0xde, 0x01},
			6,
			[][]byte{
				{0xd8, 0x3d, 0xde, 0x01},
				{0xd8, 0x3d, 0xde, 0x01},
			},
		},
	}
	for _, p := range patterns {
		f := func(t *testing.T) {
			out := chunkUCS2(p.in, p.bs)
			assert.Equal(t, p.out, out)
		}
		t.Run(p.name, f)
	}
}
