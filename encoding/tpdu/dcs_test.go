// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package tpdu_test

import (
	"fmt"
	"testing"

	"github.com/gomaja/go-sms/encoding/tpdu"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type dcsAlphabetPattern struct {
	in  byte
	out tpdu.Alphabet
}

func TestDCSAlphabet(t *testing.T) {
	// might as well test them all..
	patterns := []dcsAlphabetPattern{}
	for i := 0; i < 8; i++ {
		m := byte(i << 4)
		patterns = append(patterns,
			dcsAlphabetPattern{0x00 | m, tpdu.Alpha7Bit},
			dcsAlphabetPattern{0x04 | m, tpdu.Alpha8Bit},
			dcsAlphabetPattern{0x08 | m, tpdu.AlphaUCS2},
			dcsAlphabetPattern{0x0c | m, tpdu.Alpha7Bit},
		)
	}
	// reserved coding groups are assumed to be the GSM 7 bit default
	// alphabet (3GPP TS 23.038 Section 4)
	for i := 0x80; i < 0xc0; i++ {
		patterns = append(patterns,
			dcsAlphabetPattern{byte(i), tpdu.Alpha7Bit},
		)
	}
	for i := 0xc0; i < 0xe0; i++ {
		patterns = append(patterns,
			dcsAlphabetPattern{byte(i), tpdu.Alpha7Bit},
		)
	}
	for i := 0xe0; i < 0xf0; i++ {
		patterns = append(patterns,
			dcsAlphabetPattern{byte(i), tpdu.AlphaUCS2},
		)
	}
	for i := 0xf0; i <= 0xff; i++ {
		a := tpdu.Alpha7Bit
		if i&0x04 == 0x04 {
			a = tpdu.Alpha8Bit
		}
		patterns = append(patterns,
			dcsAlphabetPattern{byte(i), a},
		)
	}
	for _, p := range patterns {
		f := func(t *testing.T) {
			d := tpdu.DCS(p.in)
			a := d.Alphabet()
			assert.Equal(t, p.out, a)
		}
		t.Run(fmt.Sprintf("%08b", p.in), f)
	}
}

func TestApplyTPDUOption(t *testing.T) {
	s, err := tpdu.New(tpdu.DCS(0x34))
	require.Nil(t, err)
	assert.Equal(t, tpdu.DCS(0x34), s.DCS)
}

type dcsWithAlphabetIn struct {
	in byte
	a  tpdu.Alphabet
}

type dcsWithAlphabetOut struct {
	out tpdu.DCS
	err error
}

func TestDCSWithAlphabet(t *testing.T) {
	// might as well test them all..
	patterns := make(map[dcsWithAlphabetIn]dcsWithAlphabetOut) // the good ones
	for i := 0x00; i < 0x80; i++ {
		for a := 0; a < 3; a++ {
			patterns[dcsWithAlphabetIn{byte(i), tpdu.Alphabet(a)}] =
				dcsWithAlphabetOut{tpdu.DCS(i&^0x0c | a<<2), nil}
		}
	}
	for i := 0xc0; i < 0xe0; i++ {
		patterns[dcsWithAlphabetIn{byte(i), tpdu.Alpha7Bit}] =
			dcsWithAlphabetOut{tpdu.DCS(i), nil}
	}
	for i := 0xe0; i < 0xf0; i++ {
		patterns[dcsWithAlphabetIn{byte(i), tpdu.AlphaUCS2}] =
			dcsWithAlphabetOut{tpdu.DCS(i), nil}
	}
	for i := 0xf0; i <= 0xff; i++ {
		for a := 0; a < 2; a++ {
			patterns[dcsWithAlphabetIn{byte(i), tpdu.Alphabet(a)}] =
				dcsWithAlphabetOut{tpdu.DCS(i&^0x0c | a<<2), nil}
		}
	}
	for i := 0x00; i <= 0xff; i++ {
		for a := -1; a < 5; a++ {
			p, ok := patterns[dcsWithAlphabetIn{byte(i), tpdu.Alphabet(a)}]
			if !ok {
				p = dcsWithAlphabetOut{tpdu.DCS(i), tpdu.ErrInvalid} // the bad ones
			}
			f := func(t *testing.T) {
				d := tpdu.DCS(i)
				dcs, err := d.WithAlphabet(tpdu.Alphabet(a))
				assert.Equal(t, p.err, err)
				assert.Equal(t, p.out, dcs)
			}
			t.Run(fmt.Sprintf("%08b_%d", i, a), f)
		}
	}
}

type dcsClassPattern struct {
	in  byte
	out tpdu.MessageClass
}

func TestDCSClass(t *testing.T) {
	// might as well test them all..
	patterns := []dcsClassPattern{}
	for i := 0; i < 4; i++ {
		m := byte(i << 5)
		for cs := byte(0); cs < 0x0c; cs += 4 {
			patterns = append(patterns,
				dcsClassPattern{0x00 | cs | m, tpdu.MClassUnknown},
				dcsClassPattern{0x01 | cs | m, tpdu.MClassUnknown},
				dcsClassPattern{0x02 | cs | m, tpdu.MClassUnknown},
				dcsClassPattern{0x03 | cs | m, tpdu.MClassUnknown},
				dcsClassPattern{0x10 | cs | m, tpdu.MClass0},
				dcsClassPattern{0x11 | cs | m, tpdu.MClass1},
				dcsClassPattern{0x12 | cs | m, tpdu.MClass2},
				dcsClassPattern{0x13 | cs | m, tpdu.MClass3},
			)
		}
		// the reserved character set is a reserved coding, assumed to be
		// 00000000, which has no class (3GPP TS 23.038 Section 4)
		for i := byte(0); i < 0x20; i++ {
			if i&0x0c == 0x0c {
				patterns = append(patterns, dcsClassPattern{i | m, tpdu.MClassUnknown})
			}
		}
	}
	// reserved coding groups are assumed to be 00000000, which has no class
	// (3GPP TS 23.038 Section 4)
	for i := 0x80; i < 0xc0; i++ {
		patterns = append(patterns,
			dcsClassPattern{byte(i), tpdu.MClassUnknown},
		)
	}
	for i := 0xc0; i < 0xe0; i++ {
		patterns = append(patterns,
			dcsClassPattern{byte(i), tpdu.MClassUnknown},
		)
	}
	for i := 0xe0; i < 0xf0; i++ {
		patterns = append(patterns,
			dcsClassPattern{byte(i), tpdu.MClassUnknown},
		)
	}
	for i := 0xf0; i <= 0xff; i++ {
		patterns = append(patterns,
			dcsClassPattern{byte(i), tpdu.MessageClass(i & 0x3)},
		)
	}
	for _, p := range patterns {
		f := func(t *testing.T) {
			d := tpdu.DCS(p.in)
			c := d.Class()
			assert.Equal(t, p.out, c)
		}
		t.Run(fmt.Sprintf("%08b", p.in), f)
	}
}

type dcsWithClassIn struct {
	in byte
	c  tpdu.MessageClass
}

type dcsWithClassOut struct {
	out tpdu.DCS
	err error
}

func TestDCSWithClass(t *testing.T) {
	// might as well test them all..
	patterns := make(map[dcsWithClassIn]dcsWithClassOut) // the good ones
	for i := 0x00; i < 0x80; i++ {
		if i&0x0c == 0x0c {
			// a reserved coding, so incompatible with a class
			continue
		}
		for a := 0; a < 4; a++ {
			patterns[dcsWithClassIn{byte(i), tpdu.MessageClass(a)}] =
				dcsWithClassOut{tpdu.DCS(i&^0x03 | 0x10 | a), nil}
		}
		patterns[dcsWithClassIn{byte(i), tpdu.MClassUnknown}] =
			dcsWithClassOut{tpdu.DCS(i &^ 0x13), nil}
	}
	for i := 0xc0; i < 0xf0; i++ {
		patterns[dcsWithClassIn{byte(i), tpdu.MClassUnknown}] =
			dcsWithClassOut{tpdu.DCS(i), nil}
	}
	for i := 0xf0; i <= 0xff; i++ {
		for a := 0; a < 4; a++ {
			patterns[dcsWithClassIn{byte(i), tpdu.MessageClass(a)}] =
				dcsWithClassOut{tpdu.DCS(i&^0x03 | a), nil}
		}
	}
	for i := 0x00; i <= 0xff; i++ {
		for a := -1; a < 6; a++ {
			p, ok := patterns[dcsWithClassIn{byte(i), tpdu.MessageClass(a)}]
			if !ok {
				p = dcsWithClassOut{tpdu.DCS(i), tpdu.ErrInvalid} // the bad ones
			}
			f := func(t *testing.T) {
				d := tpdu.DCS(i)
				dcs, err := d.WithClass(tpdu.MessageClass(a))
				assert.Equal(t, p.err, err)
				assert.Equal(t, p.out, dcs)
			}
			t.Run(fmt.Sprintf("%08b_%d", i, a), f)
		}
	}
}

func TestDCSReservedCodingGroups(t *testing.T) {
	// 3GPP TS 23.038 Section 4: "Any reserved codings shall be assumed to be
	// the GSM 7 bit default alphabet (the same as codepoint 00000000) by a
	// receiving entity."
	for i := 0x00; i < 0xc0; i++ {
		if i < 0x80 && i&0x0c != 0x0c {
			// only the character set 11 of groups 00xx and 01xx is reserved
			continue
		}
		d := tpdu.DCS(i)
		assert.Equal(t, tpdu.Alpha7Bit, d.Alphabet(), "%02x", i)
		assert.Equal(t, tpdu.MClassUnknown, d.Class(), "%02x", i)
		assert.False(t, d.Compressed(), "%02x", i)
	}
}

func TestDCSClassNone(t *testing.T) {
	// 3GPP TS 23.038 Section 4: in groups 00xx and 01xx bit 4 set to 0
	// "indicates that bits 1 to 0 are reserved and have no message class
	// meaning", which is valid, not an error.
	for _, i := range []int{0x00, 0x01, 0x04, 0x08, 0x0c, 0x23, 0x40, 0x6b} {
		assert.Equal(t, tpdu.MClassUnknown, tpdu.DCS(i).Class(), "%02x", i)
	}
}

func TestDCSWithClassRange(t *testing.T) {
	patterns := []struct {
		name string
		in   tpdu.DCS
		c    tpdu.MessageClass
		out  tpdu.DCS
		err  error
	}{
		{"none 7bit", 0x00, tpdu.MClassUnknown, 0x00, nil},
		{"none clears class", 0x12, tpdu.MClassUnknown, 0x00, nil},
		{"none keeps ucs2", 0x3b, tpdu.MClassUnknown, 0x28, nil},
		{"none 1111", 0xf1, tpdu.MClassUnknown, 0xf1, tpdu.ErrInvalid},
		{"none mwi", 0xc8, tpdu.MClassUnknown, 0xc8, nil},
		{"none ucs2 mwi", 0xe3, tpdu.MClassUnknown, 0xe3, nil},
		{"none reserved", 0x80, tpdu.MClassUnknown, 0x80, tpdu.ErrInvalid},
		{"negative", 0x00, -1, 0x00, tpdu.ErrInvalid},
		{"too large", 0x00, tpdu.MClassUnknown + 1, 0x00, tpdu.ErrInvalid},
		{"too large 1111", 0xf0, 8, 0xf0, tpdu.ErrInvalid},
	}
	for _, p := range patterns {
		f := func(t *testing.T) {
			out, err := p.in.WithClass(p.c)
			assert.Equal(t, p.err, err)
			assert.Equal(t, p.out, out)
		}
		t.Run(p.name, f)
	}
}

func TestDCSWithAlphabetRange(t *testing.T) {
	patterns := []struct {
		name string
		in   tpdu.DCS
		a    tpdu.Alphabet
		out  tpdu.DCS
		err  error
	}{
		{"reserved", 0x00, tpdu.AlphaReserved, 0x00, tpdu.ErrInvalid},
		{"negative", 0x00, -1, 0x00, tpdu.ErrInvalid},
		{"too large", 0x00, 4, 0x00, tpdu.ErrInvalid},
		{"negative 1111", 0xf0, -1, 0xf0, tpdu.ErrInvalid},
		{"too large 1111", 0xf0, 8, 0xf0, tpdu.ErrInvalid},
	}
	for _, p := range patterns {
		f := func(t *testing.T) {
			out, err := p.in.WithAlphabet(p.a)
			assert.Equal(t, p.err, err)
			assert.Equal(t, p.out, out)
		}
		t.Run(p.name, f)
	}
}

func TestReservedDCSDeliver(t *testing.T) {
	// SMS-DELIVER with DCS 0x80 (reserved coding group 1000) and the 7-bit
	// UD "hello", which a receiving entity must decode as GSM 7 bit default
	// alphabet, as per 3GPP TS 23.038 Section 4.
	for _, dcs := range []byte{0x80, 0x9f, 0xaf, 0xbf} {
		b := []byte{
			0x04,                                           // first octet
			0x0b, 0x91, 0x16, 0x04, 0x89, 0x56, 0x26, 0xf9, // OA
			0x00,                                     // PID
			dcs,                                      // DCS
			0x71, 0x80, 0x13, 0x11, 0x12, 0x45, 0x23, // SCTS
			0x05, 0xe8, 0x32, 0x9b, 0xfd, 0x06, // UDL and UD "hello"
		}
		var pdu tpdu.TPDU
		err := pdu.UnmarshalBinary(b)
		require.Nil(t, err, "%02x", dcs)
		assert.Equal(t, tpdu.UserData("hello"), pdu.UD)
		assert.Equal(t, tpdu.DCS(dcs), pdu.DCS)
		m, err := pdu.MarshalBinary()
		require.Nil(t, err, "%02x", dcs)
		assert.Equal(t, b, m)
	}
}

func TestDCSCompressed(t *testing.T) {
	patterns := []struct {
		in  int
		out bool
	}{
		{0x00, false},
		{0x10, false},
		{0x20, true},
		{0x2c, false}, // reserved character set, assumed to be 0x00
		{0x30, true},
		{0x3c, false},
		{0x40, false},
		{0x50, false},
		{0x60, true},
		{0x6c, false},
		{0x70, true},
		{0x7f, false},
		{0x80, false},
		{0x90, false},
		{0xa0, false},
		{0xb0, false},
		{0xc0, false},
		{0xd0, false},
		{0xe0, false},
		{0xf0, false},
	}
	for _, p := range patterns {
		f := func(t *testing.T) {
			d := tpdu.DCS(p.in)
			c := d.Compressed()
			assert.Equal(t, p.out, c)
		}
		t.Run(fmt.Sprintf("%02x", p.in), f)
	}
}

func TestDCSString(t *testing.T) {
	patterns := []struct {
		in  int
		out string
	}{
		{0x00, "0x00 7bit"},
		{0xf4, "0xf4 8bit"},
		{0xe0, "0xe0 UCS-2"},
		{0x80, "0x80 7bit"},
	}
	for _, p := range patterns {
		f := func(t *testing.T) {
			out := tpdu.DCS(p.in).String()
			assert.Equal(t, p.out, out)
		}
		t.Run(fmt.Sprintf("%02x", p.in), f)
	}
}

// FuzzDCS checks that any DCS octet decodes to a defined alphabet and class,
// and that WithAlphabet and WithClass either change only the requested field
// or reject the request and return the DCS unchanged.
func FuzzDCS(f *testing.F) {
	for _, d := range []byte{0x00, 0x04, 0x08, 0x0c, 0x10, 0x16, 0x2b, 0x80, 0xbf, 0xc8, 0xe3, 0xf4, 0xff} {
		f.Add(d, int(tpdu.Alpha8Bit), int(tpdu.MClass2))
	}
	f.Add(byte(0x00), -1, int(tpdu.MClassUnknown))
	f.Add(byte(0xf0), int(tpdu.AlphaReserved), 5)
	f.Fuzz(func(t *testing.T, b byte, a, c int) {
		d := tpdu.DCS(b)
		alpha := d.Alphabet()
		if alpha != tpdu.Alpha7Bit && alpha != tpdu.Alpha8Bit && alpha != tpdu.AlphaUCS2 {
			t.Fatalf("%02x: alphabet %d", b, alpha)
		}
		class := d.Class()
		if class < tpdu.MClass0 || class > tpdu.MClassUnknown {
			t.Fatalf("%02x: class %d", b, class)
		}
		reserved := b&0xc0 == 0x80 || (b&0x80 == 0 && b&0x0c == 0x0c)
		if reserved && (alpha != tpdu.Alpha7Bit || class != tpdu.MClassUnknown || d.Compressed()) {
			t.Fatalf("%02x: reserved coding not treated as 0x00", b)
		}
		_ = d.String()

		wa, err := d.WithAlphabet(tpdu.Alphabet(a))
		if err != nil {
			if wa != d {
				t.Fatalf("%02x.WithAlphabet(%d) failed but changed DCS to %02x", b, a, byte(wa))
			}
		} else if wa.Alphabet() != tpdu.Alphabet(a) ||
			// setting the alphabet of a reserved coding makes its other
			// bits meaningful
			!reserved && (wa.Class() != class || wa.Compressed() != d.Compressed()) {
			t.Fatalf("%02x.WithAlphabet(%d) = %02x", b, a, byte(wa))
		}

		wc, err := d.WithClass(tpdu.MessageClass(c))
		if err != nil {
			if wc != d {
				t.Fatalf("%02x.WithClass(%d) failed but changed DCS to %02x", b, c, byte(wc))
			}
		} else if wc.Class() != tpdu.MessageClass(c) || wc.Alphabet() != alpha || wc.Compressed() != d.Compressed() {
			t.Fatalf("%02x.WithClass(%d) = %02x", b, c, byte(wc))
		}
	})
}
