// SPDX-License-Identifier: MIT

package sms_test

import (
	"strings"
	"testing"

	"github.com/gomaja/go-sms"
	"github.com/gomaja/go-sms/encoding/gsm7/charset"
	"github.com/gomaja/go-sms/encoding/tpdu"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// portIE is an Application Port Addressing 16 bit address IE, as defined in
// 3GPP TS 23.040 Section 9.2.3.24.4, for WAP push.
var portIE = tpdu.InformationElement{ID: 0x05, Data: []byte{0x0b, 0x84, 0x23, 0xf0}}

func lockingIE(nli int) tpdu.InformationElement {
	return tpdu.InformationElement{ID: tpdu.IEINationalLanguageLockingShift, Data: []byte{byte(nli)}}
}

func shiftIE(nli int) tpdu.InformationElement {
	return tpdu.InformationElement{ID: tpdu.IEINationalLanguageSingleShift, Data: []byte{byte(nli)}}
}

func concatIE(ref, total, seqno byte) tpdu.InformationElement {
	return tpdu.InformationElement{ID: tpdu.IEIConcat8Bit, Data: []byte{ref, total, seqno}}
}

// receive marshals the TPDUs and returns them as unmarshalled by their
// receiver.
func receive(t *testing.T, pdus []tpdu.TPDU) []*tpdu.TPDU {
	t.Helper()
	out := make([]*tpdu.TPDU, len(pdus))
	for i := range pdus {
		b, err := pdus[i].MarshalBinary()
		require.NoError(t, err)
		opts := []sms.UnmarshalOption{}
		if pdus[i].Direction == tpdu.MO {
			opts = append(opts, sms.AsMO)
		}
		out[i], err = sms.Unmarshal(b, opts...)
		require.NoError(t, err)
	}
	return out
}

// The Encoder chooses the character sets on the GSM 7 bit path, so it
// replaces any national language IEs in the template UDH by those of the
// tables it chose, and keeps every other IE of the template UDH in every
// segment, as 3GPP TS 23.040 Section 9.2.3.24.4 requires of the port IE:
// "The IEI, its associated IEI length and IEI data shall also be contained in
// every subsequent segment of the concatenated SM."
func TestEncodeKeepsTemplateUDH(t *testing.T) {
	long := strings.Repeat("ت", 200)
	patterns := []struct {
		name    string
		msg     string
		udh     tpdu.UserDataHeader
		options []sms.EncoderOption
		dcs     tpdu.DCS
		out     []tpdu.UserDataHeader
	}{
		{
			"port and locking shift",
			"hello ت",
			tpdu.UserDataHeader{portIE},
			[]sms.EncoderOption{sms.WithCharset(charset.Urdu)},
			0,
			[]tpdu.UserDataHeader{{portIE, lockingIE(charset.Urdu)}},
		},
		{
			"port and locking and single shift",
			"hello ت؎",
			tpdu.UserDataHeader{portIE},
			[]sms.EncoderOption{sms.WithAllCharsets},
			0,
			[]tpdu.UserDataHeader{{portIE, lockingIE(charset.Urdu), shiftIE(charset.Urdu)}},
		},
		{
			"port and default alphabet",
			"hello",
			tpdu.UserDataHeader{portIE},
			[]sms.EncoderOption{sms.WithCharset(charset.Urdu)},
			0,
			[]tpdu.UserDataHeader{{portIE}},
		},
		{
			"port and ucs2 fallback",
			"hello 😁",
			tpdu.UserDataHeader{portIE},
			[]sms.EncoderOption{sms.WithCharset(charset.Urdu)},
			tpdu.DcsUCS2Data,
			[]tpdu.UserDataHeader{{portIE}},
		},
		{
			"port and locking shift concatenated",
			long,
			tpdu.UserDataHeader{portIE},
			[]sms.EncoderOption{sms.WithCharset(charset.Urdu)},
			0,
			[]tpdu.UserDataHeader{
				{portIE, lockingIE(charset.Urdu), concatIE(1, 2, 1)},
				{portIE, lockingIE(charset.Urdu), concatIE(1, 2, 2)},
			},
		},
		{
			"stale locking shift replaced",
			"hello ت",
			tpdu.UserDataHeader{lockingIE(charset.Turkish), portIE},
			[]sms.EncoderOption{sms.WithCharset(charset.Urdu)},
			0,
			[]tpdu.UserDataHeader{{portIE, lockingIE(charset.Urdu)}},
		},
		{
			"stale national language IEs dropped for default alphabet",
			"hello Ø",
			tpdu.UserDataHeader{lockingIE(charset.Turkish), portIE, shiftIE(charset.Turkish)},
			nil,
			0,
			[]tpdu.UserDataHeader{{portIE}},
		},
		{
			"stale national language IE dropped leaving no UDH",
			"hello Ø",
			tpdu.UserDataHeader{lockingIE(charset.Turkish)},
			nil,
			0,
			[]tpdu.UserDataHeader{nil},
		},
		{
			"stale national language IE dropped for ucs2 fallback",
			"hello 😁",
			tpdu.UserDataHeader{shiftIE(charset.Turkish)},
			nil,
			tpdu.DcsUCS2Data,
			[]tpdu.UserDataHeader{nil},
		},
		{
			"empty template UDH kept",
			"hello",
			tpdu.UserDataHeader{},
			nil,
			0,
			[]tpdu.UserDataHeader{{}},
		},
	}
	for _, p := range patterns {
		f := func(t *testing.T) {
			tmpl := make(tpdu.UserDataHeader, len(p.udh), len(p.udh)+4)
			copy(tmpl, p.udh)
			options := append([]sms.EncoderOption{
				sms.To("1234"),
				sms.WithTemplateOption(tpdu.WithUDH(tmpl)),
			}, p.options...)
			out, err := sms.Encode([]byte(p.msg), options...)
			require.NoError(t, err)
			require.Len(t, out, len(p.out))
			for i := range out {
				assert.Equal(t, p.out[i], out[i].UDH, "segment %d", i+1)
				assert.Equal(t, p.out[i] != nil, out[i].UDHI(), "segment %d", i+1)
				assert.Equal(t, p.dcs, out[i].DCS, "segment %d", i+1)
			}
			// the template is not changed
			assert.Equal(t, p.udh, tmpl)
			// and the receiver decodes the message
			msg, err := sms.Decode(receive(t, out))
			require.NoError(t, err)
			assert.Equal(t, p.msg, string(msg))
		}
		t.Run(p.name, f)
	}
}

// With an explicit alphabet the message is not coded by the Encoder, so the
// template UDH is used as it is.
func TestEncodeExplicitAlphabetKeepsTemplateUDH(t *testing.T) {
	udh := tpdu.UserDataHeader{lockingIE(charset.Turkish), portIE}
	for _, alpha := range []sms.EncoderOption{sms.As8Bit, sms.AsUCS2} {
		out, err := sms.Encode([]byte("ab"), alpha, sms.WithTemplateOption(tpdu.WithUDH(udh)))
		require.NoError(t, err)
		require.Len(t, out, 1)
		assert.Equal(t, udh, out[0].UDH)
	}
}
