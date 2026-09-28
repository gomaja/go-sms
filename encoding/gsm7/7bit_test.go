// SPDX-License-Identifier: MIT

package gsm7_test

import (
	"bytes"
	"fmt"
	"math"
	"testing"

	"github.com/gomaja/go-sms/encoding/gsm7"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const cr byte = 0x0d

type testPattern struct {
	name     string
	fillBits int
	p        []byte
	u        []byte
}

type ussdPattern struct {
	name string
	p    []byte
	u    []byte
}

var (
	testPatterns = []testPattern{
		{
			"nil",
			0,
			nil,
			nil,
		},
		{
			"empty",
			0,
			[]byte{},
			[]byte{},
		},
		{
			"empty fill",
			1,
			[]byte{},
			[]byte{},
		},
		{
			"cr",
			0,
			[]byte{13},
			[]byte("\r"),
		},
		{
			"one",
			0,
			[]byte{49},
			[]byte("1"),
		},
		{
			"two",
			0,
			[]byte{48, 25},
			[]byte("02"),
		},
		{
			"message",
			0,
			[]byte{0xED, 0xF2, 0x7C, 0x1E, 0x3E, 0x97, 0x01},
			[]byte("message\x00"),
		},
		{
			"seven",
			0,
			[]byte{240, 48, 157, 94, 150, 187, 1},
			[]byte("pattern\x00"), // packed is zero filled
		},
		{
			"eight",
			0,
			[]byte{240, 48, 157, 94, 150, 187, 127},
			[]byte("pattern?"),
		},
		{
			"nine",
			0,
			[]byte{240, 48, 157, 94, 150, 187, 67, 33},
			[]byte("pattern!!"),
		},
		{
			"long",
			0,
			[]byte{
				97, 144, 189, 44, 207, 131, 216, 111, 247, 25, 68, 47, 207,
				233, 32, 120, 152, 78, 47, 203, 221,
			},
			[]byte("a very long test pattern"),
		},
		{
			"filler",
			0,
			[]byte{230, 52, 155, 93, 150, 255, 0},
			[]byte("filler?\x00"),
		},
		{
			"fill1",
			1,
			[]byte{0xfe},
			[]byte{0x7f},
		},
		{
			"fill2",
			2,
			[]byte{0xfc, 1},
			[]byte{0x7f, 0x00},
		},
		{
			"fill3",
			3,
			[]byte{0xf8, 3},
			[]byte{0x7f},
		},
		{
			"fill4",
			4,
			[]byte{0xf0, 7},
			[]byte{0x7f},
		},
		{
			"fill5",
			5,
			[]byte{0xe0, 0xf},
			[]byte{0x7f},
		},
		{
			"fill6",
			6,
			[]byte{0xc0, 0x1f},
			[]byte{0x7f},
		},
	}
	// ussdTestPatterns round trip in both directions under the USSD packing
	// rules of 3GPP TS 23.038 V20.0.0 Section 6.1.2.3.1.
	ussdTestPatterns = []ussdPattern{
		{
			"nil",
			nil,
			nil,
		},
		{
			"empty",
			[]byte{},
			[]byte{},
		},
		{
			"cr",
			[]byte{13},
			[]byte("\r"),
		},
		{
			"one",
			[]byte{49},
			[]byte("1"),
		},
		{
			"two",
			[]byte{48, 25},
			[]byte("02"),
		},
		{
			// 2 septets leave 2 spare bits, which are zero whatever the
			// last septet is, so no <CR> is written into them.
			"two pound",
			[]byte{0xc1, 0x00},
			[]byte{0x41, 0x01}, // "A£"
		},
		{
			"two dollar",
			[]byte{0x41, 0x01},
			[]byte{0x41, 0x02}, // "A$"
		},
		{
			// the last octet shifted right by one is <CR>, but 2 septets end
			// mid-octet, so the final septet is data, not filler.
			"reply 14",
			[]byte{0x31, 0x1a},
			[]byte("14"),
		},
		{
			"reply 25",
			[]byte{0xb2, 0x1a},
			[]byte("25"),
		},
		{
			"reply 36",
			[]byte{0x33, 0x1b},
			[]byte("36"),
		},
		{
			"reply 47",
			[]byte{0xb4, 0x1b},
			[]byte("47"),
		},
		{
			"message0",
			[]byte{0xED, 0xF2, 0x7C, 0x1E, 0x3E, 0x97, 0x01},
			[]byte("message\x00"),
		},
		{
			"message",
			[]byte{0xED, 0xF2, 0x7C, 0x1E, 0x3E, 0x97, 0x1b},
			[]byte("message"),
		},
		{
			"seven",
			[]byte{240, 48, 157, 94, 150, 187, 27}, // packed is filled with CR
			[]byte("pattern"),
		},
		{
			// 8n-1 septets ending with '@' still need the <CR> filler.
			"seven at",
			[]byte{0x41, 0, 0, 0, 0, 0, 0x1a},
			[]byte{0x41, 0, 0, 0, 0, 0, 0}, // "A@@@@@@"
		},
		{
			// the filler <CR> follows a wanted <CR>; only the filler is
			// removed.
			"seven cr",
			[]byte{0xf0, 0x30, 0x9d, 0x5e, 0x96, 0x37, 0x1a},
			[]byte("patter\r"),
		},
		{
			"eight",
			[]byte{240, 48, 157, 94, 150, 187, 127},
			[]byte("pattern?"),
		},
		{
			"nine",
			[]byte{240, 48, 157, 94, 150, 187, 67, 33},
			[]byte("pattern!!"),
		},
		{
			"long",
			[]byte{
				97, 144, 189, 44, 207, 131, 216, 111, 247, 25, 68, 47, 207, 233,
				32, 120, 152, 78, 47, 203, 221,
			},
			[]byte("a very long test pattern"),
		},
		{
			"filler",
			[]byte{230, 52, 155, 93, 150, 255, 26},
			[]byte("filler?"),
		},
		{
			"null filler",
			[]byte{230, 52, 155, 93, 150, 255, 0},
			[]byte("filler?\x00"),
		},
		{
			"fifteen",
			[]byte{
				0xe6, 0xb4, 0x99, 0x5e, 0x2e, 0xbb, 0x41, 0x63, 0x74, 0x58, 0x3e,
				0x9f, 0x87, 0x1a,
			},
			[]byte("fifteen charss!"),
		},
	}
	// ussdCRPatterns cover a wanted <CR> ending a message of 8n septets. The
	// packer adds a second <CR> so the receiver does not take the wanted one
	// for filler, and the receiver keeps both, as <CR><CR> is defined to be
	// identical to <CR> (3GPP TS 23.038 Sections 6.1.1 and 6.1.2.3.1).
	ussdCRPatterns = []struct {
		name     string
		u        []byte
		p        []byte
		unpacked []byte
	}{
		{
			"octet cr",
			[]byte("pattern\r"),
			[]byte{240, 48, 157, 94, 150, 187, 27, 13},
			[]byte("pattern\r\r"),
		},
		{
			"sixteen cr",
			[]byte("fifteen charss!\r"),
			[]byte{
				0xe6, 0xb4, 0x99, 0x5e, 0x2e, 0xbb, 0x41, 0x63, 0x74, 0x58, 0x3e,
				0x9f, 0x87, 0x1a, 0x0d,
			},
			[]byte("fifteen charss!\r\r"),
		},
		{
			// 8n+1 septets ending <CR><CR> are packed as is, so they share
			// the packing of "pattern\r" above.
			"nine cr cr",
			[]byte("pattern\r\r"),
			[]byte{240, 48, 157, 94, 150, 187, 27, 13},
			[]byte("pattern\r\r"),
		},
	}
)

func TestUnpack7Bit(t *testing.T) {
	for _, p := range testPatterns {
		f := func(t *testing.T) {
			u := gsm7.Unpack7Bit(p.p, p.fillBits)
			assert.Equal(t, p.u, u)
		}
		t.Run(p.name, f)
	}
}

func TestPack7Bit(t *testing.T) {
	for _, p := range testPatterns {
		f := func(t *testing.T) {
			d, err := gsm7.Pack7Bit(p.u, p.fillBits)
			require.NoError(t, err)
			assert.Equal(t, p.p, d)
		}
		t.Run(p.name, f)
	}
}

// TestPack7BitInvalidSeptet checks that a byte above 0x7F is rejected rather
// than packed. A septet has 7 bits (3GPP TS 23.038 Section 6.1.2.1.1), and
// the 8th bit of a byte would otherwise land in the next septet.
func TestPack7BitInvalidSeptet(t *testing.T) {
	patterns := []struct {
		name string
		u    []byte
		err  gsm7.ErrInvalidSeptet
	}{
		// packed as c1 21 this would unpack as "AC"
		{"first", []byte{0xc1, 0x42}, gsm7.ErrInvalidSeptet{Offset: 0, Septet: 0xc1}},
		{"zero septet", []byte{0x80, 0x00, 0x41}, gsm7.ErrInvalidSeptet{Offset: 0, Septet: 0x80}},
		{"last", []byte{0x41, 0x42, 0xff}, gsm7.ErrInvalidSeptet{Offset: 2, Septet: 0xff}},
		{"eighth", []byte("pattern\x8d"), gsm7.ErrInvalidSeptet{Offset: 7, Septet: 0x8d}},
	}
	for _, p := range patterns {
		t.Run(p.name, func(t *testing.T) {
			for fill := 0; fill <= 6; fill++ {
				out, err := gsm7.Pack7Bit(p.u, fill)
				assert.Equal(t, p.err, err, "fill %d", fill)
				assert.Nil(t, out, "fill %d", fill)
			}
			out, err := gsm7.Pack7BitUSSD(p.u)
			assert.Equal(t, p.err, err, "ussd")
			assert.Nil(t, out, "ussd")
		})
	}
}

// TestFillBitsOutOfRange checks that Pack7Bit and Unpack7Bit panic on a
// number of fill bits other than 0 to 6. Fill bits pad a User Data Header to
// the next septet boundary (3GPP TS 23.040 Section 9.2.3.24), so there are
// at most 6, and any other value is a programming error.
func TestFillBitsOutOfRange(t *testing.T) {
	for _, fill := range []int{math.MinInt, -100, -1, 7, 8, 9, math.MaxInt} {
		msg := fmt.Sprintf("gsm7: fillBits %d not in range 0..6", fill)
		for _, u := range [][]byte{nil, {}, []byte("ABC")} {
			assert.PanicsWithValue(t, msg, func() { _, _ = gsm7.Pack7Bit(u, fill) },
				"pack fill %d % x", fill, u)
			assert.PanicsWithValue(t, msg, func() { _ = gsm7.Unpack7Bit(u, fill) },
				"unpack fill %d % x", fill, u)
		}
	}
	for fill := 0; fill <= 6; fill++ {
		assert.NotPanics(t, func() { _, _ = gsm7.Pack7Bit([]byte("ABC"), fill) })
		assert.NotPanics(t, func() { _ = gsm7.Unpack7Bit([]byte("ABC"), fill) })
	}
}

// TestUnpack7BitCount checks the number of septets Unpack7Bit returns, as
// documented: one for every 7 bits after the fill bits. Where packing leaves
// 7 spare bits in the final octet, they come back as an extra 0x00 septet.
func TestUnpack7BitCount(t *testing.T) {
	for fill := 0; fill <= 6; fill++ {
		for n := 0; n <= 40; n++ {
			u := bytes.Repeat([]byte{0x7f}, n)
			p, err := gsm7.Pack7Bit(u, fill)
			require.NoError(t, err)
			spare := len(p)*8 - fill - n*7
			if n == 0 {
				spare = 0
			}
			require.True(t, spare >= 0 && spare <= 7, "fill %d n %d spare %d", fill, n, spare)
			got := gsm7.Unpack7Bit(p, fill)
			want := u
			if spare == 7 {
				want = append(want, 0x00)
			}
			assert.Equal(t, want, got, "fill %d n %d", fill, n)
			assert.Len(t, got, max(len(p)*8-fill, 0)/7, "fill %d n %d", fill, n)
		}
	}
}

func TestUnpack7BitUSSD(t *testing.T) {
	for _, p := range ussdTestPatterns {
		f := func(t *testing.T) {
			u := gsm7.Unpack7BitUSSD(p.p)
			assert.Equal(t, p.u, u)
		}
		t.Run(p.name, f)
	}
	for _, p := range ussdCRPatterns {
		f := func(t *testing.T) {
			u := gsm7.Unpack7BitUSSD(p.p)
			assert.Equal(t, p.unpacked, u)
		}
		t.Run(p.name, f)
	}
}

func TestPack7BitUSSD(t *testing.T) {
	for _, p := range ussdTestPatterns {
		f := func(t *testing.T) {
			d, err := gsm7.Pack7BitUSSD(p.u)
			require.NoError(t, err)
			assert.Equal(t, p.p, d)
		}
		t.Run(p.name, f)
	}
	for _, p := range ussdCRPatterns {
		f := func(t *testing.T) {
			d, err := gsm7.Pack7BitUSSD(p.u)
			require.NoError(t, err)
			assert.Equal(t, p.p, d)
		}
		t.Run(p.name, f)
	}
}

// TestUSSDLength checks the <CR> filler at every length either side of the
// 8n-1 and 8n boundaries, for septets that defeat bit pattern checks.
func TestUSSDLength(t *testing.T) {
	for _, last := range []byte{0x00, 0x01, 0x02, cr, 0x7f} {
		for n := 1; n <= 33; n++ {
			u := bytes.Repeat([]byte{0x00}, n)
			u[n-1] = last
			p, err := gsm7.Pack7BitUSSD(u)
			require.NoError(t, err)
			octets := (n*7 + 7) / 8
			doubled := n%8 == 0 && last == cr
			if doubled {
				octets++
			}
			require.Len(t, p, octets, "n=%d last=%#x", n, last)
			if n%8 == 7 {
				assert.Equal(t, cr, p[len(p)-1]>>1, "n=%d last=%#x", n, last)
			} else {
				assert.Equal(t, u, gsm7.Unpack7Bit(p, 0)[:n], "n=%d last=%#x", n, last)
			}
			want := u
			if doubled {
				want = append(want, cr)
			}
			assert.Equal(t, want, gsm7.Unpack7BitUSSD(p), "n=%d last=%#x", n, last)
		}
	}
}

// FuzzUSSDRoundTrip checks that any septet string survives USSD packing,
// except a wanted <CR> ending 8n septets, which comes back doubled.
func FuzzUSSDRoundTrip(f *testing.F) {
	for _, p := range ussdTestPatterns {
		f.Add(p.u)
	}
	for _, p := range ussdCRPatterns {
		f.Add(p.u)
	}
	f.Fuzz(func(t *testing.T, s []byte) {
		for i := range s {
			s[i] &= 0x7f
		}
		want := s
		if len(s)%8 == 0 && len(s) > 0 && s[len(s)-1] == cr {
			want = append(s[:len(s):len(s)], cr)
		}
		p, err := gsm7.Pack7BitUSSD(s)
		if err != nil {
			t.Fatalf("pack % x: %v", s, err)
		}
		if len(p) != (len(want)*7+7)/8 {
			t.Fatalf("pack % x: got %d octets % x", s, len(p), p)
		}
		if got := gsm7.Unpack7BitUSSD(p); !bytes.Equal(got, want) {
			t.Fatalf("pack % x -> % x -> unpack % x", s, p, got)
		}
	})
}

// FuzzUnpack7BitUSSD checks that arbitrary octets unpack to one septet per
// 7 bits, less a final <CR> only where the septets end on an octet boundary.
func FuzzUnpack7BitUSSD(f *testing.F) {
	for _, p := range ussdTestPatterns {
		f.Add(p.p)
	}
	f.Fuzz(func(t *testing.T, p []byte) {
		all := gsm7.Unpack7Bit(p, 0)
		want := all
		if len(p)%7 == 0 && len(all) > 0 && all[len(all)-1] == cr {
			want = all[:len(all)-1]
		}
		if got := gsm7.Unpack7BitUSSD(p); !bytes.Equal(got, want) {
			t.Fatalf("unpack % x: got % x, want % x", p, got, want)
		}
	})
}
