// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package tpdu_test

import (
	"bytes"
	"testing"

	"github.com/gomaja/go-sms/encoding/gsm7/charset"
	"github.com/gomaja/go-sms/encoding/tpdu"
	"github.com/gomaja/go-sms/encoding/ucs2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUserDataHeaderMarshalBinary(t *testing.T) {
	patterns := []struct {
		name string
		in   tpdu.UserDataHeader
		out  []byte
		err  error
	}{
		{"nil",
			nil,
			nil,
			nil,
		},
		{"empty",
			tpdu.UserDataHeader{},
			[]byte{0},
			nil,
		},
		{"one",
			tpdu.UserDataHeader{
				tpdu.InformationElement{ID: 1, Data: []byte{1, 2, 3}},
			},
			[]byte{5, 1, 3, 1, 2, 3},
			nil,
		},
		{"three",
			tpdu.UserDataHeader{
				tpdu.InformationElement{ID: 1, Data: []byte{1, 2, 3}},
				tpdu.InformationElement{ID: 1, Data: []byte{5, 6, 7}},
				tpdu.InformationElement{ID: 2, Data: []byte{1, 2, 3}},
			},
			[]byte{15, 1, 3, 1, 2, 3, 1, 3, 5, 6, 7, 2, 3, 1, 2, 3}, nil},
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

func TestUserDataHeaderUnmarshalBinary(t *testing.T) {
	patterns := []struct {
		name string
		in   []byte
		out  tpdu.UserDataHeader
		n    int
		err  error
	}{
		{"one",
			[]byte{5, 1, 3, 1, 2, 3},
			tpdu.UserDataHeader{
				tpdu.InformationElement{ID: 1, Data: []byte{1, 2, 3}},
			},
			6,
			nil,
		},
		{"three",
			[]byte{15, 1, 3, 1, 2, 3, 1, 3, 5, 6, 7, 2, 3, 1, 2, 3},
			tpdu.UserDataHeader{
				tpdu.InformationElement{ID: 1, Data: []byte{1, 2, 3}},
				tpdu.InformationElement{ID: 1, Data: []byte{5, 6, 7}},
				tpdu.InformationElement{ID: 2, Data: []byte{1, 2, 3}},
			},
			16,
			nil,
		},
		{"short udhl",
			nil,
			tpdu.UserDataHeader{},
			0,
			tpdu.NewDecodeError("udhl", 0, tpdu.ErrUnderflow),
		},
		{"short udh",
			[]byte{5, 1, 3, 1, 2},
			tpdu.UserDataHeader{},
			1,
			tpdu.NewDecodeError("ie", 1, tpdu.ErrUnderflow),
		},
		{"empty",
			[]byte{0},
			tpdu.UserDataHeader{},
			1,
			nil,
		},
		{"empty with sm",
			[]byte{0, 0x41, 0x42},
			tpdu.UserDataHeader{},
			1,
			nil,
		},
		{"empty ie",
			[]byte{2, 1, 0, 0x41},
			tpdu.UserDataHeader{
				tpdu.InformationElement{ID: 1},
			},
			3,
			nil,
		},
		// 3GPP TS 23.040 Section 9.2.3.24: "If the length of the User Data
		// Header is such that there are too few or too many octets in the
		// final Information Element then the whole User Data Header shall be
		// ignored."
		{"short ie",
			[]byte{1, 1},
			tpdu.UserDataHeader{},
			2,
			nil,
		},
		{"short ied",
			[]byte{3, 1, 3, 1, 2},
			tpdu.UserDataHeader{},
			4,
			nil,
		},
		{"ied overruns udhl",
			[]byte{3, 0, 5, 1, 2, 3, 4, 5},
			tpdu.UserDataHeader{},
			4,
			nil,
		},
		{"ied overruns udhl into sm",
			[]byte{2, 0, 3, 7, 2, 1, 0x41},
			tpdu.UserDataHeader{},
			3,
			nil,
		},
		{"dangling octet",
			[]byte{5, 0, 2, 1, 2, 0x41},
			tpdu.UserDataHeader{},
			6,
			nil,
		},
		{"dangling octet after ies",
			[]byte{
				9, 0, 3, 1, 2, 1, 0x24, 1, 1, 0x61, 0x62,
			},
			tpdu.UserDataHeader{},
			10,
			nil,
		},
	}
	for _, p := range patterns {
		f := func(t *testing.T) {
			a := tpdu.UserDataHeader{{ID: 0xff}}
			if p.err != nil {
				a = tpdu.UserDataHeader{}
			}
			n, err := a.UnmarshalBinary(p.in)
			assert.Equal(t, p.err, err)
			assert.Equal(t, p.n, n)
			assert.Equal(t, p.out, a)
		}
		t.Run(p.name, f)
	}
}

func TestUserDataHeaderIgnored(t *testing.T) {
	// An SMS-DELIVER with 8-bit data whose UDH (UDHL 2) contains a
	// concatenation IE claiming 3 octets, so running into the SM. The whole
	// UDH must be ignored, the SM starts after the UDHL octets, and the TPDU
	// decodes.
	b := []byte{
		0x44,                                           // first octet, UDHI set
		0x0b, 0x91, 0x16, 0x04, 0x89, 0x56, 0x26, 0xf9, // OA
		0x00,                                     // PID
		0x04,                                     // DCS 8-bit
		0x71, 0x80, 0x13, 0x11, 0x12, 0x45, 0x23, // SCTS
		0x07,                   // UDL
		0x02, 0x00, 0x03, 0x07, // UDH
		0x02, 0x01, 0x41, // SM
	}
	var pdu tpdu.TPDU
	err := pdu.UnmarshalBinary(b)
	require.Nil(t, err)
	assert.Equal(t, tpdu.UserDataHeader{}, pdu.UDH)
	assert.Equal(t, tpdu.UserData{0x07, 0x02, 0x01, 0x41}, pdu.UD)
	assert.True(t, pdu.IsSingleSegment())

	// The same with 7-bit data, where the data of the final IE is cut short
	// by the UDHL, and the UDH (5 octets) is followed by 2 fill bits and
	// "hello".
	b = []byte{
		0x44,                                           // first octet, UDHI set
		0x0b, 0x91, 0x16, 0x04, 0x89, 0x56, 0x26, 0xf9, // OA
		0x00,                                     // PID
		0x00,                                     // DCS 7-bit
		0x71, 0x80, 0x13, 0x11, 0x12, 0x45, 0x23, // SCTS
		0x0b,                         // UDL (septets)
		0x04, 0x00, 0x03, 0x01, 0x02, // UDH
		0xa0, 0xcb, 0x6c, 0xf6, 0x1b, // fill bits and SM
	}
	pdu = tpdu.TPDU{}
	err = pdu.UnmarshalBinary(b)
	require.Nil(t, err)
	assert.Equal(t, tpdu.UserDataHeader{}, pdu.UDH)
	assert.Equal(t, tpdu.UserData("hello"), pdu.UD)
}

func TestUserDataHeaderEmptyRoundTrip(t *testing.T) {
	// A UDH that is present (TP-UDHI set) but empty (UDHL 0) must survive
	// unmarshalling and marshalling, as must the SM that follows it.
	patterns := []struct {
		name string
		in   []byte
		ud   tpdu.UserData
	}{
		{"8bit",
			[]byte{
				0x44,                                           // first octet, UDHI set
				0x0b, 0x91, 0x16, 0x04, 0x89, 0x56, 0x26, 0xf9, // OA
				0x00,                                     // PID
				0x04,                                     // DCS 8-bit
				0x71, 0x80, 0x13, 0x11, 0x12, 0x45, 0x23, // SCTS
				0x03, 0x00, 0x41, 0x42, // UDL, UDHL and SM
			},
			tpdu.UserData("AB"),
		},
		{"7bit",
			[]byte{
				0x44,                                           // first octet, UDHI set
				0x0b, 0x91, 0x16, 0x04, 0x89, 0x56, 0x26, 0xf9, // OA
				0x00,                                     // PID
				0x00,                                     // DCS 7-bit
				0x71, 0x80, 0x13, 0x11, 0x12, 0x45, 0x23, // SCTS
				0x04, 0x00, 0x00, 0x3a, 0x0d, // UDL, UDHL, fill bits and SM
			},
			tpdu.UserData("hi"),
		},
	}
	for _, p := range patterns {
		f := func(t *testing.T) {
			var pdu tpdu.TPDU
			err := pdu.UnmarshalBinary(p.in)
			require.Nil(t, err)
			assert.True(t, pdu.UDHI())
			assert.Equal(t, tpdu.UserDataHeader{}, pdu.UDH)
			assert.Equal(t, p.ud, pdu.UD)
			b, err := pdu.MarshalBinary()
			require.Nil(t, err)
			assert.Equal(t, p.in, b)
		}
		t.Run(p.name, f)
	}
}

// ieTiles reports whether the IEs in the UDH body, i.e. the octets following
// the UDHL, exactly fill it.
func ieTiles(body []byte) bool {
	for len(body) > 0 {
		if len(body) < 2 || len(body) < 2+int(body[1]) {
			return false
		}
		body = body[2+int(body[1]):]
	}
	return true
}

// FuzzUserDataHeaderUnmarshalBinary checks that any UDH either fails to
// unmarshal because the UDHL runs past the data, or consumes exactly the UDHL
// octets and re-marshals to them. The exception is a header whose IEs do not
// exactly fill the UDHL, which must be ignored as a whole (3GPP TS 23.040
// Section 9.2.3.24), and so re-marshals as an empty header.
func FuzzUserDataHeaderUnmarshalBinary(f *testing.F) {
	for _, seed := range [][]byte{
		{},
		{0},
		{0, 0x41},
		{5, 1, 3, 1, 2, 3},
		{15, 1, 3, 1, 2, 3, 1, 3, 5, 6, 7, 2, 3, 1, 2, 3},
		{5, 1, 3, 1, 2},
		{1, 1},
		{3, 1, 3, 1, 2},
		{2, 0, 3, 7, 2, 1, 0x41},
		{5, 0, 2, 1, 2, 0x41},
		{6, 0x25, 1, 1, 0x24, 1, 13, 0x41},
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, src []byte) {
		orig := append([]byte(nil), src...)
		var udh tpdu.UserDataHeader
		n, err := udh.UnmarshalBinary(src)
		if err != nil {
			if len(src) > 0 && int(src[0]) < len(src) {
				t.Fatalf("% x: unexpected error %v", orig, err)
			}
			return
		}
		if n != int(orig[0])+1 {
			t.Fatalf("% x: read %d octets", orig, n)
		}
		if udh == nil {
			t.Fatalf("% x: nil header", orig)
		}
		// the IEs must not alias src
		for i := range src {
			src[i] ^= 0xff
		}
		b, err := udh.MarshalBinary()
		if err != nil {
			t.Fatalf("% x: marshal error %v", orig, err)
		}
		if !ieTiles(orig[1:n]) {
			if len(udh) != 0 {
				t.Fatalf("% x: malformed header not ignored: %v", orig, udh)
			}
			return
		}
		if !bytes.Equal(orig[:n], b) {
			t.Fatalf("% x: remarshalled to % x", orig[:n], b)
		}
	})
}

func TestUserDataHeaderIE(t *testing.T) {
	u := tpdu.UserDataHeader{
		tpdu.InformationElement{ID: 1, Data: []byte{1, 2, 3}},
		tpdu.InformationElement{ID: 1, Data: []byte{5, 6, 7}},
		tpdu.InformationElement{ID: 2, Data: []byte{1, 2, 3}},
	}
	_, ok := u.IE(0)
	assert.False(t, ok)
	i, ok := u.IE(1)
	assert.True(t, ok)
	assert.Equal(t, uint8(1), i.ID)
	assert.Equal(t, u[1].Data, i.Data)
}

func TestUserDataHeaderIEs(t *testing.T) {
	u := tpdu.UserDataHeader{
		tpdu.InformationElement{ID: 1, Data: []byte{1, 2, 3}},
		tpdu.InformationElement{ID: 1, Data: []byte{5, 6, 7}},
		tpdu.InformationElement{ID: 2, Data: []byte{1, 2, 3}},
	}
	i := u.IEs(0)
	assert.Nil(t, i)
	i = u.IEs(2)
	assert.Equal(t, 1, len(i))
	assert.Equal(t, uint8(2), i[0].ID)
	assert.Equal(t, u[2].Data, i[0].Data)
	i = u.IEs(1)
	assert.Equal(t, 2, len(i))
	assert.Equal(t, uint8(1), i[0].ID)
	assert.Equal(t, u[0].Data, i[0].Data)
	assert.Equal(t, uint8(1), i[1].ID)
	assert.Equal(t, u[1].Data, i[1].Data)
}

type concatTestPattern struct {
	name     string
	udh      tpdu.UserDataHeader
	mref     int
	segments int
	seqno    int
	ok       bool
}

func TestConcatInfo(t *testing.T) {
	patterns := []concatTestPattern{
		{"empty",
			tpdu.UserDataHeader{},
			0,
			0,
			0,
			false,
		},
		{"empty data",
			tpdu.UserDataHeader{
				tpdu.InformationElement{ID: 0, Data: []byte{}},
			},
			0,
			0,
			0,
			false,
		},
		{"nil data",
			tpdu.UserDataHeader{
				tpdu.InformationElement{ID: 0, Data: nil},
			},
			0,
			0,
			0,
			false,
		},
		{"concat8",
			tpdu.UserDataHeader{
				tpdu.InformationElement{ID: 0, Data: []byte{3, 2, 1}},
			},
			3,
			2,
			1,
			true,
		},
		{"id 1",
			tpdu.UserDataHeader{
				tpdu.InformationElement{ID: 1, Data: []byte{3, 2, 1}},
			},
			0,
			0,
			0,
			false,
		},
		{"concat16",
			tpdu.UserDataHeader{
				tpdu.InformationElement{ID: 8, Data: []byte{4, 3, 2, 1}},
			},
			1027,
			2,
			1,
			true,
		},
		{"short concat8",
			tpdu.UserDataHeader{
				tpdu.InformationElement{ID: 0, Data: []byte{2, 1}},
			},
			0,
			0,
			0,
			false,
		},
		{"short concat16",
			tpdu.UserDataHeader{
				tpdu.InformationElement{ID: 8, Data: []byte{3, 2, 1}},
			},
			0,
			0,
			0,
			false,
		},
	}
	for _, p := range patterns {
		f := func(t *testing.T) {
			segments, seqno, mref, ok := p.udh.ConcatInfo()
			assert.Equal(t, p.ok, ok)
			assert.Equal(t, p.segments, segments)
			assert.Equal(t, p.seqno, seqno)
			assert.Equal(t, p.mref, mref)
		}
		t.Run(p.name, f)
	}
}

func TestConcatInfo8(t *testing.T) {
	patterns := []concatTestPattern{
		{"empty",
			tpdu.UserDataHeader{},
			0,
			0,
			0,
			false,
		},
		{"empty data",
			tpdu.UserDataHeader{
				tpdu.InformationElement{ID: 0, Data: []byte{}},
			},
			0,
			0,
			0,
			false,
		},
		{"nil data",
			tpdu.UserDataHeader{
				tpdu.InformationElement{ID: 0, Data: nil},
			},
			0,
			0,
			0,
			false,
		},
		{"concat8",
			tpdu.UserDataHeader{
				tpdu.InformationElement{ID: 0, Data: []byte{3, 2, 1}},
			},
			3,
			2,
			1,
			true,
		},
		{"id 1",
			tpdu.UserDataHeader{
				tpdu.InformationElement{ID: 1, Data: []byte{3, 2, 1}},
			},
			0,
			0,
			0,
			false,
		},
		{"concat16",
			tpdu.UserDataHeader{
				tpdu.InformationElement{ID: 8, Data: []byte{4, 3, 2, 1}},
			},
			0,
			0,
			0,
			false,
		},
		{"short concat8",
			tpdu.UserDataHeader{
				tpdu.InformationElement{ID: 0, Data: []byte{2, 1}},
			},
			0,
			0,
			0,
			false,
		},
		{"short concat16",
			tpdu.UserDataHeader{
				tpdu.InformationElement{ID: 8, Data: []byte{3, 2, 1}},
			},
			0,
			0,
			0,
			false,
		},
	}
	for _, p := range patterns {
		f := func(t *testing.T) {
			segments, seqno, mref, ok := p.udh.ConcatInfo8()
			assert.Equal(t, p.ok, ok)
			assert.Equal(t, p.segments, segments)
			assert.Equal(t, p.seqno, seqno)
			assert.Equal(t, p.mref, mref)
		}
		t.Run(p.name, f)
	}
}

func TestConcatInfo16(t *testing.T) {
	patterns := []concatTestPattern{
		{"empty",
			tpdu.UserDataHeader{},
			0,
			0,
			0,
			false,
		},
		{"empty data",
			tpdu.UserDataHeader{
				tpdu.InformationElement{ID: 0, Data: []byte{}},
			},
			0,
			0,
			0,
			false,
		},
		{"nil data",
			tpdu.UserDataHeader{
				tpdu.InformationElement{ID: 0, Data: nil},
			},
			0,
			0,
			0,
			false,
		},
		{"concat8",
			tpdu.UserDataHeader{
				tpdu.InformationElement{ID: 0, Data: []byte{3, 2, 1}},
			},
			0,
			0,
			0,
			false,
		},
		{"id 1",
			tpdu.UserDataHeader{
				tpdu.InformationElement{ID: 1, Data: []byte{3, 2, 1}},
			},
			0,
			0,
			0,
			false,
		},
		{"concat16",
			tpdu.UserDataHeader{
				tpdu.InformationElement{ID: 8, Data: []byte{4, 3, 2, 1}},
			},
			1027,
			2,
			1,
			true,
		},
		{"short concat8",
			tpdu.UserDataHeader{
				tpdu.InformationElement{ID: 0, Data: []byte{2, 1}},
			},
			0,
			0,
			0,
			false,
		},
		{"short concat16",
			tpdu.UserDataHeader{
				tpdu.InformationElement{ID: 8, Data: []byte{3, 2, 1}},
			},
			0,
			0,
			0,
			false,
		},
	}
	for _, p := range patterns {
		f := func(t *testing.T) {
			segments, seqno, mref, ok := p.udh.ConcatInfo16()
			assert.Equal(t, p.ok, ok)
			assert.Equal(t, p.segments, segments)
			assert.Equal(t, p.seqno, seqno)
			assert.Equal(t, p.mref, mref)
		}
		t.Run(p.name, f)

	}
}

func TestDecodeUserData(t *testing.T) {
	// Also tests NewUDDecoder, AddLockingCharset and AddShiftCharset
	patterns := []struct {
		name    string
		ud      tpdu.UserData
		udh     tpdu.UserDataHeader
		alpha   tpdu.Alphabet
		options []tpdu.UDDecodeOption
		msg     []byte
		err     error
	}{
		{"empty",
			nil,
			nil,
			0,
			nil,
			nil,
			nil},
		{"message 7bit",
			[]byte("message\x10"),
			nil,
			tpdu.Alpha7Bit,
			nil,
			[]byte("messageΔ"),
			nil,
		},
		{"message reserved",
			[]byte("message\x10"),
			nil,
			tpdu.AlphaReserved,
			nil,
			[]byte("messageΔ"),
			nil,
		},
		{"message 7bit esc",
			[]byte("message\x1b"),
			nil,
			tpdu.Alpha7Bit,
			nil,
			[]byte("message "),
			nil,
		},
		{"message 7bit locking",
			[]byte("\x01\x02\x03"),
			tpdu.UserDataHeader{
				tpdu.InformationElement{ID: tpdu.IEINationalLanguageLockingShift, Data: []byte{byte(charset.Kannada)}},
			},
			tpdu.Alpha7Bit,
			[]tpdu.UDDecodeOption{tpdu.WithLockingCharset(charset.Kannada)},
			[]byte("\u0c82\u0c83\u0c85"),
			nil,
		},
		{"message 7bit shift", []byte("\x1b\x1e\x1b\x1f\x1b\x20"),
			tpdu.UserDataHeader{
				tpdu.InformationElement{ID: tpdu.IEINationalLanguageSingleShift, Data: []byte{byte(charset.Kannada)}},
			},
			tpdu.Alpha7Bit,
			[]tpdu.UDDecodeOption{tpdu.WithShiftCharset(charset.Kannada)},
			[]byte("\u0ce8\u0ce9\u0cea"),
			nil,
		},
		{"message 8bit",
			[]byte("message\x1b"),
			nil,
			tpdu.Alpha8Bit,
			nil,
			[]byte("message\x1b"),
			nil,
		},
		{"euro",
			[]byte("\x1be"),
			nil,
			tpdu.Alpha7Bit,
			nil,
			[]byte("€"),
			nil,
		},
		{"grin",
			[]byte{0xd8, 0x3d, 0xde, 0x01},
			nil,
			tpdu.AlphaUCS2,
			nil,
			[]byte("😁"),
			nil,
		},
		// repeat the GSM7 Kannada tests without charset to force decoding to
		// fallback to default
		{"message 7bit locking defaulted",
			[]byte("\x01\x02\x03"),
			tpdu.UserDataHeader{
				tpdu.InformationElement{ID: tpdu.IEINationalLanguageLockingShift, Data: []byte{byte(charset.Kannada)}},
			},
			tpdu.Alpha7Bit,
			nil,
			[]byte("£$¥"),
			nil,
		},
		{"message 7bit shift defaulted",
			[]byte("\x1b\x1e\x1b\x1f\x1b\x20"),
			tpdu.UserDataHeader{
				tpdu.InformationElement{ID: tpdu.IEINationalLanguageSingleShift, Data: []byte{byte(charset.Kannada)}},
			},
			tpdu.Alpha7Bit,
			nil,
			[]byte("ßÉ "),
			nil,
		},
		// error tests
		{"dangling surrogate",
			[]byte{0xd8, 0x3d, 0xde, 0x01, 0xd8, 0x3d},
			nil,
			tpdu.AlphaUCS2,
			nil,
			[]byte("😁"), ucs2.ErrDanglingSurrogate([]byte{0xd8, 0x3d}),
		},
	}
	for _, p := range patterns {
		f := func(t *testing.T) {
			msg, err := tpdu.DecodeUserData(p.ud, p.udh, p.alpha, p.options...)
			require.Equal(t, p.err, err)
			assert.Equal(t, p.msg, msg)
		}
		t.Run(p.name, f)
	}
}

func TestDecodeUserDataAllCharsets(t *testing.T) {
	// Tests decode with AddAllCharsets
	patterns := []struct {
		name    string
		ud      tpdu.UserData
		udh     tpdu.UserDataHeader
		msg     []byte
		options []tpdu.UDDecodeOption
		err     error
	}{
		{"empty",
			nil,
			nil,
			nil,
			nil,
			nil,
		},
		{"message 7bit",
			[]byte("message\x10"),
			nil,
			[]byte("messageΔ"),
			nil,
			nil,
		},
		{"message reserved",
			[]byte("message\x10"),
			nil,
			[]byte("messageΔ"),
			nil,
			nil,
		},
		{"message 7bit esc",
			[]byte("message\x1b"),
			nil,
			[]byte("message "),
			nil,
			nil,
		},
		{"message 7bit locking all cs",
			[]byte("\x01\x02\x03"),
			tpdu.UserDataHeader{
				tpdu.InformationElement{ID: tpdu.IEINationalLanguageLockingShift, Data: []byte{byte(charset.Kannada)}},
			},
			[]byte("\u0c82\u0c83\u0c85"),
			[]tpdu.UDDecodeOption{
				tpdu.WithAllCharsets,
			},
			nil,
		},
		{"message 7bit locking kannada",
			[]byte("\x01\x02\x03"),
			tpdu.UserDataHeader{
				tpdu.InformationElement{ID: tpdu.IEINationalLanguageLockingShift, Data: []byte{byte(charset.Kannada)}},
			},
			[]byte("\u0c82\u0c83\u0c85"),
			[]tpdu.UDDecodeOption{
				tpdu.WithCharset(charset.Kannada),
			},
			nil,
		},
		{"message 7bit shift all cs",
			[]byte("\x1b\x1e\x1b\x1f\x1b\x20"),
			tpdu.UserDataHeader{
				tpdu.InformationElement{ID: tpdu.IEINationalLanguageSingleShift, Data: []byte{byte(charset.Kannada)}},
			},
			[]byte("\u0ce8\u0ce9\u0cea"),
			[]tpdu.UDDecodeOption{
				tpdu.WithAllCharsets,
			},
			nil,
		},
		{"message 7bit shift kannada",
			[]byte("\x1b\x1e\x1b\x1f\x1b\x20"),
			tpdu.UserDataHeader{
				tpdu.InformationElement{ID: tpdu.IEINationalLanguageSingleShift, Data: []byte{byte(charset.Kannada)}},
			},
			[]byte("\u0ce8\u0ce9\u0cea"),
			[]tpdu.UDDecodeOption{
				tpdu.WithCharset(charset.Kannada),
			},
			nil,
		},
		{"euro",
			[]byte("\x1be"),
			nil,
			[]byte("€"),
			nil,
			nil,
		},
	}
	for _, p := range patterns {
		f := func(t *testing.T) {
			msg, err := tpdu.DecodeUserData(p.ud, p.udh, tpdu.Alpha7Bit, p.options...)
			assert.Equal(t, p.err, err)
			assert.Equal(t, p.msg, msg)
		}
		t.Run(p.name, f)
	}
}

func TestEncodeUserData(t *testing.T) {
	// Also tests WithAllCharsets, WithCharset, WithLockingCharset and
	// WithShiftCharset.
	patterns := []struct {
		name    string
		ud      tpdu.UserData
		udh     tpdu.UserDataHeader
		alpha   tpdu.Alphabet
		options []tpdu.UDEncodeOption
		msg     []byte
	}{
		{"empty",
			nil,
			nil,
			0,
			nil,
			nil,
		},
		{"message 7bit",
			[]byte("message\x10"),
			nil,
			tpdu.Alpha7Bit,
			nil,
			[]byte("messageΔ"),
		},
		{"message 7bit all cs",
			[]byte("message\x10"),
			nil,
			tpdu.Alpha7Bit,
			[]tpdu.UDEncodeOption{
				tpdu.WithAllCharsets,
			},
			[]byte("messageΔ"),
		},
		{"message 7bit locking all cs",
			[]byte("\x01\x02\x03"),
			tpdu.UserDataHeader{
				tpdu.InformationElement{ID: tpdu.IEINationalLanguageLockingShift, Data: []byte{byte(charset.Kannada)}},
			},
			tpdu.Alpha7Bit,
			[]tpdu.UDEncodeOption{
				tpdu.WithAllCharsets,
			},
			[]byte("\u0c82\u0c83\u0c85"),
		},
		{"message 7bit shift all cs",
			[]byte("\x1b\x1e\x1b\x1f\x1b\x20"),
			tpdu.UserDataHeader{
				tpdu.InformationElement{ID: tpdu.IEINationalLanguageSingleShift, Data: []byte{byte(charset.Kannada)}},
			},
			tpdu.Alpha7Bit,
			[]tpdu.UDEncodeOption{
				tpdu.WithAllCharsets,
			},
			[]byte("\u0ce8\u0ce9\u0cea"),
		},
		{"message 7bit kannada",
			[]byte("\x01\x02\x03"),
			tpdu.UserDataHeader{
				tpdu.InformationElement{ID: tpdu.IEINationalLanguageLockingShift, Data: []byte{byte(charset.Kannada)}},
			},
			tpdu.Alpha7Bit,
			[]tpdu.UDEncodeOption{
				tpdu.WithCharset(charset.Kannada),
			},
			[]byte("\u0c82\u0c83\u0c85"),
		},
		{"message 7bit locking kannada",
			[]byte("\x01\x02\x03"),
			tpdu.UserDataHeader{
				tpdu.InformationElement{ID: tpdu.IEINationalLanguageLockingShift, Data: []byte{byte(charset.Kannada)}},
			},
			tpdu.Alpha7Bit,
			[]tpdu.UDEncodeOption{
				tpdu.WithLockingCharset(charset.Kannada),
			},
			[]byte("\u0c82\u0c83\u0c85"),
		},
		{"message 7bit shift kannada",
			[]byte("\x1b\x1e\x1b\x1f\x1b\x20"),
			tpdu.UserDataHeader{
				tpdu.InformationElement{ID: tpdu.IEINationalLanguageSingleShift, Data: []byte{byte(charset.Kannada)}},
			},
			tpdu.Alpha7Bit,
			[]tpdu.UDEncodeOption{
				tpdu.WithShiftCharset(charset.Kannada),
			},
			[]byte("\u0ce8\u0ce9\u0cea"),
		},
		{"message 7bit locking and shift urdu",
			[]byte("hello \x07\x1b\x2a"),
			tpdu.UserDataHeader{
				tpdu.InformationElement{ID: tpdu.IEINationalLanguageLockingShift, Data: []byte{byte(charset.Urdu)}},
				tpdu.InformationElement{ID: tpdu.IEINationalLanguageSingleShift, Data: []byte{byte(charset.Urdu)}},
			},
			tpdu.Alpha7Bit,
			[]tpdu.UDEncodeOption{
				tpdu.WithLockingCharset(charset.Urdu),
				tpdu.WithShiftCharset(charset.Urdu),
			},
			[]byte("hello ت؎"),
		},
		{"euro",
			[]byte("\x1be"),
			nil,
			tpdu.Alpha7Bit,
			nil,
			[]byte("€"),
		},
		{"grin",
			[]byte{0xd8, 0x3d, 0xde, 0x01},
			nil,
			tpdu.AlphaUCS2,
			nil,
			[]byte("😁"),
		},
		{"grin all cs",
			[]byte{0xd8, 0x3d, 0xde, 0x01},
			nil,
			tpdu.AlphaUCS2,
			[]tpdu.UDEncodeOption{
				tpdu.WithAllCharsets,
			},
			[]byte("😁"),
		},
		// repeat the GSM7 Kannada tests without charset to force encoding to UCS2
		{"message ucs2 locking",
			[]byte{0x0c, 0x82, 0x0c, 0x83, 0x0c, 0x85},
			nil,
			tpdu.AlphaUCS2,
			nil,
			[]byte("\u0c82\u0c83\u0c85"),
		},
		{"message ucs2 shift",
			[]byte{0x0c, 0xe8, 0x0c, 0xe9, 0x0c, 0xea},
			nil,
			tpdu.AlphaUCS2,
			nil,
			[]byte("\u0ce8\u0ce9\u0cea"),
		},
	}
	for _, p := range patterns {
		f := func(t *testing.T) {
			ud, udh, alpha := tpdu.EncodeUserData(p.msg, p.options...)
			assert.Equal(t, p.ud, ud)
			assert.Equal(t, p.udh, udh)
			assert.Equal(t, p.alpha, alpha)
		}
		t.Run(p.name, f)
	}
}

func TestNationalLanguageIEIWire(t *testing.T) {
	// 3GPP TS 23.040 Section 9.2.3.24 lists the IEIs in hex: 0x24 is the
	// National Language Single Shift and 0x25 the National Language Locking
	// Shift (0x18 and 0x19 are WVG objects).
	patterns := []struct {
		name    string
		msg     []byte
		options []tpdu.UDEncodeOption
		udh     []byte
	}{
		{"locking turkish",
			[]byte("ş"),
			[]tpdu.UDEncodeOption{tpdu.WithLockingCharset(charset.Turkish)},
			[]byte{0x03, 0x25, 0x01, 0x01},
		},
		{"shift urdu",
			[]byte("hello ؎"),
			[]tpdu.UDEncodeOption{tpdu.WithShiftCharset(charset.Urdu)},
			[]byte{0x03, 0x24, 0x01, 0x0d},
		},
		{"locking and shift urdu",
			[]byte("hello ت؎"),
			[]tpdu.UDEncodeOption{
				tpdu.WithLockingCharset(charset.Urdu),
				tpdu.WithShiftCharset(charset.Urdu),
			},
			[]byte{0x06, 0x25, 0x01, 0x0d, 0x24, 0x01, 0x0d},
		},
	}
	for _, p := range patterns {
		f := func(t *testing.T) {
			_, udh, alpha := tpdu.EncodeUserData(p.msg, p.options...)
			assert.Equal(t, tpdu.Alpha7Bit, alpha)
			b, err := udh.MarshalBinary()
			require.Nil(t, err)
			assert.Equal(t, p.udh, b)
		}
		t.Run(p.name, f)
	}
}

func TestNationalLanguageIEIDeliver(t *testing.T) {
	// SMS-DELIVER from +61409865629 carrying a National Language Locking
	// Shift IE (0x25) selecting Turkish (NLI 1), followed by 3 fill bits and
	// the septet 0x1d, which is 'ş' in the Turkish locking shift table.
	b := []byte{
		0x44,                                           // first octet, UDHI set
		0x0b, 0x91, 0x16, 0x04, 0x89, 0x56, 0x26, 0xf9, // OA
		0x00,                                     // PID
		0x00,                                     // DCS
		0x71, 0x80, 0x13, 0x11, 0x12, 0x45, 0x23, // SCTS
		0x06,                               // UDL (septets)
		0x03, 0x25, 0x01, 0x01, 0xe8, 0x00, // UDH, fill bits and SM
	}
	var pdu tpdu.TPDU
	err := pdu.UnmarshalBinary(b)
	require.Nil(t, err)
	msg, err := tpdu.DecodeUserData(pdu.UD, pdu.UDH, tpdu.Alpha7Bit,
		tpdu.WithLockingCharset(charset.Turkish))
	require.Nil(t, err)
	assert.Equal(t, "ş", string(msg))
}
