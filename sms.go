// SPDX-License-Identifier: MIT

// Package sms provides encoders and decoders for SMS PDUs.
package sms

import (
	"errors"
	"slices"
	"unicode/utf8"

	"github.com/gomaja/go-sms/encoding/tpdu"
	"github.com/gomaja/go-sms/encoding/ucs2"
)

// DecodeConfig contains configuration option for Decode.
type DecodeConfig struct {
	dopts []tpdu.UDDecodeOption
}

// Decode returns the UTF-8 message contained in a set of TPDUs.
//
// For concatenated messages the segments are assumed to be the component
// TPDUs, in order. This is the case for segments returned by the Collector,
// and can be tested using IsCompleteMessage.
//
// A nil segment, as the Collector gives for a segment that was not received,
// cannot be decoded, so ErrMissingSegment is returned.
//
// A UTF-16 surrogate pair split between two consecutive UCS2 segments is
// decoded as the one character it codes. Any other surrogate is unpaired and
// decoded as U+FFFD, as ucs2.Decode does, including a high surrogate that
// ends the last segment or that is followed by a segment that does not start
// with a low surrogate.
func Decode(segments []*tpdu.TPDU, options ...DecodeOption) ([]byte, error) {
	cfg := DecodeConfig{}
	for _, option := range options {
		option.ApplyDecodeOption(&cfg)
	}
	if len(cfg.dopts) == 0 {
		cfg.dopts = []tpdu.UDDecodeOption{tpdu.WithAllCharsets}
	}
	n := 0
	for _, s := range segments {
		if s == nil {
			return nil, ErrMissingSegment
		}
		// An SMS-COMMAND has no TP-DCS, so its TP-CD is never compressed.
		if s.SmsType() != tpdu.SmsCommand && s.DCS.Compressed() {
			return nil, ErrCompressedUserData
		}
		n += len(s.UD)
	}
	m := make([]byte, 0, n)
	// dangling holds a high surrogate that ended the previous segment, which
	// the low surrogate at the start of the next segment may complete.
	var dangling ucs2.ErrDanglingSurrogate
	for _, s := range segments {
		a := s.Alphabet()
		ud := s.UD
		if dangling != nil {
			if a == tpdu.AlphaUCS2 {
				ud = append(tpdu.UserData(dangling), ud...)
			} else {
				m = utf8.AppendRune(m, utf8.RuneError)
			}
			dangling = nil
		}
		d, err := tpdu.DecodeUserData(ud, s.UDH, a, cfg.dopts...)
		if err != nil && !errors.As(err, &dangling) {
			return nil, err
		}
		m = append(m, d...)
	}
	if dangling != nil {
		m = utf8.AppendRune(m, utf8.RuneError)
	}
	return m, nil
}

// IsCompleteMessage confirms that the TPDUs contain all the segments required
// to reassemble a complete message and are in the correct order.
//
// It returns false if any segment is nil, or if the segments differ in type
// or address, as the reference number only identifies a message "together
// with the originating address and Service Centre address" (3GPP TS 23.040
// Section 9.2.3.24.1). For an SMS-SUBMIT the originator is not in the TPDU,
// so the caller must ensure the segments come from one originator, as the
// Collector does with WithOriginator.
func IsCompleteMessage(segments []*tpdu.TPDU) bool {
	if len(segments) == 0 || slices.Contains(segments, nil) {
		return false
	}
	base, ok := segments[0].ConcatInfo()
	if !ok {
		return len(segments) == 1
	}
	if base.Total != len(segments) {
		return false
	}
	first := segments[0]
	for i, s := range segments {
		if s.SmsType() != first.SmsType() || s.OA != first.OA || s.DA != first.DA {
			return false
		}
		ci, ok := s.ConcatInfo()
		if !ok {
			return false
		}
		if ci.Total != base.Total {
			return false
		}
		if ci.Ref != base.Ref || ci.Ref16Bit != base.Ref16Bit {
			return false
		}
		if ci.Seqno != i+1 {
			return false
		}
	}
	return true
}

// cloneTPDU returns a copy of the TPDU that shares no memory with it.
//
// Its slices keep their nil or empty state, as that matters when marshalling:
// an empty UDH is marshalled as a TP-UDHL of 0, while a nil one is not
// marshalled.
func cloneTPDU(t *tpdu.TPDU) tpdu.TPDU {
	c := *t
	c.PIExt = slices.Clone(t.PIExt)
	c.UD = slices.Clone(t.UD)
	if t.UDH != nil {
		c.UDH = make(tpdu.UserDataHeader, len(t.UDH))
		for i, ie := range t.UDH {
			c.UDH[i] = tpdu.InformationElement{ID: ie.ID, Data: slices.Clone(ie.Data)}
		}
	}
	return c
}

// UnmarshalConfig contains configuration options for Unmarshal.
type UnmarshalConfig struct {
	dirn tpdu.Direction
	rp   tpdu.RPMessage
}

// Unmarshal converts a binary SMS TPDU into the corresponding TPDU object.
//
// The TPDU is assumed to be MT, and a report to be carried by an RP-ACK,
// unless the options say otherwise.
func Unmarshal(src []byte, options ...UnmarshalOption) (*tpdu.TPDU, error) {
	cfg := UnmarshalConfig{}
	for _, option := range options {
		option.ApplyUnmarshalOption(&cfg)
	}
	t := tpdu.TPDU{Direction: cfg.dirn, RPMessage: cfg.rp}
	err := t.UnmarshalBinary(src)
	if err != nil {
		return nil, err
	}
	return &t, nil
}
