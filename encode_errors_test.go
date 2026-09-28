// SPDX-License-Identifier: MIT

package sms_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/gomaja/go-sms"
	"github.com/gomaja/go-sms/encoding/tpdu"
	"github.com/stretchr/testify/assert"
)

// The errors of tpdu.TPDU.Segment are returned by the Encoder, without any
// TPDU, and no counter is drawn from.
func TestEncodeReturnsSegmentErrors(t *testing.T) {
	bigUDH := tpdu.WithUDH(tpdu.UserDataHeader{{ID: 0x70, Data: make([]byte, 134)}})
	patterns := []struct {
		name    string
		msg     []byte
		options []sms.EncoderOption
		field   string
		err     error
	}{
		{
			"odd ucs2",
			[]byte{0x00, 0x41, 0x00},
			[]sms.EncoderOption{sms.AsUCS2},
			"sm",
			tpdu.ErrOddUCS2Length,
		},
		{
			"odd ucs2 concatenated",
			make([]byte, 141),
			[]sms.EncoderOption{sms.AsUCS2},
			"sm",
			tpdu.ErrOddUCS2Length,
		},
		{
			"template udh leaves no room for 8 bit",
			make([]byte, 140),
			[]sms.EncoderOption{sms.As8Bit, sms.WithTemplateOption(bigUDH)},
			"udh",
			tpdu.ErrOverlength,
		},
		{
			"template udh leaves no room for coded text",
			[]byte(strings.Repeat("a", 300)),
			[]sms.EncoderOption{sms.WithTemplateOption(bigUDH)},
			"udh",
			tpdu.ErrOverlength,
		},
		{
			"too many 8 bit segments",
			make([]byte, 134*255+1),
			[]sms.EncoderOption{sms.As8Bit},
			"sm",
			tpdu.ErrTooManySegments,
		},
		{
			"too many coded segments",
			[]byte(strings.Repeat("a", 153*255+1)),
			nil,
			"sm",
			tpdu.ErrTooManySegments,
		},
	}
	for _, p := range patterns {
		f := func(t *testing.T) {
			mr := &sms.Counter{}
			ref := &sms.Counter{}
			options := append([]sms.EncoderOption{sms.WithMR(mr), sms.WithConcatRef(ref)}, p.options...)
			out, err := sms.Encode(p.msg, options...)
			assert.Nil(t, out)
			assert.True(t, errors.Is(err, p.err), "%v", err)
			var ee tpdu.EncodeError
			if assert.True(t, errors.As(err, &ee), "%v", err) {
				assert.Equal(t, p.field, ee.Field)
			}
			out, err = sms.NewEncoder(options...).Encode(p.msg)
			assert.Nil(t, out)
			assert.True(t, errors.Is(err, p.err), "%v", err)
			assert.Zero(t, mr.Read())
			assert.Zero(t, ref.Read())
		}
		t.Run(p.name, f)
	}
	// A reserved TP-MTI in the MO direction is no TPDU type.
	mr := &sms.Counter{}
	out, err := sms.Encode([]byte("hi"), sms.WithMR(mr),
		sms.WithTemplateOption(tpdu.MO), sms.WithTemplateOption(tpdu.MtReserved))
	assert.Nil(t, out)
	var ust tpdu.ErrUnsupportedSmsType
	assert.True(t, errors.As(err, &ust), "%v", err)
	assert.Zero(t, mr.Read())
}
