// SPDX-License-Identifier: MIT

package sms_test

import (
	"testing"

	"github.com/gomaja/go-sms"
	"github.com/gomaja/go-sms/encoding/tpdu"
	"github.com/stretchr/testify/assert"
)

// The library does not compress, so a template DCS that indicates
// compression, as defined in 3GPP TS 23.038 Section 4, is rejected rather
// than sending uncompressed text that claims to be compressed.
func TestEncodeRejectsCompressedDCS(t *testing.T) {
	for _, dcs := range []tpdu.DCS{
		0x20, // compressed, GSM 7 bit, so the Encoder codes the message
		0x24, // compressed, 8 bit
		0x28, // compressed, UCS2
		0x34, // compressed, 8 bit, class 0
		0x60, // automatic deletion group, compressed, GSM 7 bit
		0x6c, // automatic deletion group, compressed, reserved
	} {
		opt := sms.WithTemplateOption(dcs)
		out, err := sms.Encode([]byte("hello"), opt)
		assert.Equal(t, sms.ErrCompressedUserData, err, "Encode %s", dcs)
		assert.Nil(t, out, "Encode %s", dcs)

		e := sms.NewEncoder(sms.AsSubmit, opt)
		out, err = e.Encode([]byte("hello"))
		assert.Equal(t, sms.ErrCompressedUserData, err, "NewEncoder %s", dcs)
		assert.Nil(t, out, "NewEncoder %s", dcs)

		e = sms.NewEncoder(sms.AsSubmit)
		out, err = e.Encode([]byte("hello"), opt)
		assert.Equal(t, sms.ErrCompressedUserData, err, "Encoder.Encode %s", dcs)
		assert.Nil(t, out, "Encoder.Encode %s", dcs)

		tmpl := tpdu.TPDU{Direction: tpdu.MO, FirstOctet: tpdu.FirstOctet(tpdu.MtSubmit), DCS: dcs}
		out, err = sms.Encode([]byte("hello"), sms.WithTemplate(tmpl))
		assert.Equal(t, sms.ErrCompressedUserData, err, "WithTemplate %s", dcs)
		assert.Nil(t, out, "WithTemplate %s", dcs)
	}
	// Bit 5 only means compression in the general data coding groups.
	for _, dcs := range []tpdu.DCS{0x00, 0x04, 0x08, 0x14, 0xf4, 0xe0} {
		// an even length, so it is also valid UTF-16 for the UCS2 codings
		_, err := sms.Encode([]byte("hell"), sms.WithTemplateOption(dcs))
		assert.NoError(t, err, "%s", dcs)
	}
	// An SMS-COMMAND has no TP-DCS, so a DCS left in its template is not
	// marshalled and does not make its TP-CD compressed.
	out, err := sms.Encode([]byte("cd"),
		sms.WithTemplateOption(tpdu.SmsCommand), sms.WithTemplateOption(tpdu.DCS(0x24)))
	assert.NoError(t, err)
	assert.Len(t, out, 1)
}

// The TP-CD of an SMS-COMMAND is octets, as 3GPP TS 23.040 Section 9.2.3.20
// says: "The TP-Command-Data-Length field is used to indicate the number of
// octets contained within the TP-Command-Data field", so the message is used
// as it is, whatever it holds, rather than coded as text.
func TestEncodeCommandData(t *testing.T) {
	for _, cd := range [][]byte{
		[]byte("hé"),
		{0xff, 0x00, 0x1b},
		{},
	} {
		out, err := sms.Encode(cd, sms.WithTemplateOption(tpdu.SmsCommand), sms.To("1234"))
		if !assert.NoError(t, err, "% x", cd) || !assert.Len(t, out, 1, "% x", cd) {
			continue
		}
		assert.Equal(t, tpdu.SmsCommand, out[0].SmsType())
		assert.Equal(t, tpdu.DCS(0), out[0].DCS, "% x", cd)
		if len(cd) == 0 {
			assert.Nil(t, out[0].UD)
		} else {
			assert.Equal(t, tpdu.UserData(cd), out[0].UD, "% x", cd)
		}
		msg, err := sms.Decode(receive(t, out))
		assert.NoError(t, err, "% x", cd)
		assert.Equal(t, cd, msg, "% x", cd)
	}
}
