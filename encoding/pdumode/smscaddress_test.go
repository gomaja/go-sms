// SPDX-License-Identifier: MIT

package pdumode_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/gomaja/go-sms/encoding/pdumode"
	"github.com/gomaja/go-sms/encoding/semioctet"
	"github.com/gomaja/go-sms/encoding/tpdu"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSMSCAddressMarshalBinary(t *testing.T) {
	patterns := []struct {
		name string
		in   pdumode.SMSCAddress
		out  []byte
		err  error
	}{
		{
			"empty",
			pdumode.SMSCAddress{},
			[]byte{0},
			nil,
		},
		{
			"number",
			pdumode.SMSCAddress{
				Address: tpdu.Address{Addr: "61409865629", TOA: 0x91},
			},
			[]byte{7, 0x91, 0x16, 0x04, 0x89, 0x56, 0x26, 0xf9},
			nil,
		},
		{
			"present number",
			pdumode.SMSCAddress{
				Address: tpdu.Address{Addr: "61409865629", TOA: 0x91},
				Present: true,
			},
			[]byte{7, 0x91, 0x16, 0x04, 0x89, 0x56, 0x26, 0xf9},
			nil,
		},
		{
			"only toa",
			pdumode.SMSCAddress{
				Address: tpdu.Address{TOA: 0x91},
				Present: true,
			},
			[]byte{1, 0x91},
			nil,
		},
		{
			"absent with toa",
			pdumode.SMSCAddress{
				Address: tpdu.Address{TOA: 0x91},
			},
			[]byte{0},
			nil,
		},
		{
			"number alphabet",
			pdumode.SMSCAddress{
				Address: tpdu.Address{Addr: "0123456789*#abc", TOA: 0x91},
			},
			[]byte{9, 0x91, 0x10, 0x32, 0x54, 0x76, 0x98, 0xba, 0xdc, 0xfe},
			nil,
		},
		{
			"alpha",
			pdumode.SMSCAddress{
				Address: tpdu.Address{Addr: "messages", TOA: 0xd1},
			},
			nil,
			tpdu.EncodeError("addr", semioctet.ErrInvalidDigit(0x6d)),
		},
		{
			"invalid number",
			pdumode.SMSCAddress{
				Address: tpdu.Address{Addr: "6140f98656", TOA: 0x91},
			},
			nil,
			tpdu.EncodeError("addr", semioctet.ErrInvalidDigit('f')),
		},
		{
			"max length",
			pdumode.SMSCAddress{
				Address: tpdu.Address{Addr: "12345678901234567890", TOA: 0x91},
			},
			[]byte{
				11, 0x91, 0x21, 0x43, 0x65, 0x87, 0x09, 0x21, 0x43, 0x65,
				0x87, 0x09,
			},
			nil,
		},
		{
			"overlength",
			pdumode.SMSCAddress{
				Address: tpdu.Address{Addr: "123456789012345678901", TOA: 0x91},
			},
			nil,
			tpdu.EncodeError("addr", tpdu.ErrOverlength),
		},
		{
			"length octet wrap",
			pdumode.SMSCAddress{
				Address: tpdu.Address{Addr: strings.Repeat("1", 510), TOA: 0x91},
			},
			nil,
			tpdu.EncodeError("addr", tpdu.ErrOverlength),
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

func TestSMSCAddressUnmarshalBinary(t *testing.T) {
	patterns := []struct {
		name string
		in   []byte
		out  pdumode.SMSCAddress
		n    int
		err  error
	}{
		{
			"nil",
			nil,
			pdumode.SMSCAddress{},
			0,
			tpdu.NewDecodeError("length", 0, tpdu.ErrUnderflow),
		},
		{
			"only toa",
			[]byte{1, 0},
			pdumode.SMSCAddress{Present: true},
			2,
			nil,
		},
		{
			"only international toa",
			[]byte{1, 0x91, 0x01},
			pdumode.SMSCAddress{
				Address: tpdu.Address{TOA: 0x91},
				Present: true,
			},
			2,
			nil,
		},
		{
			"only fill",
			[]byte{2, 0x91, 0xff},
			pdumode.SMSCAddress{
				Address: tpdu.Address{TOA: 0x91},
				Present: true,
			},
			3,
			nil,
		},
		{
			"number",
			[]byte{7, 0x91, 0x16, 0x04, 0x89, 0x56, 0x26, 0xf9},
			pdumode.SMSCAddress{
				Address: tpdu.Address{Addr: "61409865629", TOA: 0x91},
				Present: true,
			},
			8,
			nil,
		},
		{
			"number alphabet",
			[]byte{9, 0x91, 0x10, 0x32, 0x54, 0x76, 0x98, 0xba, 0xdc, 0xfe},
			pdumode.SMSCAddress{
				Address: tpdu.Address{Addr: "0123456789*#abc", TOA: 0x91},
				Present: true,
			},
			10,
			nil,
		},
		{
			"alpha",
			[]byte{8, 0xd1, 0xED, 0xF2, 0x7C, 0x1E, 0x3E, 0x97, 0xE7},
			pdumode.SMSCAddress{
				Address: tpdu.Address{Addr: "bc2a7c1c3797c", TOA: 0xd1},
				Present: true,
			},
			9,
			nil,
		},
		{
			"zero length",
			[]byte{0},
			pdumode.SMSCAddress{},
			1,
			nil,
		},
		{
			"short number",
			[]byte{11, 0x91, 0x16, 0x04, 0x89, 0x56},
			pdumode.SMSCAddress{},
			6,
			tpdu.NewDecodeError("addr", 2, tpdu.ErrUnderflow),
		},
		{
			"short number pad",
			[]byte{11, 0x91, 0x16, 0x04, 0x89, 0x56, 0x97, 0xf7},
			pdumode.SMSCAddress{},
			8,
			tpdu.NewDecodeError("addr", 2, tpdu.ErrUnderflow),
		},
		{
			"max length",
			[]byte{
				11, 0x91, 0x21, 0x43, 0x65, 0x87, 0x09, 0x21, 0x43, 0x65,
				0x87, 0x09, 0x01,
			},
			pdumode.SMSCAddress{
				Address: tpdu.Address{Addr: "12345678901234567890", TOA: 0x91},
				Present: true,
			},
			12,
			nil,
		},
		{
			"overlength",
			[]byte{
				12, 0x91, 0x21, 0x43, 0x65, 0x87, 0x09, 0x21, 0x43, 0x65,
				0x87, 0x09, 0xf1,
			},
			pdumode.SMSCAddress{},
			1,
			tpdu.NewDecodeError("length", 0, tpdu.ErrOverlength),
		},
		{
			"overlength max",
			append([]byte{0xff, 0x91}, bytes.Repeat([]byte{0x11}, 254)...),
			pdumode.SMSCAddress{},
			1,
			tpdu.NewDecodeError("length", 0, tpdu.ErrOverlength),
		},
		{
			"underflow alpha",
			[]byte{5, 0xd1, 0xCF, 0xE5, 0x39},
			pdumode.SMSCAddress{},
			5,
			tpdu.NewDecodeError("addr", 2, tpdu.ErrUnderflow),
		},
		{
			"underflow toa",
			[]byte{1},
			pdumode.SMSCAddress{},
			1,
			tpdu.NewDecodeError("toa", 1, tpdu.ErrUnderflow),
		},
	}
	for _, p := range patterns {
		f := func(t *testing.T) {
			a := pdumode.SMSCAddress{}
			n, err := a.UnmarshalBinary(p.in)
			assert.Equal(t, p.err, err)
			assert.Equal(t, p.n, n)
			assert.Equal(t, p.out, a)

			// Decoding into a used value must give the same result.
			u := pdumode.SMSCAddress{
				Address: tpdu.Address{Addr: "639170000293", TOA: 0x91},
				Present: true,
			}
			n, err = u.UnmarshalBinary(p.in)
			assert.Equal(t, p.err, err)
			assert.Equal(t, p.n, n)
			assert.Equal(t, p.out, u)
		}
		t.Run(p.name, f)
	}
}

// TestSMSCAddressRoundTrip checks that a decoded address marshals back to
// the same field, and in particular that an address with a TOA but no digits
// is not turned into a zero length, which selects the default SMSC.
func TestSMSCAddressRoundTrip(t *testing.T) {
	patterns := []struct {
		name string
		in   []byte
		out  []byte
	}{
		{"default", []byte{0}, []byte{0}},
		{"only toa", []byte{1, 0x91}, []byte{1, 0x91}},
		{"only unknown toa", []byte{1, 0x81}, []byte{1, 0x81}},
		{"only zero toa", []byte{1, 0x00}, []byte{1, 0x00}},
		{"only fill", []byte{2, 0x91, 0xff}, []byte{1, 0x91}},
		{
			"number",
			[]byte{7, 0x91, 0x16, 0x04, 0x89, 0x56, 0x26, 0xf9},
			[]byte{7, 0x91, 0x16, 0x04, 0x89, 0x56, 0x26, 0xf9},
		},
	}
	for _, p := range patterns {
		f := func(t *testing.T) {
			var a pdumode.SMSCAddress
			n, err := a.UnmarshalBinary(p.in)
			require.NoError(t, err)
			require.Equal(t, len(p.in), n)
			b, err := a.MarshalBinary()
			require.NoError(t, err)
			assert.Equal(t, p.out, b)

			pdu, err := pdumode.UnmarshalBinary(append(p.in, 0x01, 0x00))
			require.NoError(t, err)
			b, err = pdu.MarshalBinary()
			require.NoError(t, err)
			assert.Equal(t, append(p.out, 0x01, 0x00), b)
		}
		t.Run(p.name, f)
	}
}
