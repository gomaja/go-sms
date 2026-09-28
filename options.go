// SPDX-License-Identifier: MIT

package sms

import (
	"slices"

	"github.com/gomaja/go-sms/encoding/tpdu"
)

// EncoderOption is an optional mutator for the Encoder.
type EncoderOption interface {
	ApplyEncoderOption(*Encoder)
}

// DecodeOption defines options for Decode.
type DecodeOption interface {
	ApplyDecodeOption(*DecodeConfig)
}

// UnmarshalOption defines options for Unmarhsal.
type UnmarshalOption interface {
	ApplyUnmarshalOption(*UnmarshalConfig)
}

// WithTemplate specifies the TPDU to be used as the template for encoding.
func WithTemplate(t tpdu.TPDU) EncoderOption {
	return tpduTemplate{t}
}

type tpduTemplate struct {
	t tpdu.TPDU
}

func (o tpduTemplate) ApplyEncoderOption(e *Encoder) {
	e.pdu = o.t
}

type templateOption struct {
	tpdu.Option
}

func (o templateOption) ApplyEncoderOption(e *Encoder) {
	if e.err != nil {
		return
	}
	if err := o.ApplyTPDUOption(&e.pdu); err != nil {
		e.err = err
	}
}

// WithTemplateOption wraps a TPDU option in a TemplateOption so it can be
// applied to an Encoder template PDU.
func WithTemplateOption(option tpdu.Option) EncoderOption {
	return templateOption{option}
}

var (
	// AsSubmit indicates that generated PDUs will be of type SmsSubmit.
	AsSubmit = templateOption{tpdu.SmsSubmit}

	// AsDeliver indicates that generated PDUs will be of type SmsDeliver.
	AsDeliver = templateOption{tpdu.SmsDeliver}

	// As8Bit indicates that generated PDUs encode user data as 8bit.
	As8Bit = templateOption{tpdu.Dcs8BitData}

	// AsUCS2 indicates that generated PDUs encode user data as UCS2.
	AsUCS2 = templateOption{tpdu.DcsUCS2Data}

	// AsMO indicates that the TPDU originated from the mobile station.
	AsMO = directionOption{tpdu.MO}

	// AsMT indicates that the TPDU as destined for the mobile station.
	AsMT = directionOption{tpdu.MT}

	// AsRPAck indicates that an SMS-DELIVER-REPORT or SMS-SUBMIT-REPORT is
	// carried by an RP-ACK, so has no TP-FCS.
	//
	// This is the default.
	AsRPAck = rpMessageOption{tpdu.RPAck}

	// AsRPError indicates that an SMS-DELIVER-REPORT or SMS-SUBMIT-REPORT is
	// carried by an RP-ERROR, so has a TP-FCS.
	AsRPError = rpMessageOption{tpdu.RPError}

	// WithAllCharsets specifies that all character sets are available for
	// encoding or decoding.
	//
	// This is the default policy for decoding.
	WithAllCharsets = AllCharsetsOption{}

	// WithDefaultCharset specifies that only the default character set is
	// available for encoding or decoding.
	//
	// This is the default policy for encoding.
	WithDefaultCharset = CharsetOption{}
)

// WithMR specifies the counter that provides the TP-MR of each SMS-SUBMIT
// and SMS-COMMAND TPDU. A TPDU of another type keeps the TP-MR of the
// template, as described for NewEncoder. A nil counter selects the counter
// shared by Encoders.
//
// The counter must be safe for concurrent use if the Encoder is used
// concurrently, as Counter is.
func WithMR(c tpdu.Counter) EncoderOption {
	return mrOption{c}
}

type mrOption struct {
	c tpdu.Counter
}

func (o mrOption) ApplyEncoderOption(e *Encoder) {
	e.MsgCount = o.c
}

// WithConcatRef specifies the counter that provides the reference of each
// concatenated message. A nil counter selects the counter shared by
// Encoders.
//
// The counter must be safe for concurrent use if the Encoder is used
// concurrently, as Counter is.
func WithConcatRef(c tpdu.Counter) EncoderOption {
	return concatRefOption{c}
}

type concatRefOption struct {
	c tpdu.Counter
}

func (o concatRefOption) ApplyEncoderOption(e *Encoder) {
	e.ConcatRef = o.c
}

// With16BitConcatRef specifies that concatenated messages carry the
// Concatenated short messages, 16-bit reference number IE, as defined in 3GPP
// TS 23.040 Section 9.2.3.24.8, rather than the 8-bit one of Section
// 9.2.3.24.1.
//
// The references are those of the ConcatRef counter modulo 65536, rather
// than modulo 256, so there are more of them, but the IE takes an octet more
// of each segment.
var With16BitConcatRef EncoderOption = segmentationOption{tpdu.With16BitConcatRef}

type segmentationOption struct {
	o tpdu.SegmentationOption
}

func (o segmentationOption) ApplyEncoderOption(e *Encoder) {
	e.sopts = append(e.sopts, o.o)
}

// To specifies the DA for a SMS-SUBMIT TPDU, or an SMS-COMMAND.
//
// The type of address is that tpdu.FromNumber gives it, the default of 3GPP
// TS 27.005 Section 3.1: a number that starts with '+' is an international
// number, and has a TOA of 0x91, and any other number, such as a short code
// or a national number, is of unknown type, and has a TOA of 0x81. For
// another type of address, use WithTemplateOption(tpdu.WithDA(addr)).
func To(number string) EncoderOption {
	addr := tpdu.NewAddress(tpdu.FromNumber(number))
	return templateOption{tpdu.WithDA(addr)}
}

// From specifies the OA for a SMS-DELIVER TPDU.
//
// The type of address is given as for To. For another type of address, such
// as an alphanumeric one, use WithTemplateOption(tpdu.WithOA(addr)).
func From(number string) EncoderOption {
	addr := tpdu.NewAddress(tpdu.FromNumber(number))
	return templateOption{tpdu.WithOA(addr)}
}

// AllCharsetsOption specifies that all character sets are available for
// encoding or decoding.
type AllCharsetsOption struct{}

// ApplyEncoderOption applies the AllCharsetsOption to an Encoder.
func (o AllCharsetsOption) ApplyEncoderOption(e *Encoder) {
	e.eopts = append(e.eopts, tpdu.WithAllCharsets)
}

// ApplyDecodeOption applies the AllCharsetsOption to decoding.
func (o AllCharsetsOption) ApplyDecodeOption(cc *DecodeConfig) {
	cc.dopts = append(cc.dopts, tpdu.WithAllCharsets)
}

// WithCharset creates an CharsetOption.
//
// The identifiers are copied, so the caller may go on to change its slice.
func WithCharset(nli ...int) CharsetOption {
	return CharsetOption{slices.Clone(nli)}
}

// CharsetOption defines the character sets available for encoding or decoding.
type CharsetOption struct {
	nli []int
}

// ApplyEncoderOption applies the CharsetOption to an Encoder.
func (o CharsetOption) ApplyEncoderOption(e *Encoder) {
	e.eopts = append(e.eopts, tpdu.WithCharset(o.nli...))
}

// ApplyDecodeOption applies the CharsetOption to decoding.
func (o CharsetOption) ApplyDecodeOption(cc *DecodeConfig) {
	cc.dopts = append(cc.dopts, tpdu.WithCharset(o.nli...))
}

// WithLockingCharset creates an LockingCharsetOption.
//
// The identifiers are copied, so the caller may go on to change its slice.
func WithLockingCharset(nli ...int) LockingCharsetOption {
	return LockingCharsetOption{slices.Clone(nli)}
}

// LockingCharsetOption defines the locking character sets available for
// encoding or decoding.
type LockingCharsetOption struct {
	nli []int
}

// ApplyEncoderOption applies the LockingCharsetOption to an Encoder.
func (o LockingCharsetOption) ApplyEncoderOption(e *Encoder) {
	e.eopts = append(e.eopts, tpdu.WithLockingCharset(o.nli...))
}

// ApplyDecodeOption applies the LockingCharsetOption to decoding.
func (o LockingCharsetOption) ApplyDecodeOption(cc *DecodeConfig) {
	cc.dopts = append(cc.dopts, tpdu.WithLockingCharset(o.nli...))
}

// WithShiftCharset creates an ShiftCharsetOption.
//
// The identifiers are copied, so the caller may go on to change its slice.
func WithShiftCharset(nli ...int) ShiftCharsetOption {
	return ShiftCharsetOption{slices.Clone(nli)}
}

// ShiftCharsetOption defines the shift character sets available for encoding
// or decoding.
type ShiftCharsetOption struct {
	nli []int
}

// ApplyEncoderOption applies the ShiftCharsetOption to an Encoder.
func (o ShiftCharsetOption) ApplyEncoderOption(e *Encoder) {
	e.eopts = append(e.eopts, tpdu.WithShiftCharset(o.nli...))
}

// ApplyDecodeOption applies the ShiftCharsetOption to decoding.
func (o ShiftCharsetOption) ApplyDecodeOption(cc *DecodeConfig) {
	cc.dopts = append(cc.dopts, tpdu.WithShiftCharset(o.nli...))
}

type directionOption struct {
	d tpdu.Direction
}

func (o directionOption) ApplyUnmarshalOption(d *UnmarshalConfig) {
	d.dirn = o.d
}

type rpMessageOption struct {
	m tpdu.RPMessage
}

func (o rpMessageOption) ApplyUnmarshalOption(d *UnmarshalConfig) {
	d.rp = o.m
}
