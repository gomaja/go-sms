// SPDX-License-Identifier: MIT

package sms_test

import (
	"testing"
	"time"

	"github.com/gomaja/go-sms"
	"github.com/gomaja/go-sms/encoding/gsm7/charset"
	"github.com/gomaja/go-sms/encoding/tpdu"
	"github.com/gomaja/go-sms/encoding/ucs2"
	"github.com/stretchr/testify/assert"
)

func TestDecode(t *testing.T) {
	patterns := []struct {
		name    string
		in      []*tpdu.TPDU
		options []sms.DecodeOption
		out     []byte
		err     error
	}{
		{
			"two segment 7bit",
			[]*tpdu.TPDU{
				{
					UD: []byte("this is a very long message that does not fit in a single SMS message, at least it will if I keep adding more to it as 160 characters is more than you mi"),
				},
				{
					UD: []byte("ght think"),
				},
			},
			nil,
			twoSegmentMsg,
			nil,
		},
		{
			"two segment 8bit",
			[]*tpdu.TPDU{
				{
					DCS: tpdu.Dcs8BitData,
					UD:  []byte("this is a very long message that does not fit in a single SMS message, at least it will if I keep adding more to it as 160 characters is more than you mi"),
				},
				{
					DCS: tpdu.Dcs8BitData,
					UD:  []byte("ght think"),
				},
			},
			nil,
			twoSegmentMsg,
			nil,
		},
		{
			"single segment 8bit",
			[]*tpdu.TPDU{
				{
					DCS: tpdu.Dcs8BitData,
					UD:  []byte("this is not a very long message"),
				},
			},
			nil,
			[]byte("this is not a very long message"),
			nil,
		},
		{
			"single segment 7bit",
			[]*tpdu.TPDU{
				{
					UD: []byte("hello \x03"),
				},
			},
			nil,
			[]byte("hello ¥"),
			nil,
		},
		{
			"compressed user data",
			[]*tpdu.TPDU{
				{
					DCS: 0x20,
					UD:  []byte("compressed"),
				},
			},
			nil,
			nil,
			sms.ErrCompressedUserData,
		},
		{
			// An SMS-COMMAND has no TP-DCS, so a DCS left in the struct does
			// not make its TP-CD compressed.
			"command with compressed dcs",
			[]*tpdu.TPDU{
				{
					Direction:  tpdu.MO,
					FirstOctet: tpdu.FirstOctet(tpdu.MtCommand),
					DCS:        0x20,
					UD:         []byte("cd"),
				},
			},
			nil,
			[]byte("cd"),
			nil,
		},
		{
			"single segment 7bit implicit urdu",
			[]*tpdu.TPDU{
				{
					UDH: tpdu.UserDataHeader{
						tpdu.InformationElement{ID: tpdu.IEINationalLanguageLockingShift, Data: []byte{0x0d}},
					},
					UD: []byte("hello \x03"),
				},
			},
			nil, // decode uses all charsets by default
			[]byte("hello ٻ"),
			nil,
		},
		{
			"single segment 7bit explicit urdu",
			[]*tpdu.TPDU{
				{
					UDH: tpdu.UserDataHeader{
						tpdu.InformationElement{ID: tpdu.IEINationalLanguageLockingShift, Data: []byte{0x0d}},
					},
					UD: []byte("hello \x03"),
				},
			},
			[]sms.DecodeOption{sms.WithCharset(charset.Urdu)},
			[]byte("hello ٻ"),
			nil,
		},
		{
			"single segment 7bit locking urdu",
			[]*tpdu.TPDU{
				{
					UDH: tpdu.UserDataHeader{
						tpdu.InformationElement{ID: tpdu.IEINationalLanguageLockingShift, Data: []byte{0x0d}},
					},
					UD: []byte("hello \x03"),
				},
			},
			[]sms.DecodeOption{sms.WithLockingCharset(charset.Urdu)},
			[]byte("hello ٻ"),
			nil,
		},
		{
			"single segment 7bit shift urdu",
			[]*tpdu.TPDU{
				{
					UDH: tpdu.UserDataHeader{
						tpdu.InformationElement{ID: tpdu.IEINationalLanguageSingleShift, Data: []byte{0x0d}},
					},
					UD: []byte("hello \x1b\x2b"),
				},
			},
			[]sms.DecodeOption{sms.WithShiftCharset(charset.Urdu)},
			[]byte("hello ؏"),
			nil,
		},
		{
			"single segment 7bit urdu unsupported",
			[]*tpdu.TPDU{
				{
					UDH: tpdu.UserDataHeader{
						tpdu.InformationElement{ID: tpdu.IEINationalLanguageLockingShift, Data: []byte{0x0d}},
					},
					UD: []byte("hello \x03"),
				},
			},
			[]sms.DecodeOption{sms.WithDefaultCharset},
			[]byte("hello ¥"),
			nil,
		},
		{
			"ucs2 split surrogate",
			[]*tpdu.TPDU{
				{
					DCS: tpdu.DcsUCS2Data,
					UD:  []byte{0xd8, 0x3d, 0xde, 0x01, 0xd8, 0x3d},
				},
				{
					DCS: tpdu.DcsUCS2Data,
					UD:  []byte{0xde, 0x01, 0xd8, 0x3d, 0xde, 0x01},
				},
			},
			nil,
			[]byte("😁😁😁"),
			nil,
		},
		{
			"ucs2 odd length",
			[]*tpdu.TPDU{
				{
					DCS: tpdu.DcsUCS2Data,
					UD:  []byte{0xd8, 0x3d, 0xde, 0x01, 0xd8},
				},
			},
			nil,
			nil,
			ucs2.ErrInvalidLength,
		},
	}
	for _, p := range patterns {
		f := func(t *testing.T) {
			out, err := sms.Decode(p.in, p.options...)
			assert.Equal(t, p.err, err)
			assert.Equal(t, p.out, out)
		}
		t.Run(p.name, f)
	}
}

// WithAllCharsets is a decode option as well as an encode option, and adds
// every character set to those given by other options.
func TestDecodeWithAllCharsets(t *testing.T) {
	in := []*tpdu.TPDU{
		{
			UDH: tpdu.UserDataHeader{
				tpdu.InformationElement{ID: tpdu.IEINationalLanguageLockingShift, Data: []byte{byte(charset.Urdu)}},
			},
			UD: []byte("hello \x03"),
		},
	}
	out, err := sms.Decode(in, sms.WithCharset(charset.Turkish))
	assert.NoError(t, err)
	assert.Equal(t, "hello ¥", string(out))
	out, err = sms.Decode(in, sms.WithCharset(charset.Turkish), sms.WithAllCharsets)
	assert.NoError(t, err)
	assert.Equal(t, "hello ٻ", string(out))
	out, err = sms.Decode(in, sms.WithAllCharsets)
	assert.NoError(t, err)
	assert.Equal(t, "hello ٻ", string(out))
}

// ucs2Segment returns a UCS2 segment of a concatenated message holding the
// UD.
func ucs2Segment(ud ...byte) *tpdu.TPDU {
	return &tpdu.TPDU{DCS: tpdu.DcsUCS2Data, UD: ud}
}

// An unpaired surrogate decodes as U+FFFD wherever it is, as ucs2.Decode
// does, rather than failing the message or taking a character from the next
// segment.
func TestDecodeUnpairedSurrogate(t *testing.T) {
	patterns := []struct {
		name string
		in   []*tpdu.TPDU
		out  string
	}{
		{
			"low surrogate ending a segment",
			[]*tpdu.TPDU{
				ucs2Segment(0x00, 0x41, 0xde, 0x01),
				ucs2Segment(0x00, 0x42, 0x00, 0x43),
			},
			"A�BC",
		},
		{
			"high surrogate ending the message",
			[]*tpdu.TPDU{
				ucs2Segment(0xd8, 0x3d, 0xde, 0x01, 0xd8, 0x3d),
			},
			"😁�",
		},
		{
			"high surrogate ending the last segment",
			[]*tpdu.TPDU{
				ucs2Segment(0x00, 0x41),
				ucs2Segment(0x00, 0x42, 0xd8, 0x3d),
			},
			"AB�",
		},
		{
			"high surrogate before a segment not starting with a low surrogate",
			[]*tpdu.TPDU{
				ucs2Segment(0x00, 0x41, 0xd8, 0x3d),
				ucs2Segment(0x00, 0x42),
			},
			"A�B",
		},
		{
			"high surrogate before a GSM 7 bit segment",
			[]*tpdu.TPDU{
				ucs2Segment(0x00, 0x41, 0xd8, 0x3d),
				{UD: []byte("BC")},
			},
			"A�BC",
		},
		{
			"high surrogate before an 8 bit segment",
			[]*tpdu.TPDU{
				ucs2Segment(0xd8, 0x3d),
				{DCS: tpdu.Dcs8BitData, UD: []byte("BC")},
			},
			"�BC",
		},
		{
			"surrogate pair split over three segments",
			[]*tpdu.TPDU{
				ucs2Segment(0xd8, 0x3d),
				ucs2Segment(0xde, 0x01, 0xd8, 0x3d),
				ucs2Segment(0xde, 0x02),
			},
			"😁😂",
		},
	}
	for _, p := range patterns {
		f := func(t *testing.T) {
			out, err := sms.Decode(p.in)
			assert.NoError(t, err)
			assert.Equal(t, p.out, string(out))
		}
		t.Run(p.name, f)
	}
}

// A nil segment, as the expiry handler and Pipes give for a missing one, is
// reported rather than dereferenced.
func TestDecodeMissingSegment(t *testing.T) {
	seg := func(seqno byte) *tpdu.TPDU {
		return &tpdu.TPDU{
			UDH: tpdu.UserDataHeader{
				tpdu.InformationElement{ID: 0, Data: []byte{3, 3, seqno}},
			},
			UD: []byte("abc"),
		}
	}
	for _, in := range [][]*tpdu.TPDU{
		{nil},
		{nil, seg(2), seg(3)},
		{seg(1), nil, seg(3)},
		{seg(1), seg(2), nil},
	} {
		assert.NotPanics(t, func() {
			out, err := sms.Decode(in)
			assert.Equal(t, sms.ErrMissingSegment, err)
			assert.Nil(t, out)
		})
		assert.NotPanics(t, func() {
			assert.False(t, sms.IsCompleteMessage(in))
		})
	}
}

func TestIsCompleteMessage(t *testing.T) {
	patterns := []struct {
		name string
		in   []*tpdu.TPDU
		out  bool
	}{
		{
			"nil",
			nil,
			false,
		},
		{
			"empty",
			[]*tpdu.TPDU{},
			false,
		},
		{
			"single segment",
			[]*tpdu.TPDU{
				{},
			},
			true,
		},
		{
			"segment count mismatch",
			[]*tpdu.TPDU{
				{
					UDH: tpdu.UserDataHeader{
						tpdu.InformationElement{ID: 0, Data: []byte{3, 2, 1}},
					},
				},
			},
			false,
		},
		{
			"segments mismatch",
			[]*tpdu.TPDU{
				{
					UDH: tpdu.UserDataHeader{
						tpdu.InformationElement{ID: 0, Data: []byte{3, 2, 1}},
					},
				},
				{
					UDH: tpdu.UserDataHeader{
						tpdu.InformationElement{ID: 0, Data: []byte{3, 3, 2}},
					},
				},
			},
			false,
		},
		{
			"concatRef mismatch",
			[]*tpdu.TPDU{
				{
					UDH: tpdu.UserDataHeader{
						tpdu.InformationElement{ID: 0, Data: []byte{3, 2, 1}},
					},
				},
				{
					UDH: tpdu.UserDataHeader{
						tpdu.InformationElement{ID: 0, Data: []byte{4, 2, 2}},
					},
				},
			},
			false,
		},
		// 3GPP TS 23.040 Section 9.2.3.24.1: the reference "together with the
		// originating address and Service Centre address" identifies the
		// message, so segments of different senders, recipients or types
		// never form one message.
		{
			"originator mismatch",
			[]*tpdu.TPDU{
				{
					OA:  tpdu.NewAddress(tpdu.FromNumber("111")),
					UDH: tpdu.UserDataHeader{{ID: 0, Data: []byte{7, 2, 1}}},
				},
				{
					OA:  tpdu.NewAddress(tpdu.FromNumber("222")),
					UDH: tpdu.UserDataHeader{{ID: 0, Data: []byte{7, 2, 2}}},
				},
			},
			false,
		},
		{
			"destination mismatch",
			[]*tpdu.TPDU{
				{
					Direction:  tpdu.MO,
					FirstOctet: tpdu.FirstOctet(tpdu.MtSubmit),
					DA:         tpdu.NewAddress(tpdu.FromNumber("111")),
					UDH:        tpdu.UserDataHeader{{ID: 0, Data: []byte{7, 2, 1}}},
				},
				{
					Direction:  tpdu.MO,
					FirstOctet: tpdu.FirstOctet(tpdu.MtSubmit),
					DA:         tpdu.NewAddress(tpdu.FromNumber("222")),
					UDH:        tpdu.UserDataHeader{{ID: 0, Data: []byte{7, 2, 2}}},
				},
			},
			false,
		},
		{
			"type mismatch",
			[]*tpdu.TPDU{
				{
					UDH: tpdu.UserDataHeader{{ID: 0, Data: []byte{7, 2, 1}}},
				},
				{
					Direction:  tpdu.MO,
					FirstOctet: tpdu.FirstOctet(tpdu.MtSubmit),
					UDH:        tpdu.UserDataHeader{{ID: 0, Data: []byte{7, 2, 2}}},
				},
			},
			false,
		},
		{
			"same originator",
			[]*tpdu.TPDU{
				{
					OA:  tpdu.NewAddress(tpdu.FromNumber("111")),
					UDH: tpdu.UserDataHeader{{ID: 0, Data: []byte{7, 2, 1}}},
				},
				{
					OA:  tpdu.NewAddress(tpdu.FromNumber("111")),
					UDH: tpdu.UserDataHeader{{ID: 0, Data: []byte{7, 2, 2}}},
				},
			},
			true,
		},
		{
			"misordered segments",
			[]*tpdu.TPDU{
				{
					UDH: tpdu.UserDataHeader{
						tpdu.InformationElement{ID: 0, Data: []byte{3, 2, 2}},
					},
				},
				{
					UDH: tpdu.UserDataHeader{
						tpdu.InformationElement{ID: 0, Data: []byte{3, 2, 1}},
					},
				},
			},
			false,
		},
		{
			"missing concat",
			[]*tpdu.TPDU{
				{
					UDH: tpdu.UserDataHeader{
						tpdu.InformationElement{ID: 0, Data: []byte{3, 2, 1}},
					},
				},
				{},
			},
			false,
		},
		{
			"no concat",
			[]*tpdu.TPDU{
				{},
				{
					UDH: tpdu.UserDataHeader{
						tpdu.InformationElement{ID: 3, Data: []byte{3, 2, 2}},
					},
				},
			},
			false,
		},
		{
			"two segments",
			[]*tpdu.TPDU{
				{
					UDH: tpdu.UserDataHeader{
						tpdu.InformationElement{ID: 0, Data: []byte{3, 2, 1}},
					},
				},
				{
					UDH: tpdu.UserDataHeader{
						tpdu.InformationElement{ID: 0, Data: []byte{3, 2, 2}},
					},
				},
			},
			true,
		},
		// ignored concatenation IEs (3GPP TS 23.040 Section 9.2.3.24.1)
		{
			"zero total",
			[]*tpdu.TPDU{
				{
					UDH: tpdu.UserDataHeader{
						tpdu.InformationElement{ID: 0, Data: []byte{3, 0, 1}},
					},
				},
			},
			true,
		},
		{
			"zero seqno",
			[]*tpdu.TPDU{
				{
					UDH: tpdu.UserDataHeader{
						tpdu.InformationElement{ID: 0, Data: []byte{3, 1, 0}},
					},
				},
			},
			true,
		},
		{
			"seqno beyond total",
			[]*tpdu.TPDU{
				{
					UDH: tpdu.UserDataHeader{
						tpdu.InformationElement{ID: 8, Data: []byte{0, 3, 1, 2}},
					},
				},
			},
			true,
		},
		{
			"reference size mismatch",
			[]*tpdu.TPDU{
				{
					UDH: tpdu.UserDataHeader{
						tpdu.InformationElement{ID: 0, Data: []byte{3, 2, 1}},
					},
				},
				{
					UDH: tpdu.UserDataHeader{
						tpdu.InformationElement{ID: 8, Data: []byte{0, 3, 2, 2}},
					},
				},
			},
			false,
		},
		{
			"last concat IE used",
			[]*tpdu.TPDU{
				{
					UDH: tpdu.UserDataHeader{
						tpdu.InformationElement{ID: 0, Data: []byte{3, 2, 2}},
						tpdu.InformationElement{ID: 8, Data: []byte{1, 3, 2, 1}},
					},
				},
				{
					UDH: tpdu.UserDataHeader{
						tpdu.InformationElement{ID: 8, Data: []byte{1, 3, 2, 2}},
					},
				},
			},
			true,
		},
	}
	for _, p := range patterns {
		f := func(t *testing.T) {
			out := sms.IsCompleteMessage(p.in)
			assert.Equal(t, p.out, out)
		}
		t.Run(p.name, f)
	}
}

func TestUnmarshal(t *testing.T) {
	tz1 := time.FixedZone("SCTS", 3600)
	tz8 := time.FixedZone("SCTS", 28800)
	patterns := []struct {
		name    string
		in      []byte
		options []sms.UnmarshalOption
		out     *tpdu.TPDU
		err     error
	}{
		{
			"empty",
			nil,
			nil,
			nil,
			tpdu.NewDecodeError("tpdu.firstOctet", 0, tpdu.ErrUnderflow),
		},
		{
			"deliver single segment",
			[]byte{
				0x04, 0x04, 0x91, 0x36, 0x19, 0x00, 0x00, 0x51, 0x50, 0x71,
				0x32, 0x20, 0x05, 0x23, 0x08, 0xC8, 0x30, 0x3A, 0x8C, 0x0E,
				0xA3, 0xC3,
			},
			nil,
			&tpdu.TPDU{
				FirstOctet: 0x04,
				OA:         tpdu.Address{TOA: 0x91, Addr: "6391"},
				SCTS: tpdu.Timestamp{
					Time: time.Date(2015, 5, 17, 23, 2, 50, 0, tz8),
				},
				UD: []byte("Hahahaha"),
			},
			nil,
		},
		{
			"submit single segment",
			[]byte{
				0x31, 0x07, 0x04, 0x91, 0x36, 0x19, 0x00, 0x00, 0xA9, 0x08,
				0xC8, 0x30, 0x3A, 0x8C, 0x0E, 0xA3, 0xC3,
			},
			[]sms.UnmarshalOption{sms.AsMO},
			&tpdu.TPDU{
				Direction:  1,
				FirstOctet: 0x31,
				MR:         0x07,
				DA:         tpdu.Address{TOA: 0x91, Addr: "6391"},
				VP: tpdu.ValidityPeriod{
					Format:   tpdu.VpfRelative,
					Duration: time.Hour * 72,
				},
				UD: []byte("Hahahaha"),
			},
			nil,
		},
		{
			"submitreport",
			[]byte{
				0x06, 0x18, 0x04, 0x91, 0x36, 0x19, 0x11, 0x10, 0x11, 0x71,
				0x95, 0x51, 0x40, 0x11, 0x10, 0x11, 0x71, 0x95, 0x71, 0x40,
				0x00,
			},
			nil,
			&tpdu.TPDU{
				FirstOctet: 0x06,
				MR:         0x18,
				RA:         tpdu.Address{TOA: 0x91, Addr: "6391"},
				SCTS: tpdu.Timestamp{
					Time: time.Date(2011, 1, 11, 17, 59, 15, 0, tz1),
				},
				DT: tpdu.Timestamp{
					Time: time.Date(2011, 1, 11, 17, 59, 17, 0, tz1),
				},
			},
			nil,
		},
		{
			"deliverreport rp-ack",
			[]byte{0x00, 0x01, 0x7f},
			[]sms.UnmarshalOption{sms.AsMO, sms.AsRPAck},
			&tpdu.TPDU{
				Direction: tpdu.MO,
				PI:        tpdu.PiPID,
				PID:       0x7f,
			},
			nil,
		},
		{
			"deliverreport rp-error",
			[]byte{0x00, 0xd0, 0x01, 0x7f},
			[]sms.UnmarshalOption{sms.AsMO, sms.AsRPError},
			&tpdu.TPDU{
				Direction: tpdu.MO,
				RPMessage: tpdu.RPError,
				FCS:       0xd0,
				PI:        tpdu.PiPID,
				PID:       0x7f,
			},
			nil,
		},
	}
	for _, p := range patterns {
		f := func(t *testing.T) {
			out, err := sms.Unmarshal(p.in, p.options...)
			assert.Equal(t, p.err, err)
			assert.Equal(t, p.out, out)
		}
		t.Run(p.name, f)
	}
}
