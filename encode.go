// SPDX-License-Identifier: MIT

package sms

import (
	"slices"
	"sync/atomic"

	"github.com/gomaja/go-sms/encoding/tpdu"
)

// Encode builds a set of TPDUs containing the message.
//
// Long messages are split into multiple concatenated TPDUs, while short
// messages may fit in one.
//
// By default messages are encoded into SMS-SUBMIT TPDUs.  This behaviour may
// be overridden via options.
//
// For 8-bit encoding the message is encoded as is.
//
// For 7-bit encoding the message is assumed to contain UTF-8.
//
// For explicit UCS-2 encoding the message is assumed to contain UTF-16,
// encoded as an array of bytes.  This can be created from an array of UTF-16
// runes using ucs2.Encode.
//
// For implicit UCS-2 encoding (the fallback with 7-bit fails) the message is
// assumed to contain UTF-8.
func Encode(msg []byte, options ...EncoderOption) ([]tpdu.TPDU, error) {
	options = append([]EncoderOption{AsSubmit}, options...)
	e := NewEncoder(options...)
	return e.Encode(msg)
}

// Encoder builds SMS TPDUs from simple inputs such as the destination number
// and the message in a UTF8 form.
//
// An Encoder is safe for concurrent use by multiple goroutines, provided its
// fields are not changed once it is in use and its counters are themselves
// safe for concurrent use, as Counter is.
type Encoder struct {
	// options for encoding UD
	eopts []tpdu.UDEncodeOption

	// options for segmentation
	sopts []tpdu.SegmentationOption

	// The template TPDU for encoding.
	pdu tpdu.TPDU

	err error

	// MsgCount is the number of TPDUs encoded.
	MsgCount tpdu.Counter

	// ConcatRef is the number of multi-segment messages encoded.
	ConcatRef tpdu.Counter
}

// NewEncoder creates an Encoder.
func NewEncoder(options ...EncoderOption) *Encoder {
	e := Encoder{}
	for _, option := range options {
		option.ApplyEncoderOption(&e)
	}
	// The options may hold slices the caller goes on to change.
	e.pdu = cloneTPDU(&e.pdu)
	if e.MsgCount == nil {
		e.MsgCount = &Counter{}
	}
	if e.ConcatRef == nil {
		e.ConcatRef = &Counter{}
	}
	return &e
}

// Encode builds a set of TPDUs containing the message.
//
// Long messages are split into multiple concatenated TPDUs, while short
// messages may fit in one.
//
// By default messages are encoded into SMS-DELIVER TPDUs.  This behaviour may
// be overridden via options, either to NewEncoder or Encode.
//
// For 8-bit encoding the message is encoded as is.
//
// For 7-bit encoding the message is assumed to contain UTF-8.
//
// For explicit UCS-2 encoding the message is assumed to contain UTF-16,
// encoded as an array of bytes.  This can be created from an array of UTF-16
// runes using ucs2.Encode.
//
// For implicit UCS-2 encoding (the fallback with 7-bit fails) the message is
// assumed to contain UTF-8.
//
// When the Encoder chooses the alphabet, it also chooses the national language
// tables, so it replaces any National Language Single Shift or Locking Shift
// IE in the template UDH by those of the tables it chose, as defined in 3GPP
// TS 23.040 Sections 9.2.3.24.15 and 9.2.3.24.16. The other IEs of the
// template UDH, such as application port addressing, are kept in every TPDU.
//
// The TPDUs returned share no memory with the Encoder, its template or the
// message, so the caller may change them, and reuse the message.
func (e Encoder) Encode(msg []byte, options ...EncoderOption) ([]tpdu.TPDU, error) {
	// e is a copy, but its slices share their backing arrays with the
	// Encoder, which may be in use by other goroutines, so appending to them
	// must allocate.
	e.eopts = slices.Clip(e.eopts)
	e.sopts = slices.Clip(e.sopts)
	for _, option := range options {
		option.ApplyEncoderOption(&e)
	}
	if e.err != nil {
		return nil, e.err
	}
	pdus, err := e.segment(msg)
	if err != nil {
		return nil, err
	}
	// The segments share the template UDH and the message, or its GSM7
	// coding, whose segments share one backing array.
	for i := range pdus {
		pdus[i] = cloneTPDU(&pdus[i])
	}
	return pdus, nil
}

// segment codes the message, if required, and segments it into TPDUs.
func (e *Encoder) segment(msg []byte) ([]tpdu.TPDU, error) {
	sopts := append(e.sopts, tpdu.WithMR(e.MsgCount), tpdu.WithConcatRef(e.ConcatRef))
	// take the DCS in the template TPDU as a hint...
	alpha := e.pdu.DCS.Alphabet()
	switch alpha {
	case tpdu.Alpha8Bit, tpdu.AlphaUCS2:
		return e.pdu.Segment(msg, sopts...)
	default:
		// encode as GSM7, or failing that UCS2...
		d, udh, alpha, err := tpdu.EncodeUserData(msg, e.eopts...)
		if err != nil {
			return nil, err
		}
		dcs, err := e.pdu.DCS.WithAlphabet(alpha)
		if err != nil {
			return nil, ErrDcsConflict
		}
		if dcs != e.pdu.DCS {
			e.pdu.SetDCS(byte(dcs))
		}
		if h, changed := withNationalLanguage(e.pdu.UDH, udh); changed {
			e.pdu.SetUDH(h)
		}
		return e.pdu.Segment(d, sopts...)
	}
}

// withNationalLanguage returns the template UDH with its national language
// IEs replaced by those in nl, and whether that changed it.
//
// The other IEs of the template, such as application port addressing, are
// kept in order, and the national language IEs follow them. The result is a
// new slice, so the template is not changed. If no IE remains then the
// result is nil, unless the template UDH was empty but not nil and had no IE
// to remove, as that is then unchanged.
func withNationalLanguage(udh, nl tpdu.UserDataHeader) (tpdu.UserDataHeader, bool) {
	h := make(tpdu.UserDataHeader, 0, len(udh)+len(nl))
	for _, ie := range udh {
		if ie.ID != tpdu.IEINationalLanguageLockingShift &&
			ie.ID != tpdu.IEINationalLanguageSingleShift {
			h = append(h, ie)
		}
	}
	if len(h) == len(udh) && len(nl) == 0 {
		return udh, false
	}
	h = append(h, nl...)
	if len(h) == 0 {
		return nil, true
	}
	return h, true
}

// Counter is an implementation of the tpdu.Counter interface.
//
// It also provides a Read method on the current value for diagnostic purposes.
type Counter struct {
	c int64
}

// Count increments and returns the counter.
func (c *Counter) Count() int {
	return int(atomic.AddInt64(&c.c, 1))
}

// Read returns the counter.
func (c *Counter) Read() int {
	return int(atomic.LoadInt64(&c.c))
}
