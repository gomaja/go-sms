// SPDX-License-Identifier: MIT

package tpdu_test

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/gomaja/go-sms/encoding/gsm7"
	"github.com/gomaja/go-sms/encoding/semioctet"
	"github.com/gomaja/go-sms/encoding/tpdu"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewAddress(t *testing.T) {
	a := tpdu.NewAddress()
	assert.Equal(t, uint8(0x80), a.TOA)
	a = tpdu.NewAddress(tpdu.FromNumber("1234"))
	assert.Equal(t, uint8(0x91), a.TOA)
	assert.Equal(t, "1234", a.Addr)
	a = tpdu.NewAddress(tpdu.FromNumber("+4321"))
	assert.Equal(t, uint8(0x91), a.TOA)
	assert.Equal(t, "4321", a.Addr)
}

type addressMarshalPattern struct {
	name string
	in   tpdu.Address
	out  []byte
	err  error
}

func TestAddressMarshalBinary(t *testing.T) {
	patterns := []addressMarshalPattern{
		{"empty",
			tpdu.Address{},
			[]byte{0, 0},
			nil,
		},
		{"number",
			tpdu.Address{Addr: "61409865629", TOA: 0x91},
			[]byte{11, 0x91, 0x16, 0x04, 0x89, 0x56, 0x26, 0xf9},
			nil,
		},
		{"number alphabet",
			tpdu.Address{Addr: "0123456789*#abc", TOA: 0x91},
			[]byte{15, 0x91, 0x10, 0x32, 0x54, 0x76, 0x98, 0xba, 0xdc, 0xfe},
			nil,
		},
		{"alpha",
			tpdu.Address{Addr: "messages", TOA: 0xd1},
			[]byte{14, 0xd1, 0xED, 0xF2, 0x7C, 0x1E, 0x3E, 0x97, 0xE7},
			nil,
		},
		{"alpha odd",
			tpdu.Address{Addr: "message", TOA: 0xd1},
			[]byte{13, 0xd1, 0xED, 0xF2, 0x7C, 0x1E, 0x3E, 0x97, 0x01},
			nil,
		},
		{"alpha Vodafone",
			tpdu.Address{Addr: "Vodafone", TOA: 0xd0},
			[]byte{14, 0xd0, 0xD6, 0x37, 0x39, 0x6C, 0x7E, 0xBB, 0xCB},
			nil,
		},
		{"invalid number",
			tpdu.Address{Addr: "6140f98656", TOA: 0x91},
			nil,
			tpdu.NewEncodeError("addr", semioctet.ErrInvalidDigit('f')),
		},
		// 3GPP TS 23.040 Section 9.1.2.5: "The maximum length of the full
		// address field (Address-Length, Type-of-Address and Address-Value)
		// is 12 octets."
		{"max number",
			tpdu.Address{Addr: "12345678901234567890", TOA: 0x91},
			[]byte{20, 0x91, 0x21, 0x43, 0x65, 0x87, 0x09, 0x21, 0x43, 0x65, 0x87, 0x09},
			nil,
		},
		{"overlength number",
			tpdu.Address{Addr: "123456789012345678901", TOA: 0x91},
			nil,
			tpdu.NewEncodeError("addr", tpdu.ErrOverlength),
		},
		{"length wraps",
			tpdu.Address{Addr: strings.Repeat("1", 256), TOA: 0x81},
			nil,
			tpdu.NewEncodeError("addr", tpdu.ErrOverlength),
		},
		{"max alpha",
			tpdu.Address{Addr: "Hello World", TOA: 0xd0},
			[]byte{20, 0xd0, 0xc8, 0x32, 0x9b, 0xfd, 0x06, 0x5d, 0xdf, 0x72, 0x36, 0x19},
			nil,
		},
		{"overlength alpha",
			tpdu.Address{Addr: "Hello World!", TOA: 0xd0},
			nil,
			tpdu.NewEncodeError("addr", tpdu.ErrOverlength),
		},
		// A command not tied to a particular SM has a zero length DA with a
		// zero TOA.
		{"empty command DA",
			tpdu.Address{},
			[]byte{0x00, 0x00},
			nil,
		},
		// test characters only available in the extension table - which should
		// be unavailable.
		{"invalid alpha euro",
			tpdu.Address{Addr: "a euro €32", TOA: 0xd1},
			nil,
			tpdu.NewEncodeError("addr", gsm7.ErrUnencodable{Offset: 7, Rune: '€'}),
		},
		{"invalid alpha bar",
			tpdu.Address{Addr: "a bar | ", TOA: 0xd1},
			nil,
			tpdu.NewEncodeError("addr", gsm7.ErrUnencodable{Offset: 6, Rune: '|'}),
		},
		// test characters not available in the default character set at all.
		{"invalid alpha",
			tpdu.Address{Addr: "mes⌘sages", TOA: 0xd1},
			nil,
			tpdu.NewEncodeError("addr", gsm7.ErrUnencodable{Offset: 3, Rune: '⌘'}),
		},
	}
	for _, p := range patterns {
		f := func(t *testing.T) {
			b, err := p.in.MarshalBinary()
			assert.Equal(t, p.err, err)
			assert.Equal(t, p.out, b)
		}
		t.Run(p.name, f)
	}
}

type addressUnmarshalPattern struct {
	name string
	in   []byte
	out  tpdu.Address
	n    int
	err  error
}

func TestAddressUnmarshalBinary(t *testing.T) {
	patterns := []addressUnmarshalPattern{
		{"empty",
			[]byte{0, 0},
			tpdu.Address{},
			2,
			nil,
		},
		{"number",
			[]byte{11, 0x91, 0x16, 0x04, 0x89, 0x56, 0x26, 0xf9},
			tpdu.Address{Addr: "61409865629", TOA: 0x91},
			8,
			nil,
		},
		{"number alphabet",
			[]byte{15, 0x91, 0x10, 0x32, 0x54, 0x76, 0x98, 0xba, 0xdc, 0xfe},
			tpdu.Address{Addr: "0123456789*#abc", TOA: 0x91},
			10,
			nil},
		{"alpha",
			[]byte{14, 0xd1, 0xED, 0xF2, 0x7C, 0x1E, 0x3E, 0x97, 0xE7},
			tpdu.Address{Addr: "messages", TOA: 0xd1},
			9,
			nil,
		},
		{"alpha odd",
			[]byte{13, 0xd1, 0xED, 0xF2, 0x7C, 0x1E, 0x3E, 0x97, 0x01},
			tpdu.Address{Addr: "message", TOA: 0xd1},
			9,
			nil,
		},
		{"alpha Vodafone",
			[]byte{14, 0xd0, 0xD6, 0x37, 0x39, 0x6C, 0x7E, 0xBB, 0xCB},
			tpdu.Address{Addr: "Vodafone", TOA: 0xd0},
			9,
			nil,
		},
		{"overlong number",
			[]byte{11, 0x91, 0x16, 0x04, 0x89, 0x56, 0x26, 0x09},
			tpdu.Address{},
			8,
			tpdu.NewDecodeError("addr", 2, semioctet.ErrMissingFill),
		},
		{"short binary",
			[]byte{0}, tpdu.Address{},
			0,
			tpdu.NewDecodeError("addr", 0, tpdu.ErrUnderflow),
		},
		{"short number",
			[]byte{11, 0x91, 0x16, 0x04, 0x89, 0x56},
			tpdu.Address{},
			6,
			tpdu.NewDecodeError("addr", 2, tpdu.ErrUnderflow),
		},
		{"short number pad",
			[]byte{12, 0x91, 0x16, 0x04, 0x89, 0x56, 0x97, 0xf7},
			tpdu.Address{},
			8,
			tpdu.NewDecodeError("addr", 2, tpdu.ErrUnderflow),
		},
		{"invalid alpha bar",
			[]byte{10, 0xd1, 0xED, 0xF2, 0x7C, 0x03, 0x9c, 0x87, 0xCF, 0xE5, 0x39},
			tpdu.Address{},
			2,
			tpdu.NewDecodeError("addr", 2, gsm7.ErrInvalidSeptet{Offset: 4, Septet: 0x40, Escaped: true}),
		},
		{"underflow alpha",
			[]byte{10, 0xd1, 0xCF, 0xE5, 0x39},
			tpdu.Address{},
			5,
			tpdu.NewDecodeError("addr", 2, tpdu.ErrUnderflow),
		},
		{"max number",
			[]byte{20, 0x91, 0x21, 0x43, 0x65, 0x87, 0x09, 0x21, 0x43, 0x65, 0x87, 0x09},
			tpdu.Address{Addr: "12345678901234567890", TOA: 0x91},
			12,
			nil,
		},
		// 3GPP TS 23.040 Section 9.1.2.3: "If a mobile receives "1111" in a
		// position prior to the last semi-octet then processing shall
		// commence with the next semi-octet and the intervening semi-octet
		// shall be ignored."
		{"fill within number",
			[]byte{11, 0x91, 0x16, 0x04, 0x26, 0xf9, 0x89, 0x56},
			tpdu.Address{Addr: "61406299865", TOA: 0x91},
			8,
			nil,
		},
		{"max alpha",
			[]byte{20, 0xd0, 0xc8, 0x32, 0x9b, 0xfd, 0x06, 0x5d, 0xdf, 0x72, 0x36, 0x19},
			tpdu.Address{Addr: "Hello World", TOA: 0xd0},
			12,
			nil,
		},
		{"overlength number",
			[]byte{21, 0x91, 0x21, 0x43, 0x65, 0x87, 0x09, 0x21, 0x43, 0x65, 0x87, 0x09, 0xf1},
			tpdu.Address{},
			0,
			tpdu.NewDecodeError("addr", 0, tpdu.ErrOverlength),
		},
		{"overlength alpha",
			[]byte{22, 0xd0, 0xc8, 0x32, 0x9b, 0xfd, 0x06, 0x5d, 0xdf, 0x72, 0x36, 0x39, 0x04},
			tpdu.Address{},
			0,
			tpdu.NewDecodeError("addr", 0, tpdu.ErrOverlength),
		},
		{"max length octet",
			append([]byte{255, 0x81}, bytes.Repeat([]byte{0x11}, 128)...),
			tpdu.Address{},
			0,
			tpdu.NewDecodeError("addr", 0, tpdu.ErrOverlength),
		},
	}
	for _, p := range patterns {
		f := func(t *testing.T) {
			a := tpdu.Address{}
			n, err := a.UnmarshalBinary(p.in)
			assert.Equal(t, p.err, err)
			assert.Equal(t, p.n, n)
			assert.Equal(t, p.out, a)
		}
		t.Run(p.name, f)
	}
}

func TestAddressNumber(t *testing.T) {
	a := tpdu.Address{Addr: "61409865629", TOA: 0}
	assert.Equal(t, "61409865629", a.Number())
	a.SetTypeOfNumber(tpdu.TonInternational)
	assert.Equal(t, "+61409865629", a.Number())
}

func TestAddressNumberingPlan(t *testing.T) {
	patterns := []tpdu.NumberingPlan{0, 2, 0xf, 0x12}
	for _, p := range patterns {
		f := func(t *testing.T) {
			a := tpdu.NewAddress()
			a.SetNumberingPlan((p))
			assert.Equal(t, p&0x0f, a.NumberingPlan())
		}
		t.Run(fmt.Sprintf("%02x", p), f)
	}
}

func TestAddressSetNumber(t *testing.T) {
	a := tpdu.NewAddress()
	assert.Equal(t, uint8(0x80), a.TOA)
	a.SetNumber("1234")
	assert.Equal(t, uint8(0x91), a.TOA)
	assert.Equal(t, "1234", a.Addr)
	assert.Equal(t, "+1234", a.Number())
	a.SetNumber("+4321")
	assert.Equal(t, uint8(0x91), a.TOA)
	assert.Equal(t, "4321", a.Addr)
	assert.Equal(t, "+4321", a.Number())
}

func TestAddressTypeOfNumber(t *testing.T) {
	patterns := []tpdu.TypeOfNumber{0, 2, 3, 5}
	for _, p := range patterns {
		f := func(t *testing.T) {
			a := tpdu.NewAddress()
			a.SetTypeOfNumber((p))
			ton := a.TypeOfNumber()
			assert.Equal(t, p&0x0f, ton)
		}
		t.Run(fmt.Sprintf("%02x", p), f)
	}
}

func TestAddressOverlengthTPDU(t *testing.T) {
	// The address error must reach the caller rather than a TPDU with an
	// oversize address field.
	pdu, err := tpdu.NewSubmit(tpdu.WithDA(tpdu.NewAddress(tpdu.FromNumber("+123456789012345678901"))))
	require.Nil(t, err)
	pdu.UD = []byte("hello")
	b, err := pdu.MarshalBinary()
	assert.Equal(t, tpdu.NewEncodeError("SmsSubmit.da.addr", tpdu.ErrOverlength), err)
	assert.Nil(t, b)
}

// midFill reports whether a semi-octet address value contains a fill
// semi-octet (1111) before its last semi-octet.
func midFill(value []byte) bool {
	for i := 0; i < 2*len(value)-1; i++ {
		if (value[i/2]>>(4*(i%2)))&0x0f == 0x0f {
			return true
		}
	}
	return false
}

// FuzzAddressUnmarshalBinary checks that a decoded address marshals back to
// the octets it was decoded from.
//
// The exceptions are a semi-octet address with a fill semi-octet before the
// last, which is ignored (3GPP TS 23.040 Section 9.1.2.3), and an
// alphanumeric address, where an Address-Length that does not match the
// number of septets, or non-zero fill bits, carry no information. Neither is
// retained, so such an address must re-marshal to one that decodes to the
// same Address, and is no longer.
func FuzzAddressUnmarshalBinary(f *testing.F) {
	for _, seed := range [][]byte{
		{0, 0},
		{11, 0x91, 0x16, 0x04, 0x89, 0x56, 0x26, 0xf9},
		{15, 0x91, 0x10, 0x32, 0x54, 0x76, 0x98, 0xba, 0xdc, 0xfe},
		{14, 0xd1, 0xED, 0xF2, 0x7C, 0x1E, 0x3E, 0x97, 0xE7},
		{13, 0xd1, 0xED, 0xF2, 0x7C, 0x1E, 0x3E, 0x97, 0x01},
		{14, 0xd0, 0xD6, 0x37, 0x39, 0x6C, 0x7E, 0xBB, 0xCB},
		{20, 0x91, 0x21, 0x43, 0x65, 0x87, 0x09, 0x21, 0x43, 0x65, 0x87, 0x09},
		{20, 0xd0, 0xc8, 0x32, 0x9b, 0xfd, 0x06, 0x5d, 0xdf, 0x72, 0x36, 0x19},
		{21, 0x91, 0x21, 0x43, 0x65, 0x87, 0x09, 0x21, 0x43, 0x65, 0x87, 0x09, 0xf1},
		{11, 0x91, 0x16, 0x04, 0x89, 0x56, 0x26, 0x09},
		{11, 0x91, 0x16, 0x04, 0x26, 0xf9, 0x89, 0x56},
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, src []byte) {
		var a tpdu.Address
		n, err := a.UnmarshalBinary(src)
		if err != nil {
			return
		}
		if n != 2+(int(src[0])+1)/2 || n > len(src) {
			t.Fatalf("% x: read %d octets", src, n)
		}
		b, err := a.MarshalBinary()
		if err != nil {
			t.Fatalf("% x: %+v marshal error %v", src, a, err)
		}
		if a.TypeOfNumber() != tpdu.TonAlphanumeric && !midFill(src[2:n]) {
			if !bytes.Equal(src[:n], b) {
				t.Fatalf("% x: remarshalled to % x", src[:n], b)
			}
			return
		}
		if len(b) > n {
			t.Fatalf("% x: remarshalled to longer % x", src[:n], b)
		}
		var ra tpdu.Address
		rn, err := ra.UnmarshalBinary(b)
		if err != nil || rn != len(b) || ra != a {
			t.Fatalf("% x: %+v remarshalled to % x, decoded as %+v, %v", src[:n], a, b, ra, err)
		}
	})
}
