// SPDX-License-Identifier: MIT

package sms

import (
	"math/rand/v2"
	"slices"
	"sync/atomic"

	"github.com/gomaja/go-sms/encoding/tpdu"
)

// Encode builds a set of TPDUs containing the message.
//
// It is the same as NewEncoder(options...).Encode(msg), except that the
// TPDUs are SMS-SUBMIT unless the options say otherwise.
//
// The TP-MR and concatenation references are drawn from the counters shared
// by all Encoders created without WithMR or WithConcatRef, as described for
// NewEncoder, so consecutive calls do not reuse a reference.
func Encode(msg []byte, options ...EncoderOption) ([]tpdu.TPDU, error) {
	options = append([]EncoderOption{AsSubmit}, options...)
	e := NewEncoder(options...)
	return e.Encode(msg)
}

// The counters shared by the Encoders created without WithMR or
// WithConcatRef.
//
// The reference counter starts at a random value, so that the references of
// different runs of a program are unlikely to repeat. It covers the range of
// the 16-bit references, and so of the 8-bit ones. The value is not a secret,
// so a cryptographic source is not needed.
var (
	sharedMR        = &Counter{}
	sharedConcatRef = NewCounter(rand.IntN(1 << 16))
)

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

	// MsgCount provides the TP-MR of each SMS-SUBMIT and SMS-COMMAND TPDU
	// encoded.
	MsgCount tpdu.Counter

	// ConcatRef provides the reference of each concatenated message encoded.
	ConcatRef tpdu.Counter
}

// NewEncoder creates an Encoder.
//
// The Encoder draws the TP-MR of each SMS-SUBMIT and SMS-COMMAND TPDU from
// the counter given by WithMR, and the reference of each concatenated message
// from the counter given by WithConcatRef. Without them it draws from
// counters shared by all such Encoders, and by Encode, so that consecutive
// concatenated messages get different references, which 3GPP TS 23.040
// Section 9.2.3.24.1 requires to tell them apart, even when an Encoder is
// created for each message. The shared reference counter starts at a random
// value, so that different runs of a program, which each start their
// counters afresh, are unlikely to reuse a reference. The shared TP-MR
// counter starts at 0, so the first TP-MR is 1.
//
// An MS continues the TP-MR from the LastUsedTPMR held by its (U)SIM, as
// Section 9.2.3.6 requires, which WithMR(NewCounter(lastUsedTPMR)) provides.
// An application sending on behalf of several originators needs a TP-MR
// counter for each of them.
//
// Only the originator of an SMS-SUBMIT or SMS-COMMAND allocates a TP-MR
// (Section 9.2.3.6), so a TPDU of another type keeps the TP-MR of the
// template, and draws none, as tpdu.TPDU.Segment describes. That is the
// TP-MR of the SMS-SUBMIT or SMS-COMMAND an SMS-STATUS-REPORT reports on,
// which the template must hold, and the other types have no TP-MR.
func NewEncoder(options ...EncoderOption) *Encoder {
	e := Encoder{}
	for _, option := range options {
		option.ApplyEncoderOption(&e)
	}
	// The options may hold slices the caller goes on to change.
	e.pdu = cloneTPDU(&e.pdu)
	if e.MsgCount == nil {
		e.MsgCount = sharedMR
	}
	if e.ConcatRef == nil {
		e.ConcatRef = sharedConcatRef
	}
	return &e
}

// Encode builds a set of TPDUs containing the message.
//
// The TPDUs are copies of the template, an SMS-DELIVER unless the options say
// otherwise, each carrying a segment of the message in its UD. A message that
// fits in one TPDU, including an empty message, results in one TPDU, and a
// longer one is split into concatenated segments, as for
// tpdu.TPDU.Segment. The options apply to this call only.
//
// How the message is taken depends on the alphabet of the template:
//
//   - 8 bit, which the TP-CD of an SMS-COMMAND always is, whatever the DCS:
//     the message is octets, and is used as it is.
//   - UCS2: the message is UTF-16, big endian, so two octets for each code
//     point up to U+FFFF and four, a surrogate pair, for each above it, as
//     ucs2.Encode returns for a slice of code points. It is used as it is.
//   - GSM 7 bit, which includes the reserved codings: the message is UTF-8,
//     which the Encoder codes in the GSM 7 bit default alphabet, or in the
//     national language tables that the charset options make available, or,
//     failing those, in UCS2, and sets the alphabet of the DCS to match.
//     tpdu.ErrInvalidUTF8 is returned if the message is not valid UTF-8, and
//     ErrDcsConflict if the DCS cannot indicate the alphabet.
//
// The Encoder does not compress, so ErrCompressedUserData is returned if the
// DCS of the template, or the DCS the Encoder would send, indicates compressed
// data, as defined in 3GPP TS 23.038 Section 4.
//
// When the Encoder chooses the alphabet, it also chooses the national language
// tables, so it replaces any National Language Single Shift or Locking Shift
// IE in the template UDH by those of the tables it chose, as defined in 3GPP
// TS 23.040 Sections 9.2.3.24.15 and 9.2.3.24.16. The other IEs of the
// template UDH, such as application port addressing, are kept in every TPDU.
//
// The errors of tpdu.TPDU.Segment are returned too, and can be matched with
// errors.Is or errors.As: tpdu.ErrOddUCS2Length for a UCS2 message of odd length,
// tpdu.ErrOverlength if the template UDH leaves no room for the message,
// tpdu.ErrTooManySegments if the message needs more than 255 segments, and a
// tpdu.ErrUnsupportedSmsType if the template is of no TPDU type. No TPDU is
// returned with an error, and no counter is drawn from.
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
	// The Encoder does not compress, so it cannot provide the compressed
	// data such a DCS indicates.
	if e.pdu.SmsType() != tpdu.SmsCommand && e.pdu.DCS.Compressed() {
		return nil, ErrCompressedUserData
	}
	sopts := append(e.sopts, tpdu.WithMR(e.MsgCount), tpdu.WithConcatRef(e.ConcatRef))
	// take the alphabet of the template TPDU as a hint, which, for an
	// SMS-COMMAND, is always 8 bit...
	alpha := e.pdu.Alphabet()
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
		// A reserved coding in the template is read as 0x00, which is not
		// compressed, but setting its alphabet makes its other bits count,
		// so the DCS that is sent must be checked too.
		if dcs.Compressed() {
			return nil, ErrCompressedUserData
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

// Counter is an implementation of the tpdu.Counter interface, which is safe
// for concurrent use.
//
// Its zero value has counted to 0, so its first Count returns 1.
//
// It also provides a Read method on the current value for diagnostic purposes.
type Counter struct {
	c int64
}

// NewCounter returns a Counter that has counted to last, so its first Count
// returns last+1.
//
// To continue the TP-MR of a (U)SIM, as 3GPP TS 23.040 Section 9.2.3.6
// requires, last is its LastUsedTPMR.
func NewCounter(last int) *Counter {
	return &Counter{c: int64(last)}
}

// Count increments and returns the counter.
func (c *Counter) Count() int {
	return int(atomic.AddInt64(&c.c, 1))
}

// Read returns the value the counter has counted to, which is the value last
// returned by Count.
func (c *Counter) Read() int {
	return int(atomic.LoadInt64(&c.c))
}
