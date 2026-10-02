// SPDX-License-Identifier: MIT

package sms_test

import (
	"errors"
	"testing"

	"github.com/gomaja/go-sms"
	"github.com/gomaja/go-sms/encoding/gsm7/charset"
	"github.com/gomaja/go-sms/encoding/tpdu"
	"github.com/gomaja/go-sms/encoding/ucs2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var twoSegmentMsg = []byte("this is a very long message that does not fit in a single SMS message, at least it will if I keep adding more to it as 160 characters is more than you might think")

type badTPDUOption struct {
	err error
}

func (o badTPDUOption) ApplyTPDUOption(*tpdu.TPDU) error {
	return o.err
}

func TestNewEncoder(t *testing.T) {
	e := sms.NewEncoder()
	assert.NotNil(t, e)
}

var patterns = []struct {
	name    string
	msg     []byte
	options []sms.EncoderOption
	out     []tpdu.TPDU
	err     error
}{
	{
		// an empty message is carried by one TPDU with a TP-UDL of 0.
		"nil",
		nil,
		nil,
		[]tpdu.TPDU{
			{
				Direction:  tpdu.MO,
				FirstOctet: tpdu.FirstOctet(tpdu.MtSubmit),
				MR:         1,
			},
		},
		nil,
	},
	{
		"empty",
		[]byte{},
		nil,
		[]tpdu.TPDU{
			{
				Direction:  tpdu.MO,
				FirstOctet: tpdu.FirstOctet(tpdu.MtSubmit),
				MR:         1,
			},
		},
		nil,
	},
	{
		"single segment",
		[]byte("hello"),
		nil,
		[]tpdu.TPDU{
			{
				Direction:  tpdu.MO,
				FirstOctet: tpdu.FirstOctet(tpdu.MtSubmit),
				MR:         1,
				UD:         []byte("hello"),
			},
		},
		nil,
	},
	{
		"single segment grin",
		[]byte("hello 😁"),
		nil,
		[]tpdu.TPDU{
			{
				Direction:  tpdu.MO,
				FirstOctet: tpdu.FirstOctet(tpdu.MtSubmit),
				PI:         tpdu.PiDCS, // not relevant for Submit, but set as side-effect
				DCS:        tpdu.DcsUCS2Data,
				MR:         1,
				UD: []byte{
					0x00, 0x68, 0x00, 0x65, 0x00, 0x6c, 0x00, 0x6c, 0x00, 0x6f,
					0x00, 0x20, 0xd8, 0x3d, 0xde, 0x01,
				},
			},
		},
		nil,
	},
	{
		"three grins",
		[]byte("😁😁😁"),
		nil,
		[]tpdu.TPDU{
			{
				Direction:  tpdu.MO,
				FirstOctet: tpdu.FirstOctet(tpdu.MtSubmit),
				PI:         tpdu.PiDCS, // not relevant for Submit, but set as side-effect
				DCS:        tpdu.DcsUCS2Data,
				MR:         1,
				UD: []byte{
					0xd8, 0x3d, 0xde, 0x01, 0xd8, 0x3d, 0xde, 0x01, 0xd8, 0x3d,
					0xde, 0x01,
				},
			},
		},
		nil,
	},
	{
		"single segment unused urdu",
		[]byte("hello"),
		[]sms.EncoderOption{sms.WithCharset(charset.Urdu)},
		[]tpdu.TPDU{
			{
				Direction:  tpdu.MO,
				FirstOctet: tpdu.FirstOctet(tpdu.MtSubmit),
				MR:         1,
				UD:         []byte("hello"),
			},
		},
		nil,
	},
	{
		"single segment with urdu",
		[]byte("hello ت"),
		[]sms.EncoderOption{sms.WithCharset(charset.Urdu)},
		[]tpdu.TPDU{
			{
				Direction:  tpdu.MO,
				FirstOctet: 0x41, // UDHI | Submit
				MR:         1,
				PI:         tpdu.PiUDL,
				UDH: tpdu.UserDataHeader{
					tpdu.InformationElement{ID: tpdu.IEINationalLanguageLockingShift, Data: []byte{0x0d}},
				},
				UD: []byte("hello \x07"),
			},
		},
		nil,
	},
	{
		"single segment with locking urdu",
		[]byte("hello ت"),
		[]sms.EncoderOption{sms.WithLockingCharset(charset.Urdu)},
		[]tpdu.TPDU{
			{
				Direction:  tpdu.MO,
				FirstOctet: 0x41, // UDHI | Submit
				MR:         1,
				PI:         tpdu.PiUDL,
				UDH: tpdu.UserDataHeader{
					tpdu.InformationElement{ID: tpdu.IEINationalLanguageLockingShift, Data: []byte{0x0d}},
				},
				UD: []byte("hello \x07"),
			},
		},
		nil,
	},
	{
		"single segment with shift urdu",
		[]byte("hello ؎"),
		[]sms.EncoderOption{sms.WithShiftCharset(charset.Urdu)},
		[]tpdu.TPDU{
			{
				Direction:  tpdu.MO,
				FirstOctet: 0x41, // UDHI | Submit
				MR:         1,
				PI:         tpdu.PiUDL,
				UDH: tpdu.UserDataHeader{
					tpdu.InformationElement{ID: tpdu.IEINationalLanguageSingleShift, Data: []byte{0x0d}},
				},
				UD: []byte("hello \x1b\x2a"),
			},
		},
		nil,
	},
	{
		"single segment discovered urdu",
		[]byte("hello ت؎"),
		[]sms.EncoderOption{sms.WithAllCharsets},
		[]tpdu.TPDU{
			{
				Direction:  tpdu.MO,
				FirstOctet: 0x41, // UDHI | Submit
				MR:         1,
				PI:         tpdu.PiUDL,
				UDH: tpdu.UserDataHeader{
					tpdu.InformationElement{ID: tpdu.IEINationalLanguageLockingShift, Data: []byte{0x0d}},
					tpdu.InformationElement{ID: tpdu.IEINationalLanguageSingleShift, Data: []byte{0x0d}},
				},
				UD: []byte("hello \x07\x1b\x2a"),
			},
		},
		nil,
	},
	{
		"single segment UCS2",
		[]byte("hello!"), // this isn't UCS2, but demonstrates it is passed raw, not re-encoded.
		[]sms.EncoderOption{sms.AsUCS2},
		[]tpdu.TPDU{
			{
				Direction:  tpdu.MO,
				FirstOctet: tpdu.FirstOctet(tpdu.MtSubmit),
				PI:         tpdu.PiDCS, // not relevant for Submit, but set as side-effect
				DCS:        tpdu.DcsUCS2Data,
				MR:         1,
				UD:         []byte("hello!"),
			},
		},
		nil,
	},
	{
		"dcs conflict",
		[]byte("hello 😁"),
		[]sms.EncoderOption{sms.WithTemplateOption(tpdu.DCS(0x80))},
		nil,
		sms.ErrDcsConflict,
	},
	{
		// An SMS-DELIVER has no TP-MR, so draws none (TS 23.040 9.2.3.6).
		"deliver single segment",
		[]byte("hello"),
		[]sms.EncoderOption{sms.AsDeliver},
		[]tpdu.TPDU{
			{
				UD: []byte("hello"),
			},
		},
		nil,
	},
	{
		// A number without a '+' is not known to be international, so it
		// is sent with a type of number of unknown (3GPP TS 27.005 Section
		// 3.1, <toda>: "when first character of <da> is + (IRA 43) default
		// is 145, otherwise default is 129").
		"number",
		[]byte("hello"),
		[]sms.EncoderOption{sms.To("1234")},
		[]tpdu.TPDU{
			{
				Direction:  tpdu.MO,
				FirstOctet: tpdu.FirstOctet(tpdu.MtSubmit),
				MR:         1,
				DA:         tpdu.Address{TOA: 0x81, Addr: "1234"},
				UD:         []byte("hello"),
			},
		},
		nil,
	},
	{
		"deliver number",
		[]byte("hello"),
		[]sms.EncoderOption{sms.AsDeliver, sms.From("1234")},
		[]tpdu.TPDU{
			{
				OA: tpdu.Address{TOA: 0x81, Addr: "1234"},
				UD: []byte("hello"),
			},
		},
		nil,
	},
	{
		"deliver plus number",
		[]byte("hello"),
		[]sms.EncoderOption{sms.AsDeliver, sms.From("+1234")},
		[]tpdu.TPDU{
			{
				OA: tpdu.Address{TOA: 0x91, Addr: "1234"},
				UD: []byte("hello"),
			},
		},
		nil,
	},
	{
		"plus number",
		[]byte("hello"),
		[]sms.EncoderOption{sms.To("+1234")},
		[]tpdu.TPDU{
			{
				Direction:  tpdu.MO,
				FirstOctet: tpdu.FirstOctet(tpdu.MtSubmit),
				MR:         1,
				DA:         tpdu.Address{TOA: 0x91, Addr: "1234"},
				UD:         []byte("hello"),
			},
		},
		nil,
	},
	{
		"two segment 7bit",
		twoSegmentMsg,
		[]sms.EncoderOption{sms.To("+1234")},
		[]tpdu.TPDU{
			{
				Direction:  tpdu.MO,
				FirstOctet: 0x41, // Submit | UDHI
				MR:         1,
				PI:         tpdu.PiUDL, // not relevant for Submit, but set as side-effect
				DA:         tpdu.Address{TOA: 0x91, Addr: "1234"},
				UDH: tpdu.UserDataHeader{
					tpdu.InformationElement{ID: 0, Data: []byte{1, 2, 1}},
				},
				UD: []byte("this is a very long message that does not fit in a single SMS message, at least it will if I keep adding more to it as 160 characters is more than you mi"),
			},
			{
				Direction:  tpdu.MO,
				FirstOctet: 0x41,
				MR:         2,
				PI:         tpdu.PiUDL, // not relevant for Submit, but set as side-effect
				DA:         tpdu.Address{TOA: 0x91, Addr: "1234"},
				UDH: tpdu.UserDataHeader{
					tpdu.InformationElement{ID: 0, Data: []byte{1, 2, 2}},
				},
				UD: []byte("ght think"),
			},
		},
		nil,
	},
	{
		"two segment 7bit with UDH",
		twoSegmentMsg,
		[]sms.EncoderOption{
			sms.To("+1234"),
			sms.WithTemplateOption(
				tpdu.WithUDH(
					tpdu.UserDataHeader{
						tpdu.InformationElement{ID: 3, Data: []byte{1, 2, 3}},
					},
				)),
		},
		[]tpdu.TPDU{
			{
				Direction:  tpdu.MO,
				FirstOctet: 0x41, // Submit | UDHI
				MR:         1,
				PI:         tpdu.PiUDL, // not relevant for Submit, but set as side-effect
				DA:         tpdu.Address{TOA: 0x91, Addr: "1234"},
				UDH: tpdu.UserDataHeader{
					tpdu.InformationElement{ID: 3, Data: []byte{1, 2, 3}},
					tpdu.InformationElement{ID: 0, Data: []byte{1, 2, 1}},
				},
				UD: []byte("this is a very long message that does not fit in a single SMS message, at least it will if I keep adding more to it as 160 characters is more than "),
			},
			{
				Direction:  tpdu.MO,
				FirstOctet: 0x41,
				MR:         2,
				PI:         tpdu.PiUDL, // not relevant for Submit, but set as side-effect
				DA:         tpdu.Address{TOA: 0x91, Addr: "1234"},
				UDH: tpdu.UserDataHeader{
					tpdu.InformationElement{ID: 3, Data: []byte{1, 2, 3}},
					tpdu.InformationElement{ID: 0, Data: []byte{1, 2, 2}},
				},
				UD: []byte("you might think"),
			},
		},
		nil,
	},
	{
		"two segment 8bit with template",
		twoSegmentMsg,
		[]sms.EncoderOption{
			sms.To("1234"), // overridden by template
			sms.WithTemplate(
				tpdu.TPDU{
					DCS: 0x14,
					DA:  tpdu.Address{TOA: 0x91, Addr: "4321"},
					UDH: tpdu.UserDataHeader{
						tpdu.InformationElement{ID: 3, Data: []byte{1, 2, 3}},
					},
				}),
			sms.AsSubmit,
		},
		[]tpdu.TPDU{
			{
				Direction:  tpdu.MO,
				FirstOctet: 0x41, // Submit | UDHI
				MR:         1,
				PI:         tpdu.PiUDL, // not relevant for Submit, but set as side-effect
				DCS:        0x14,
				DA:         tpdu.Address{TOA: 0x91, Addr: "4321"},
				UDH: tpdu.UserDataHeader{
					tpdu.InformationElement{ID: 3, Data: []byte{1, 2, 3}},
					tpdu.InformationElement{ID: 0, Data: []byte{1, 2, 1}},
				},
				UD: []byte("this is a very long message that does not fit in a single SMS message, at least it will if I keep adding more to it as 160 charac"),
			},
			{
				Direction:  tpdu.MO,
				FirstOctet: 0x41,
				MR:         2,
				PI:         tpdu.PiUDL, // not relevant for Submit, but set as side-effect
				DCS:        0x14,
				DA:         tpdu.Address{TOA: 0x91, Addr: "4321"},
				UDH: tpdu.UserDataHeader{
					tpdu.InformationElement{ID: 3, Data: []byte{1, 2, 3}},
					tpdu.InformationElement{ID: 0, Data: []byte{1, 2, 2}},
				},
				UD: []byte("ters is more than you might think"),
			},
		},
		nil,
	},
	{
		"two segment 8bit",
		twoSegmentMsg,
		[]sms.EncoderOption{sms.To("+1234"), sms.As8Bit},
		[]tpdu.TPDU{
			{
				Direction:  tpdu.MO,
				FirstOctet: 0x41, // Submit | UDHI
				MR:         1,
				DCS:        tpdu.Dcs8BitData,
				PI:         tpdu.PiUDL | tpdu.PiDCS, // not relevant for Submit, but set as side-effect
				DA:         tpdu.Address{TOA: 0x91, Addr: "1234"},
				UDH: tpdu.UserDataHeader{
					tpdu.InformationElement{ID: 0, Data: []byte{1, 2, 1}},
				},
				UD: []byte("this is a very long message that does not fit in a single SMS message, at least it will if I keep adding more to it as 160 characters "),
			},
			{
				Direction:  tpdu.MO,
				FirstOctet: 0x41,
				MR:         2,
				DCS:        0x04,
				PI:         tpdu.PiUDL | tpdu.PiDCS, // not relevant for Submit, but set as side-effect
				DA:         tpdu.Address{TOA: 0x91, Addr: "1234"},
				UDH: tpdu.UserDataHeader{
					tpdu.InformationElement{ID: 0, Data: []byte{1, 2, 2}},
				},
				UD: []byte("is more than you might think"),
			},
		},
		nil,
	},
	{
		"deliver two segment 7bit",
		twoSegmentMsg,
		[]sms.EncoderOption{sms.AsDeliver, sms.From("+1234")},
		[]tpdu.TPDU{
			{
				FirstOctet: 0x40,       // UDHI
				PI:         tpdu.PiUDL, // not relevant for Deliver, but set as side-effect
				OA:         tpdu.Address{TOA: 0x91, Addr: "1234"},
				UDH: tpdu.UserDataHeader{
					tpdu.InformationElement{ID: 0, Data: []byte{1, 2, 1}},
				},
				UD: []byte("this is a very long message that does not fit in a single SMS message, at least it will if I keep adding more to it as 160 characters is more than you mi"),
			},
			{
				FirstOctet: 0x40,
				PI:         tpdu.PiUDL, // not relevant for Deliver, but set as side-effect
				OA:         tpdu.Address{TOA: 0x91, Addr: "1234"},
				UDH: tpdu.UserDataHeader{
					tpdu.InformationElement{ID: 0, Data: []byte{1, 2, 2}},
				},
				UD: []byte("ght think"),
			},
		},
		nil,
	},
}

// freshCounters makes the TP-MR and concatenation reference of the patterns
// start at 1, rather than continue those shared by Encode.
func freshCounters(options ...sms.EncoderOption) []sms.EncoderOption {
	return append([]sms.EncoderOption{
		sms.WithMR(&sms.Counter{}),
		sms.WithConcatRef(&sms.Counter{}),
	}, options...)
}

func TestEncode(t *testing.T) {
	for _, p := range patterns {
		f := func(t *testing.T) {
			out, err := sms.Encode(p.msg, freshCounters(p.options...)...)
			assert.Equal(t, p.err, err)
			assert.Equal(t, p.out, out)
		}
		t.Run(p.name, f)
	}
}

func TestEncodeInvalidUTF8(t *testing.T) {
	for _, msg := range [][]byte{
		{'h', 0xff, 'i'},
		{0xc3},
		{0xed, 0xa0, 0x80}, // encoded surrogate
		[]byte("hello \xf0\x9f\x98"),
	} {
		out, err := sms.Encode(msg)
		assert.Equal(t, tpdu.ErrInvalidUTF8, err, "% x", msg)
		assert.Nil(t, out, "% x", msg)
	}
	// U+FFFD itself is valid UTF-8, and is encoded as UCS2
	out, err := sms.Encode([]byte("h�i"))
	assert.Nil(t, err)
	if assert.Len(t, out, 1) {
		assert.Equal(t, tpdu.UserData{0x00, 'h', 0xff, 0xfd, 0x00, 'i'}, out[0].UD)
	}
}

// TestEncodeAsUCS2Octets checks that AsUCS2 takes the octets of ucs2.Encode,
// strict UCS2, and of ucs2.EncodeUTF16, which codes a character above U+FFFF
// as a surrogate pair, as they are, and that Decode gives the message back.
func TestEncodeAsUCS2Octets(t *testing.T) {
	strict, err := ucs2.Encode([]rune("Привет €"))
	require.NoError(t, err)
	wide, err := ucs2.EncodeUTF16([]rune("hi 😁"))
	require.NoError(t, err)
	assert.Equal(t, []byte{0x00, 'h', 0x00, 'i', 0x00, ' ', 0xd8, 0x3d, 0xde, 0x01}, wide)
	for _, p := range []struct {
		msg  []byte
		want string
	}{
		{strict, "Привет €"},
		{wide, "hi 😁"},
	} {
		pdus, err := sms.Encode(p.msg, sms.AsUCS2)
		require.NoError(t, err)
		require.Len(t, pdus, 1)
		assert.Equal(t, tpdu.DcsUCS2Data, pdus[0].DCS)
		assert.Equal(t, p.msg, []byte(pdus[0].UD))
		out, err := sms.Decode([]*tpdu.TPDU{&pdus[0]})
		require.NoError(t, err)
		assert.Equal(t, p.want, string(out))
	}
}

func TestEncodeReturnsTemplateOptionError(t *testing.T) {
	errTemplate := errors.New("template option failed")
	out, err := sms.Encode([]byte("hello"), sms.WithTemplateOption(badTPDUOption{err: errTemplate}))
	assert.Nil(t, out)
	assert.True(t, errors.Is(err, errTemplate))
}

func TestEncoderEncodeReturnsNewEncoderTemplateOptionError(t *testing.T) {
	errTemplate := errors.New("template option failed")
	e := sms.NewEncoder(sms.WithTemplateOption(badTPDUOption{err: errTemplate}))
	out, err := e.Encode([]byte("hello"))
	assert.Nil(t, out)
	assert.True(t, errors.Is(err, errTemplate))
}

func TestEncoderEncode(t *testing.T) {
	for _, p := range patterns {
		f := func(t *testing.T) {
			e := sms.NewEncoder(freshCounters(sms.AsSubmit)...)
			out, err := e.Encode(p.msg, p.options...)
			assert.Equal(t, p.err, err)
			assert.Equal(t, p.out, out)
		}
		t.Run(p.name, f)
	}
}

func TestEncoderCounters(t *testing.T) {
	msgC := &sms.Counter{}
	concatC := &sms.Counter{}
	e := sms.NewEncoder(sms.AsSubmit, sms.WithMR(msgC), sms.WithConcatRef(concatC))
	assert.Equal(t, 0, msgC.Read())
	assert.Equal(t, 0, concatC.Read())

	p, err := e.Encode([]byte("blah"))
	assert.Nil(t, err)
	assert.Equal(t, 1, len(p))
	assert.Equal(t, 1, msgC.Read())
	assert.Equal(t, 0, concatC.Read())

	p, err = e.Encode(twoSegmentMsg)
	assert.Nil(t, err)
	assert.Equal(t, 2, len(p))
	assert.Equal(t, 3, msgC.Read())
	assert.Equal(t, 1, concatC.Read())
}
