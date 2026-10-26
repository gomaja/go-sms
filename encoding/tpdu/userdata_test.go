// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package tpdu_test

import (
	"bytes"
	"errors"
	"fmt"
	"testing"

	"github.com/gomaja/go-sms/encoding/gsm7/charset"
	"github.com/gomaja/go-sms/encoding/tpdu"
	"github.com/gomaja/go-sms/encoding/ucs2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncodeUserDataDefaultCharsetReset(t *testing.T) {
	for _, option := range []tpdu.UDEncodeOption{
		tpdu.WithAllCharsets,
		tpdu.WithCharset(charset.Turkish),
		tpdu.WithLockingCharset(charset.Turkish),
		tpdu.WithShiftCharset(charset.Turkish),
	} {
		ud, udh, alpha, err := tpdu.EncodeUserData([]byte("ı"), option, tpdu.WithDefaultCharset)
		require.NoError(t, err)
		assert.Equal(t, tpdu.AlphaUCS2, alpha)
		assert.Nil(t, udh)
		assert.Equal(t, tpdu.UserData{0x01, 0x31}, ud)
		_, udh, alpha, err = tpdu.EncodeUserData([]byte("ı"), tpdu.WithDefaultCharset, option)
		require.NoError(t, err)
		assert.Equal(t, tpdu.Alpha7Bit, alpha)
		assert.NotEmpty(t, udh)
	}
}

// 3GPP TS 23.038 Section 6.2.1.2.5: "the receiving entity shall ignore the
// IE" for an unsupported national language. 0x07 and ESC 0x69 encode ı in
// the Turkish tables (Annexes A.3.1 and A.2.1).
// WithAllCharsets adds the tables after those of earlier options, as
// WithCharset with every identifier does, so those are tried first: ç is in
// both the Portuguese and the Turkish locking shift tables (3GPP TS 23.038
// Annexes A.3.3 and A.3.1).
func TestEncodeUserDataAllCharsetsOrder(t *testing.T) {
	all := make([]int, 0, charset.Size)
	for nli := charset.Start; nli < charset.End; nli++ {
		all = append(all, nli)
	}
	for _, option := range []tpdu.UDEncodeOption{tpdu.WithAllCharsets, tpdu.WithCharset(all...)} {
		_, udh, alpha, err := tpdu.EncodeUserData([]byte("ç"), tpdu.WithCharset(charset.Portuguese), option)
		require.NoError(t, err)
		assert.Equal(t, tpdu.Alpha7Bit, alpha)
		assert.Equal(t, tpdu.UserDataHeader{{ID: 0x25, Data: []byte{byte(charset.Portuguese)}}}, udh)
	}
	_, udh, _, err := tpdu.EncodeUserData([]byte("ç"), tpdu.WithAllCharsets)
	require.NoError(t, err)
	assert.Equal(t, tpdu.UserDataHeader{{ID: 0x25, Data: []byte{byte(charset.Turkish)}}}, udh)
}

func TestDecodeUserDataDefaultCharsetReset(t *testing.T) {
	udh := tpdu.UserDataHeader{
		{ID: 0x25, Data: []byte{0x01}},
		{ID: 0x24, Data: []byte{0x01}},
	}
	patterns := []struct {
		option tpdu.UDDecodeOption
		want   string
	}{
		{tpdu.WithAllCharsets, "ıı"},
		{tpdu.WithCharset(charset.Turkish), "ıı"},
		{tpdu.WithLockingCharset(charset.Turkish), "ıi"},
		{tpdu.WithShiftCharset(charset.Turkish), "ìı"},
	}
	for _, p := range patterns {
		msg, err := tpdu.DecodeUserData(tpdu.UserData{0x07, 0x1b, 0x69}, udh, tpdu.Alpha7Bit,
			p.option, tpdu.WithDefaultCharset)
		require.NoError(t, err)
		assert.Equal(t, "ìi", string(msg))
		msg, err = tpdu.DecodeUserData(tpdu.UserData{0x07, 0x1b, 0x69}, udh, tpdu.Alpha7Bit,
			tpdu.WithDefaultCharset, p.option)
		require.NoError(t, err)
		assert.Equal(t, p.want, string(msg))
	}
}

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
		{"max",
			tpdu.UserDataHeader{
				tpdu.InformationElement{ID: 1, Data: make([]byte, 253)},
			},
			append([]byte{255, 1, 253}, make([]byte, 253)...),
			nil,
		},
		{"ied overlength",
			tpdu.UserDataHeader{
				tpdu.InformationElement{ID: 1, Data: make([]byte, 256)},
			},
			nil,
			tpdu.NewEncodeError("ied", tpdu.ErrOverlength),
		},
		{"udhl overlength",
			tpdu.UserDataHeader{
				tpdu.InformationElement{ID: 1, Data: make([]byte, 200)},
				tpdu.InformationElement{ID: 2, Data: make([]byte, 200)},
			},
			nil,
			tpdu.NewEncodeError("udhl", tpdu.ErrOverlength),
		},
		{"udhl overlength by one",
			tpdu.UserDataHeader{
				tpdu.InformationElement{ID: 1, Data: make([]byte, 254)},
			},
			nil,
			tpdu.NewEncodeError("udhl", tpdu.ErrOverlength),
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

func TestUserDataHeaderOverlengthTPDU(t *testing.T) {
	// The UDH error must reach the caller rather than a TPDU with wrapped
	// length octets.
	pdu, err := tpdu.NewSubmit()
	require.Nil(t, err)
	pdu.SetUDH(tpdu.UserDataHeader{
		tpdu.InformationElement{ID: 0x80, Data: make([]byte, 256)},
	})
	pdu.UD = []byte("hello")
	b, err := pdu.MarshalBinary()
	assert.Equal(t, tpdu.NewEncodeError("SmsSubmit.ud.udh.ied", tpdu.ErrOverlength), err)
	assert.Nil(t, b)
}

func TestInformationElementUnmarshalBinary(t *testing.T) {
	type pattern struct {
		name   string
		src    []byte
		data   []byte
		n      int
		field  string
		offset int
	}
	patterns := []pattern{
		{"empty", nil, nil, 0, "iei", 0},
		{"iei only", []byte{0x70}, nil, 0, "iedl", 1},
		{"empty data", []byte{0x70, 0}, nil, 2, "", 0},
		{"missing data", []byte{0x70, 1}, nil, 0, "ied", 2},
		{"trailing octet", []byte{0x70, 1, 0x41, 0x99}, []byte{0x41}, 3, "", 0},
	}
	for length := 0; length <= 255; length++ {
		var data []byte
		for i := 0; i < length; i++ {
			data = append(data, byte(i))
		}
		src := append([]byte{0x70, byte(length)}, data...)
		src = append(src, 0xee)
		patterns = append(patterns, pattern{fmt.Sprintf("length %d", length), src, data, 2 + length, "", 0})
		if length > 0 {
			patterns = append(patterns, pattern{fmt.Sprintf("short length %d", length), append([]byte(nil), src[:1+length]...), nil, 0, "ied", 2})
		}
	}
	for _, p := range patterns {
		t.Run(p.name, func(t *testing.T) {
			ie := tpdu.InformationElement{ID: 1, Data: []byte{2}}
			n, err := ie.UnmarshalBinary(p.src)
			assert.Equal(t, p.n, n)
			if p.field != "" {
				require.True(t, errors.Is(err, tpdu.ErrUnderflow))
				var de tpdu.DecodeError
				require.True(t, errors.As(err, &de))
				assert.Equal(t, p.field, de.Field)
				assert.Equal(t, p.offset, de.Offset)
				assert.Equal(t, tpdu.InformationElement{ID: 1, Data: []byte{2}}, ie)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, byte(0x70), ie.ID)
			assert.Equal(t, p.data, ie.Data)
			for i := range p.src {
				p.src[i] ^= 0xff
			}
			assert.Equal(t, p.data, ie.Data)
		})
	}
}

func FuzzInformationElementUnmarshalBinary(f *testing.F) {
	for _, seed := range [][]byte{nil, {0x70}, {0x70, 0}, {0x70, 1}, {0x70, 1, 0x41, 0x99},
		append([]byte{0x70, 255}, make([]byte, 255)...)} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, src []byte) {
		ie := tpdu.InformationElement{ID: 1, Data: []byte{2}}
		n, err := ie.UnmarshalBinary(src)
		if len(src) < 2 || len(src) < 2+int(src[1]) {
			require.Zero(t, n)
			require.True(t, errors.Is(err, tpdu.ErrUnderflow))
			var de tpdu.DecodeError
			require.True(t, errors.As(err, &de))
			field, offset := "ied", 2
			if len(src) == 0 {
				field, offset = "iei", 0
			} else if len(src) == 1 {
				field, offset = "iedl", 1
			}
			assert.Equal(t, field, de.Field)
			assert.Equal(t, offset, de.Offset)
			assert.Equal(t, tpdu.InformationElement{ID: 1, Data: []byte{2}}, ie)
			return
		}
		require.NoError(t, err)
		require.Equal(t, 2+int(src[1]), n)
		assert.Equal(t, src[0], ie.ID)
		data := append([]byte(nil), src[2:n]...)
		assert.Equal(t, data, ie.Data)
		want := append([]byte{byte(n)}, src[:n]...)
		for i := range src {
			src[i] ^= 0xff
		}
		assert.Equal(t, data, ie.Data)
		out, err := (tpdu.UserDataHeader{ie}).MarshalBinary()
		if n > 255 {
			require.ErrorIs(t, err, tpdu.ErrOverlength)
			return
		}
		require.NoError(t, err)
		assert.Equal(t, want, out)
	})
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
		{"ied borrows message octet", []byte{2, 0x70, 1, 0x41}, tpdu.UserDataHeader{}, 3, nil},
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
	err := unmarshalTPDU(t, &pdu, b)
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
	err = unmarshalTPDU(t, &pdu, b)
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
			err := unmarshalTPDU(t, &pdu, p.in)
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

// referenceUserDataHeader follows the UDHL and IE length fields in 3GPP TS
// 23.040 Section 9.2.3.24. An incomplete final IE discards the whole header.
func referenceUserDataHeader(src []byte) (tpdu.UserDataHeader, int, error) {
	if len(src) == 0 {
		return nil, 0, tpdu.NewDecodeError("udhl", 0, tpdu.ErrUnderflow)
	}
	end := 1 + int(src[0])
	if end > len(src) {
		return nil, 1, tpdu.NewDecodeError("ie", 1, tpdu.ErrUnderflow)
	}
	header := tpdu.UserDataHeader{}
	var data []byte
	var id byte
	remaining := -1
	for _, octet := range src[1:end] {
		switch remaining {
		case -1:
			id = octet
			remaining = -2
		case -2:
			remaining = int(octet)
			data = nil
		default:
			data = append(data, octet)
			remaining--
		}
		if remaining == 0 {
			header = append(header, tpdu.InformationElement{ID: id, Data: data})
			remaining = -1
		}
	}
	if remaining != -1 {
		return tpdu.UserDataHeader{}, end, nil
	}
	return header, end, nil
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
		want, count, decodeErr := referenceUserDataHeader(src)
		require.Equal(t, decodeErr, err)
		require.Equal(t, count, n)
		require.Equal(t, want, udh)
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
	name string
	udh  tpdu.UserDataHeader
	ci   tpdu.ConcatInfo
	ok   bool
}

var concatTestPatterns = []concatTestPattern{
	{"empty",
		tpdu.UserDataHeader{},
		tpdu.ConcatInfo{},
		false,
	},
	{"empty data",
		tpdu.UserDataHeader{
			tpdu.InformationElement{ID: 0, Data: []byte{}},
		},
		tpdu.ConcatInfo{},
		false,
	},
	{"nil data",
		tpdu.UserDataHeader{
			tpdu.InformationElement{ID: 0, Data: nil},
		},
		tpdu.ConcatInfo{},
		false,
	},
	{"concat8",
		tpdu.UserDataHeader{
			tpdu.InformationElement{ID: 0, Data: []byte{3, 2, 1}},
		},
		tpdu.ConcatInfo{Ref: 3, Total: 2, Seqno: 1},
		true,
	},
	{"id 1",
		tpdu.UserDataHeader{
			tpdu.InformationElement{ID: 1, Data: []byte{3, 2, 1}},
		},
		tpdu.ConcatInfo{},
		false,
	},
	{"concat16",
		tpdu.UserDataHeader{
			tpdu.InformationElement{ID: 8, Data: []byte{4, 3, 2, 1}},
		},
		tpdu.ConcatInfo{Ref: 1027, Ref16Bit: true, Total: 2, Seqno: 1},
		true,
	},
	{"short concat8",
		tpdu.UserDataHeader{
			tpdu.InformationElement{ID: 0, Data: []byte{2, 1}},
		},
		tpdu.ConcatInfo{},
		false,
	},
	{"short concat16",
		tpdu.UserDataHeader{
			tpdu.InformationElement{ID: 8, Data: []byte{3, 2, 1}},
		},
		tpdu.ConcatInfo{},
		false,
	},
	{"long concat8",
		tpdu.UserDataHeader{
			tpdu.InformationElement{ID: 0, Data: []byte{3, 2, 1, 0}},
		},
		tpdu.ConcatInfo{},
		false,
	},
	// 3GPP TS 23.040 Section 9.2.3.24.1: a total of zero, a sequence number of
	// zero, or one greater than the total, means the whole IE is ignored.
	{"concat8 zero total",
		tpdu.UserDataHeader{
			tpdu.InformationElement{ID: 0, Data: []byte{3, 0, 0}},
		},
		tpdu.ConcatInfo{},
		false,
	},
	{"concat8 zero seqno",
		tpdu.UserDataHeader{
			tpdu.InformationElement{ID: 0, Data: []byte{3, 2, 0}},
		},
		tpdu.ConcatInfo{},
		false,
	},
	{"concat8 seqno beyond total",
		tpdu.UserDataHeader{
			tpdu.InformationElement{ID: 0, Data: []byte{3, 2, 3}},
		},
		tpdu.ConcatInfo{},
		false,
	},
	{"concat16 zero total",
		tpdu.UserDataHeader{
			tpdu.InformationElement{ID: 8, Data: []byte{4, 3, 0, 1}},
		},
		tpdu.ConcatInfo{},
		false,
	},
	{"concat16 zero seqno",
		tpdu.UserDataHeader{
			tpdu.InformationElement{ID: 8, Data: []byte{4, 3, 2, 0}},
		},
		tpdu.ConcatInfo{},
		false,
	},
	{"concat16 seqno beyond total",
		tpdu.UserDataHeader{
			tpdu.InformationElement{ID: 8, Data: []byte{4, 3, 2, 3}},
		},
		tpdu.ConcatInfo{},
		false,
	},
	{"concat8 single",
		tpdu.UserDataHeader{
			tpdu.InformationElement{ID: 0, Data: []byte{3, 1, 1}},
		},
		tpdu.ConcatInfo{Ref: 3, Total: 1, Seqno: 1},
		true,
	},
	{"concat8 max",
		tpdu.UserDataHeader{
			tpdu.InformationElement{ID: 0, Data: []byte{0xff, 0xff, 0xff}},
		},
		tpdu.ConcatInfo{Ref: 0xff, Total: 0xff, Seqno: 0xff},
		true,
	},
	{"concat16 max",
		tpdu.UserDataHeader{
			tpdu.InformationElement{ID: 8, Data: []byte{0xff, 0xff, 0xff, 0xff}},
		},
		tpdu.ConcatInfo{Ref: 0xffff, Ref16Bit: true, Total: 0xff, Seqno: 0xff},
		true,
	},
	// 3GPP TS 23.040 Section 9.2.3.24: of mutually exclusive IEs "the last
	// occurring IE shall be used".
	{"concat8 then concat16",
		tpdu.UserDataHeader{
			tpdu.InformationElement{ID: 0, Data: []byte{3, 2, 1}},
			tpdu.InformationElement{ID: 8, Data: []byte{4, 3, 5, 4}},
		},
		tpdu.ConcatInfo{Ref: 1027, Ref16Bit: true, Total: 5, Seqno: 4},
		true,
	},
	{"concat16 then concat8",
		tpdu.UserDataHeader{
			tpdu.InformationElement{ID: 8, Data: []byte{4, 3, 5, 4}},
			tpdu.InformationElement{ID: 0, Data: []byte{3, 2, 1}},
		},
		tpdu.ConcatInfo{Ref: 3, Total: 2, Seqno: 1},
		true,
	},
	{"duplicate concat8",
		tpdu.UserDataHeader{
			tpdu.InformationElement{ID: 0, Data: []byte{3, 2, 1}},
			tpdu.InformationElement{ID: 0, Data: []byte{7, 4, 3}},
		},
		tpdu.ConcatInfo{Ref: 7, Total: 4, Seqno: 3},
		true,
	},
	{"concat8 among other IEs",
		tpdu.UserDataHeader{
			tpdu.InformationElement{ID: 0x24, Data: []byte{1}},
			tpdu.InformationElement{ID: 0, Data: []byte{3, 2, 1}},
			tpdu.InformationElement{ID: 0x25, Data: []byte{1}},
		},
		tpdu.ConcatInfo{Ref: 3, Total: 2, Seqno: 1},
		true,
	},
	// An ignored IE is skipped over, so an earlier valid IE still applies.
	{"concat16 then ignored concat8",
		tpdu.UserDataHeader{
			tpdu.InformationElement{ID: 8, Data: []byte{4, 3, 5, 4}},
			tpdu.InformationElement{ID: 0, Data: []byte{3, 0, 1}},
		},
		tpdu.ConcatInfo{Ref: 1027, Ref16Bit: true, Total: 5, Seqno: 4},
		true,
	},
	{"concat8 then short concat16",
		tpdu.UserDataHeader{
			tpdu.InformationElement{ID: 0, Data: []byte{3, 2, 1}},
			tpdu.InformationElement{ID: 8, Data: []byte{4, 3, 5}},
		},
		tpdu.ConcatInfo{Ref: 3, Total: 2, Seqno: 1},
		true,
	},
}

func TestConcatInfo(t *testing.T) {
	for _, p := range concatTestPatterns {
		f := func(t *testing.T) {
			ci, ok := p.udh.ConcatInfo()
			assert.Equal(t, p.ok, ok)
			assert.Equal(t, p.ci, ci)
		}
		t.Run(p.name, f)
	}
}

func TestUserDataHeaderConcatInfoAllValues(t *testing.T) {
	for _, wide := range []bool{false, true} {
		for total := 0; total <= 255; total++ {
			for seqno := 0; seqno <= 255; seqno++ {
				ref := total ^ seqno
				ie := tpdu.InformationElement{ID: tpdu.IEIConcat8Bit, Data: []byte{byte(ref), byte(total), byte(seqno)}}
				if wide {
					ref = total<<8 | seqno
					ie = tpdu.InformationElement{ID: tpdu.IEIConcat16Bit, Data: []byte{byte(total), byte(seqno), byte(total), byte(seqno)}}
				}
				// 3GPP TS 23.040 Sections 9.2.3.24.1 and 9.2.3.24.8:
				// ignore a zero total, a zero sequence number, or one beyond the total.
				wantOK := total >= 1 && seqno >= 1 && seqno <= total
				decoded, err := ie.ConcatInfo()
				wantDecoded := tpdu.ConcatInfo{Ref: ref, Ref16Bit: wide, Total: total, Seqno: seqno}
				if err != nil || decoded != wantDecoded || decoded.Valid() != wantOK {
					t.Fatalf("wide=%t total=%d seqno=%d: decoded %+v, %v; want %+v, valid=%t", wide, total, seqno, decoded, err, wantDecoded, wantOK)
				}
				want := tpdu.ConcatInfo{}
				if wantOK {
					want = wantDecoded
				}
				got, ok := (tpdu.UserDataHeader{ie}).ConcatInfo()
				if got != want || ok != wantOK {
					t.Fatalf("wide=%t total=%d seqno=%d: got %+v, %t; want %+v, %t", wide, total, seqno, got, ok, want, wantOK)
				}
			}
		}
	}
}

func TestInformationElementConcatInfo(t *testing.T) {
	patterns := []struct {
		name   string
		ie     tpdu.InformationElement
		ci     tpdu.ConcatInfo
		valid  bool
		err    error
		field  string
		offset int
	}{
		{"concat8", tpdu.InformationElement{ID: 0, Data: []byte{78, 3, 2}}, tpdu.ConcatInfo{Ref: 78, Total: 3, Seqno: 2}, true, nil, "", 0},
		{"concat16", tpdu.InformationElement{ID: 8, Data: []byte{0x12, 0x34, 3, 2}}, tpdu.ConcatInfo{Ref: 0x1234, Ref16Bit: true, Total: 3, Seqno: 2}, true, nil, "", 0},
		{"part 5 of 3", tpdu.InformationElement{ID: 0, Data: []byte{78, 3, 5}}, tpdu.ConcatInfo{Ref: 78, Total: 3, Seqno: 5}, false, nil, "", 0},
		{"part 0 of 3", tpdu.InformationElement{ID: 8, Data: []byte{0x12, 0x34, 3, 0}}, tpdu.ConcatInfo{Ref: 0x1234, Ref16Bit: true, Total: 3}, false, nil, "", 0},
		{"total 0", tpdu.InformationElement{ID: 0, Data: []byte{78, 0, 1}}, tpdu.ConcatInfo{Ref: 78, Seqno: 1}, false, nil, "", 0},
		{"zero8", tpdu.InformationElement{ID: 0, Data: []byte{0, 0, 0}}, tpdu.ConcatInfo{}, false, nil, "", 0},
		{"zero16", tpdu.InformationElement{ID: 8, Data: []byte{0, 0, 0, 0}}, tpdu.ConcatInfo{Ref16Bit: true}, false, nil, "", 0},
		{"max8", tpdu.InformationElement{ID: 0, Data: []byte{255, 255, 255}}, tpdu.ConcatInfo{Ref: 255, Total: 255, Seqno: 255}, true, nil, "", 0},
		{"max16", tpdu.InformationElement{ID: 8, Data: []byte{255, 255, 255, 255}}, tpdu.ConcatInfo{Ref: 65535, Ref16Bit: true, Total: 255, Seqno: 255}, true, nil, "", 0},
		{"id1 length3", tpdu.InformationElement{ID: 1, Data: []byte{1, 2, 3}}, tpdu.ConcatInfo{}, false, tpdu.ErrInvalid, "iei", 0},
		{"id1 length4", tpdu.InformationElement{ID: 1, Data: []byte{1, 2, 3, 4}}, tpdu.ConcatInfo{}, false, tpdu.ErrInvalid, "iei", 0},
		{"id9 length3", tpdu.InformationElement{ID: 9, Data: []byte{1, 2, 3}}, tpdu.ConcatInfo{}, false, tpdu.ErrInvalid, "iei", 0},
		{"id9 length4", tpdu.InformationElement{ID: 9, Data: []byte{1, 2, 3, 4}}, tpdu.ConcatInfo{}, false, tpdu.ErrInvalid, "iei", 0},
		{"nil8", tpdu.InformationElement{ID: 0}, tpdu.ConcatInfo{}, false, tpdu.ErrUnderflow, "ied", 2},
		{"empty8", tpdu.InformationElement{ID: 0, Data: []byte{}}, tpdu.ConcatInfo{}, false, tpdu.ErrUnderflow, "ied", 2},
		{"short8", tpdu.InformationElement{ID: 0, Data: []byte{1, 2}}, tpdu.ConcatInfo{}, false, tpdu.ErrUnderflow, "ied", 2},
		{"long8", tpdu.InformationElement{ID: 0, Data: []byte{1, 2, 1, 4}}, tpdu.ConcatInfo{}, false, tpdu.ErrOverlength, "ied", 2},
		{"nil16", tpdu.InformationElement{ID: 8}, tpdu.ConcatInfo{}, false, tpdu.ErrUnderflow, "ied", 2},
		{"short16", tpdu.InformationElement{ID: 8, Data: []byte{1, 2, 3}}, tpdu.ConcatInfo{}, false, tpdu.ErrUnderflow, "ied", 2},
		{"long16", tpdu.InformationElement{ID: 8, Data: []byte{1, 2, 3, 1, 5}}, tpdu.ConcatInfo{}, false, tpdu.ErrOverlength, "ied", 2},
	}
	for _, p := range patterns {
		t.Run(p.name, func(t *testing.T) {
			ci, err := p.ie.ConcatInfo()
			assert.Equal(t, p.ci, ci)
			if p.err != nil {
				require.True(t, errors.Is(err, p.err))
				var de tpdu.DecodeError
				require.True(t, errors.As(err, &de))
				assert.Equal(t, p.field, de.Field)
				assert.Equal(t, p.offset, de.Offset)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, p.valid, ci.Valid())
		})
	}
}

func TestInformationElementConcatInfoAllIDsAndLengths(t *testing.T) {
	for id := 0; id <= 255; id++ {
		for length := 0; length <= 255; length++ {
			ie := tpdu.InformationElement{ID: byte(id), Data: bytes.Repeat([]byte{1}, length)}
			ci, err := ie.ConcatInfo()
			var wantErr error
			field, offset := "ied", 2
			switch {
			case id != 0 && id != 8:
				wantErr, field, offset = tpdu.ErrInvalid, "iei", 0
			case id == 0 && length < 3 || id == 8 && length < 4:
				wantErr = tpdu.ErrUnderflow
			case id == 0 && length > 3 || id == 8 && length > 4:
				wantErr = tpdu.ErrOverlength
			}
			if wantErr == nil {
				if err != nil {
					t.Fatalf("id=%d length=%d: %v", id, length, err)
				}
				continue
			}
			var de tpdu.DecodeError
			if !errors.Is(err, wantErr) || !errors.As(err, &de) || de.Field != field || de.Offset != offset || ci != (tpdu.ConcatInfo{}) {
				t.Fatalf("id=%d length=%d: got %+v, %v; want zero, %s at %d: %v", id, length, ci, err, field, offset, wantErr)
			}
		}
	}
}

func TestConcatInfoValid(t *testing.T) {
	patterns := []struct {
		name  string
		ci    tpdu.ConcatInfo
		valid bool
	}{
		{"zero", tpdu.ConcatInfo{}, false},
		{"negative ref8", tpdu.ConcatInfo{Ref: -1, Total: 1, Seqno: 1}, false},
		{"max ref8", tpdu.ConcatInfo{Ref: 255, Total: 1, Seqno: 1}, true},
		{"large ref8", tpdu.ConcatInfo{Ref: 256, Total: 1, Seqno: 1}, false},
		{"negative ref16", tpdu.ConcatInfo{Ref: -1, Ref16Bit: true, Total: 1, Seqno: 1}, false},
		{"ref16", tpdu.ConcatInfo{Ref: 256, Ref16Bit: true, Total: 1, Seqno: 1}, true},
		{"max ref16", tpdu.ConcatInfo{Ref: 65535, Ref16Bit: true, Total: 1, Seqno: 1}, true},
		{"large ref16", tpdu.ConcatInfo{Ref: 65536, Ref16Bit: true, Total: 1, Seqno: 1}, false},
		{"negative total", tpdu.ConcatInfo{Total: -1, Seqno: 1}, false},
		{"zero total", tpdu.ConcatInfo{Total: 0, Seqno: 1}, false},
		{"one total", tpdu.ConcatInfo{Total: 1, Seqno: 1}, true},
		{"max total", tpdu.ConcatInfo{Total: 255, Seqno: 1}, true},
		{"large total", tpdu.ConcatInfo{Total: 256, Seqno: 1}, false},
		{"negative seqno", tpdu.ConcatInfo{Total: 3, Seqno: -1}, false},
		{"zero seqno", tpdu.ConcatInfo{Total: 3, Seqno: 0}, false},
		{"one seqno", tpdu.ConcatInfo{Total: 3, Seqno: 1}, true},
		{"last seqno", tpdu.ConcatInfo{Total: 3, Seqno: 3}, true},
		{"large seqno", tpdu.ConcatInfo{Total: 3, Seqno: 4}, false},
		{"negative pair", tpdu.ConcatInfo{Total: -5, Seqno: -6}, false},
	}
	for _, p := range patterns {
		t.Run(p.name, func(t *testing.T) {
			assert.Equal(t, p.valid, p.ci.Valid())
		})
	}
}

func TestDecodeUserData(t *testing.T) {
	// Also tests the WithCharset, WithLockingCharset and WithShiftCharset
	// decode options.
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
		// An IE with a reserved or unsupported NLI is ignored (3GPP TS 23.038
		// Section 6.2.1.2.5), i.e. skipped over (3GPP TS 23.040 Section
		// 9.2.3.24.16), so an earlier occurrence still applies.
		{"message 7bit locking reserved last",
			[]byte("\x01\x02\x03"),
			tpdu.UserDataHeader{
				tpdu.InformationElement{ID: tpdu.IEINationalLanguageLockingShift, Data: []byte{byte(charset.Kannada)}},
				tpdu.InformationElement{ID: tpdu.IEINationalLanguageLockingShift, Data: []byte{0x63}},
			},
			tpdu.Alpha7Bit,
			[]tpdu.UDDecodeOption{tpdu.WithAllCharsets},
			[]byte("ಂಃಅ"),
			nil,
		},
		{"message 7bit shift unsupported last",
			[]byte("\x1b\x1e\x1b\x1f\x1b\x20"),
			tpdu.UserDataHeader{
				tpdu.InformationElement{ID: tpdu.IEINationalLanguageSingleShift, Data: []byte{byte(charset.Kannada)}},
				tpdu.InformationElement{ID: tpdu.IEINationalLanguageSingleShift, Data: []byte{byte(charset.Turkish)}},
			},
			tpdu.Alpha7Bit,
			[]tpdu.UDDecodeOption{tpdu.WithShiftCharset(charset.Kannada)},
			[]byte("೨೩೪"),
			nil,
		},
		{"message 7bit locking empty last",
			[]byte("\x01\x02\x03"),
			tpdu.UserDataHeader{
				tpdu.InformationElement{ID: tpdu.IEINationalLanguageLockingShift, Data: []byte{byte(charset.Kannada)}},
				tpdu.InformationElement{ID: tpdu.IEINationalLanguageLockingShift},
			},
			tpdu.Alpha7Bit,
			[]tpdu.UDDecodeOption{tpdu.WithLockingCharset(charset.Kannada)},
			[]byte("ಂಃಅ"),
			nil,
		},
		// NLI 0 is reserved (3GPP TS 23.038 Table 6.2.1.2.4.1), so an IE
		// carrying it is ignored even if charset.Default is enabled.
		{"message 7bit locking nli 0 last",
			[]byte("\x01\x02\x03"),
			tpdu.UserDataHeader{
				tpdu.InformationElement{ID: tpdu.IEINationalLanguageLockingShift, Data: []byte{byte(charset.Kannada)}},
				tpdu.InformationElement{ID: tpdu.IEINationalLanguageLockingShift, Data: []byte{0}},
			},
			tpdu.Alpha7Bit,
			[]tpdu.UDDecodeOption{tpdu.WithLockingCharset(charset.Kannada, charset.Default)},
			[]byte("ಂಃಅ"),
			nil,
		},
		{"message 7bit locking reserved enabled last",
			[]byte("\x01\x02\x03"),
			tpdu.UserDataHeader{
				tpdu.InformationElement{ID: tpdu.IEINationalLanguageLockingShift, Data: []byte{byte(charset.Kannada)}},
				tpdu.InformationElement{ID: tpdu.IEINationalLanguageLockingShift, Data: []byte{0x63}},
			},
			tpdu.Alpha7Bit,
			[]tpdu.UDDecodeOption{tpdu.WithLockingCharset(charset.Kannada, 0x63)},
			[]byte("ಂಃಅ"),
			nil,
		},
		{"message 7bit shift nli 0 last",
			[]byte("\x1b\x1e\x1b\x1f\x1b\x20"),
			tpdu.UserDataHeader{
				tpdu.InformationElement{ID: tpdu.IEINationalLanguageSingleShift, Data: []byte{byte(charset.Kannada)}},
				tpdu.InformationElement{ID: tpdu.IEINationalLanguageSingleShift, Data: []byte{0}},
			},
			tpdu.Alpha7Bit,
			[]tpdu.UDDecodeOption{tpdu.WithCharset(charset.Kannada, charset.Default)},
			[]byte("೨೩೪"),
			nil,
		},
		{"message 7bit locking supported last",
			[]byte("\x01\x02\x03"),
			tpdu.UserDataHeader{
				tpdu.InformationElement{ID: tpdu.IEINationalLanguageLockingShift, Data: []byte{byte(charset.Turkish)}},
				tpdu.InformationElement{ID: tpdu.IEINationalLanguageLockingShift, Data: []byte{byte(charset.Kannada)}},
			},
			tpdu.Alpha7Bit,
			[]tpdu.UDDecodeOption{tpdu.WithAllCharsets},
			[]byte("ಂಃಅ"),
			nil,
		},
		// error tests
		{"odd ucs2 with prefix",
			[]byte{0x00, 0x41, 0x00},
			nil,
			tpdu.AlphaUCS2,
			nil,
			[]byte("A"), ucs2.ErrInvalidLength,
		},
		{"odd ucs2 high surrogate",
			[]byte{0x00, 0x41, 0xd8, 0x3d, 0x00},
			nil,
			tpdu.AlphaUCS2,
			nil,
			[]byte("A\uFFFD"), ucs2.ErrInvalidLength,
		},
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
			ud, udh, alpha, err := tpdu.EncodeUserData(p.msg, p.options...)
			require.Nil(t, err)
			assert.Equal(t, p.ud, ud)
			assert.Equal(t, p.udh, udh)
			assert.Equal(t, p.alpha, alpha)
		}
		t.Run(p.name, f)
	}
}

func TestEncodeUserDataInvalidUTF8(t *testing.T) {
	for _, msg := range [][]byte{
		{'h', 0xff, 'i'},
		{0xc3},
		{0xed, 0xa0, 0x80},       // encoded surrogate
		{0xf4, 0x90, 0x80, 0x80}, // beyond U+10FFFF
		[]byte("hello \xf0\x9f\x98"),
	} {
		ud, udh, _, err := tpdu.EncodeUserData(msg, tpdu.WithAllCharsets)
		assert.Equal(t, tpdu.ErrInvalidUTF8, err, "% x", msg)
		assert.Nil(t, ud, "% x", msg)
		assert.Nil(t, udh, "% x", msg)
	}
	// U+FFFD is valid UTF8 and is encoded as UCS2
	ud, udh, alpha, err := tpdu.EncodeUserData([]byte("h\uFFFDi"))
	require.Nil(t, err)
	assert.Nil(t, udh)
	assert.Equal(t, tpdu.AlphaUCS2, alpha)
	assert.Equal(t, tpdu.UserData{0x00, 'h', 0xff, 0xfd, 0x00, 'i'}, ud)
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
			_, udh, alpha, err := tpdu.EncodeUserData(p.msg, p.options...)
			require.Nil(t, err)
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
	err := unmarshalTPDU(t, &pdu, b)
	require.Nil(t, err)
	msg, err := tpdu.DecodeUserData(pdu.UD, pdu.UDH, tpdu.Alpha7Bit,
		tpdu.WithLockingCharset(charset.Turkish))
	require.Nil(t, err)
	assert.Equal(t, "ş", string(msg))
}

// TestUserDataHeaderIgnoredRoundTrip checks that a TPDU whose UDH is ignored
// still marshals to the octets it was decoded from, as "irrespective of
// whether any part of the User Data Header is ignored or discarded, the MS
// shall always store the entire TPDU exactly as received" (3GPP TS 23.040
// Section 9.2.3.24).
func TestUserDataHeaderIgnoredRoundTrip(t *testing.T) {
	scts := []byte{0x71, 0x80, 0x13, 0x11, 0x12, 0x45, 0x23}
	oa := []byte{0x0b, 0x91, 0x16, 0x04, 0x89, 0x56, 0x26, 0xf9}
	deliver := func(dcs byte, tpud ...byte) []byte {
		b := append([]byte{0x44}, oa...)
		b = append(b, 0x00, dcs)
		b = append(b, scts...)
		return append(b, tpud...)
	}
	patterns := []struct {
		name string
		dirn tpdu.Direction
		in   []byte
		udhl int
		bs   int // room left by the UDH as received
		ud   tpdu.UserData
	}{
		{"deliver 8bit ie borrows message octet", tpdu.MT,
			deliver(0x04, 0x04, 0x02, 0x70, 0x01, 0x41),
			2, 140 - 3, tpdu.UserData{0x41}},
		{"submit 8bit ie overruns udhl", tpdu.MO,
			[]byte{0x41, 0x00, 0x00, 0x80, 0x00, 0x04, 0x05, 0x03, 0x00, 0x02, 0x01, 0x41},
			3, 140 - 4, tpdu.UserData{0x41}},
		{"deliver 8bit ie overruns udhl", tpdu.MT,
			deliver(0x04, 0x07, 0x02, 0x00, 0x03, 0x07, 0x02, 0x01, 0x41),
			2, 140 - 3, tpdu.UserData{0x07, 0x02, 0x01, 0x41}},
		{"deliver 7bit ie cut short", tpdu.MT,
			deliver(0x00, 0x0b, 0x04, 0x00, 0x03, 0x01, 0x02, 0xa0, 0xcb, 0x6c, 0xf6, 0x1b),
			4, 160 - 6, tpdu.UserData("hello")},
	}
	for _, p := range patterns {
		f := func(t *testing.T) {
			pdu := tpdu.TPDU{Direction: p.dirn}
			require.Nil(t, unmarshalTPDU(t, &pdu, p.in))
			assert.Equal(t, tpdu.UserDataHeader{}, pdu.UDH)
			assert.Equal(t, p.ud, pdu.UD)
			assert.Equal(t, p.udhl, pdu.UDHL())
			assert.Equal(t, p.bs, pdu.UDBlockSize())
			out, err := pdu.MarshalBinary()
			require.Nil(t, err)
			assert.Equal(t, p.in, out)

			// the header octets are only kept while the UDH is left as
			// decoded
			pdu.SetUDH(tpdu.UserDataHeader{})
			out, err = pdu.MarshalBinary()
			require.Nil(t, err)
			assert.NotEqual(t, p.in, out)
			again := tpdu.TPDU{Direction: p.dirn}
			require.Nil(t, unmarshalTPDU(t, &again, out))
			assert.Equal(t, p.ud, again.UD)
		}
		t.Run(p.name, f)
	}
}

// The charset options copy the national language identifiers they are given,
// so a later write by the caller to its slice does not change them.
func TestCharsetOptionsCopyIdentifiers(t *testing.T) {
	nli := []int{charset.Urdu}
	options := []tpdu.UDEncodeOption{tpdu.WithCharset(nli...), tpdu.WithLockingCharset(nli...)}
	decode := []tpdu.UDDecodeOption{tpdu.WithCharset(nli...), tpdu.WithLockingCharset(nli...)}
	snli := []int{charset.Turkish}
	shift := tpdu.WithShiftCharset(snli...)
	nli[0] = charset.Spanish
	snli[0] = charset.Spanish
	for i, o := range options {
		ud, udh, alpha, err := tpdu.EncodeUserData([]byte("hello ٻ"), o)
		require.NoError(t, err)
		assert.Equal(t, tpdu.Alpha7Bit, alpha)
		assert.Equal(t, tpdu.InformationElement{ID: tpdu.IEINationalLanguageLockingShift, Data: []byte{byte(charset.Urdu)}}, udh[0])
		msg, err := tpdu.DecodeUserData(ud, udh, alpha, decode[i])
		require.NoError(t, err)
		assert.Equal(t, "hello ٻ", string(msg))
	}
	_, udh, alpha, err := tpdu.EncodeUserData([]byte("hello ş"), shift)
	require.NoError(t, err)
	assert.Equal(t, tpdu.Alpha7Bit, alpha)
	assert.Equal(t, tpdu.InformationElement{ID: tpdu.IEINationalLanguageSingleShift, Data: []byte{byte(charset.Turkish)}}, udh[0])
}
