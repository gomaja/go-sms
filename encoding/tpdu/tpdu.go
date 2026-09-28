// SPDX-License-Identifier: MIT

// Package tpdu provides the TPDU type and conversions to and from its binary
// form.
package tpdu

import (
	"encoding/binary"

	"github.com/gomaja/go-sms/encoding/gsm7"
)

// TPDU represents all SMS TPDUs.
type TPDU struct {
	// Direction indicates whether the TPDU is mobile originated (MO) or
	// terminated (MT).
	//
	// It is not carried in the TPDU, so it must be set before calling
	// UnmarshalBinary, which keeps it.
	Direction Direction

	// RPMessage indicates whether an SMS-DELIVER-REPORT or SMS-SUBMIT-REPORT
	// is carried by an RP-ACK or an RP-ERROR, as only the latter has a TP-FCS.
	//
	// It is not carried in the TPDU, so, like the Direction, it must be set
	// before calling UnmarshalBinary, which keeps it. It is used by
	// UnmarshalBinary, MarshalBinary and UDBlockSize.
	//
	// Only applies to SMS-DELIVER-REPORT and SMS-SUBMIT-REPORT
	RPMessage RPMessage

	// FirstOctet is the first octet of all TPDUs.
	FirstOctet FirstOctet

	// OA contains the TP-OA Originating Address field.
	//
	// Only applies to SMS-DELIVER
	OA Address

	// FCS contains the TP-FCS Failure Cause field.
	//
	// Only applies to an SMS-DELIVER-REPORT or SMS-SUBMIT-REPORT with an
	// RPMessage of RPError. MarshalBinary returns an error if it is set for
	// one with an RPMessage of RPAck, which has no TP-FCS.
	FCS byte

	// MR contains the TP-MP Message Reference field.
	//
	// Only applies to SMS-COMMAND, SMS-SUBMIT and SMS-STATUS-REPORT
	MR byte

	// CT contains the TP-CT Command Type field.
	//
	// Only applies to SMS-COMMAND
	CT byte

	// MN contains the TP-MN Message Number field.
	//
	// Only applies to SMS-COMMAND
	MN byte

	// DA contains the TP-DA Destination Address field.
	//
	// Only applies to SMS-COMMAND and SMS-SUBMIT
	DA Address

	// RA contains the TP-RA Recipient Address field.
	//
	// Only applies to SMS-STATUS-REPORT
	RA Address

	// PI contains the first octet of the TP-PI Parameter Indicator field.
	//
	// Its extension bit, PiExt, is set on marshal exactly when PIExt is not
	// empty. Its reserved bits are marshalled as held, although the
	// additional information they announce is not: it is discarded on
	// unmarshal, as required by 3GPP TS 23.040 Section 9.2.3.27.
	//
	// Only applies to SMS-DELIVER-REPORT, SMS-SUBMIT-REPORT and
	// SMS-STATUS-REPORT
	PI PI

	// PIExt contains the TP-PI octets that follow the first, as announced by
	// the extension bit of the octet before each.
	//
	// None of their bits is defined by 3GPP TS 23.040 Section 9.2.3.27, other
	// than the extension bit, which is set on marshal on all but the last.
	//
	// Only applies to SMS-DELIVER-REPORT, SMS-SUBMIT-REPORT and
	// SMS-STATUS-REPORT
	PIExt []byte

	// SCTS contains the TP-SCTS Service Center Time Stamp field.
	//
	// The SCTS timestamp indicates the time the SMS was sent.
	// The time is the originator's local time, the timezone of which may
	// differ from the receiver's.
	//
	// Only applies to SMS-DELIVER, SMS-SUBMIT-REPORT and SMS-STATUS-REPORT
	SCTS Timestamp

	// DT contains the TP-DT Discharge Time field.
	//
	// Only applies to SMS-STATUS-REPORT
	DT Timestamp

	// ST contains the TP-ST Status field.
	//
	// Only applies to SMS-STATUS-REPORT
	ST byte

	// PID contains the TP-PID field.
	PID byte

	// DCS contains the TP-DCS Data Coding Scheme field.
	DCS DCS

	// VP contains the TP-VP Validity Period field.
	//
	//  Only applies to SMS-SUBMIT
	VP ValidityPeriod

	// UDH contains the TP-UDH User Data Header field.
	UDH UserDataHeader

	// UD contains the short message from the User Data.
	//
	// It does not include the User Data Header, which is provided separately
	// in the UDH.
	// The interpretation of UD depends on the Alphabet:
	// For Alpha7Bit, UD is an array of GSM7 septets, each septet stored in the
	// lower 7 bits of a byte.
	//  These have NOT been converted to the corresponding UTF8.
	//  Use the gsm7 package to convert to UTF8.
	// For AlphaUCS2, UD is an array of UCS2 characters packed into a byte
	// array in Big Endian.
	//  These have NOT been converted to the corresponding UTF8.
	//  Use the usc2 package to convert to UTF8.
	// For Alpha8Bit, UD contains the raw octets.
	UD UserData
}

// New creates a new TPDU
func New(options ...Option) (*TPDU, error) {
	t := TPDU{OA: NewAddress(), DA: NewAddress(), RA: NewAddress()}
	for _, option := range options {
		err := option.ApplyTPDUOption(&t)
		if err != nil {
			return nil, err
		}
	}
	return &t, nil
}

// NewDeliver creates a new TPDU of type SmsDeliver.
func NewDeliver(options ...Option) (*TPDU, error) {
	options = append([]Option{SmsDeliver}, options...)
	return New(options...)
}

// NewSubmit creates a new TPDU of type SmsSubmit.
func NewSubmit(options ...Option) (*TPDU, error) {
	options = append([]Option{SmsSubmit}, options...)
	return New(options...)
}

// Alphabet returns the alphabet field from the DCS of the SMS TPDU.
func (t *TPDU) Alphabet() Alphabet {
	return t.DCS.Alphabet()
}

// ConcatInfo extracts the segmentation info contained in the provided User
// Data Header.
func (t *TPDU) ConcatInfo() (ConcatInfo, bool) {
	return t.UDH.ConcatInfo()
}

// IsSingleSegment returns true unless the TPDU is part of a multi-part
// message.
func (t *TPDU) IsSingleSegment() bool {
	_, ok := t.ConcatInfo()
	return !ok
}

// MTI returns the MessageType from the first octet of the SMS TPDU.
func (t *TPDU) MTI() MessageType {
	return t.FirstOctet.MTI()
}

// Counter provides a reference couunter that is incremented every time Count
// is called.
type Counter interface {
	Count() int
}

type segmentationConfig struct {
	// concat IE factory
	ief func(msgCount int, segCount int, segment int) InformationElement

	// concat ref generator
	cr Counter

	// MR generator
	mr Counter
}

// SegmentationOption provides an option to modify the behaviour of segmentation.
type SegmentationOption func(*segmentationConfig)

// Segment returns the set of SMS TPDUs required to transmit the message.
//
// The TPDU acts as the template for the generated TPDUs and provides all the
// fields in the resulting TPDUs, other than the UD, which is populated using
// the message.  For multi-part messages, the UDH provided in the TPDU is
// extended with a concatenation IE. The TPDU UDH must not contain a
// concatenation IE (ID 0 or 8) or the resulting TPDUs will be non-conformant.
func (t TPDU) Segment(msg []byte, options ...SegmentationOption) []TPDU {
	if len(msg) == 0 {
		return nil
	}
	cfg := segmentationConfig{newInfoElement, nil, nil}
	for _, o := range options {
		o(&cfg)
	}
	bs := t.UDBlockSize()
	if len(msg) <= bs {
		// single segment
		t.UD = msg
		if cfg.mr != nil {
			t.MR = byte(cfg.mr.Count())
		}
		return []TPDU{t}
	}
	// add contcat IE and recalc bs
	t.SetUDH(append(t.UDH, cfg.ief(0, 0, 0)))
	bs = t.UDBlockSize()
	t.UDH = t.UDH[:len(t.UDH)-1]
	alpha := t.Alphabet()
	chunks := chunk(msg, alpha, bs)
	count := len(chunks)
	pdus := make([]TPDU, count)
	concatRef := 1
	if cfg.cr != nil {
		concatRef = cfg.cr.Count()
	}
	for i := 0; i < count; i++ {
		pdus[i] = t
		if cfg.mr != nil {
			pdus[i].MR = byte(cfg.mr.Count())
		}
		udh := append(t.UDH[:0:0], t.UDH...)
		udh = append(udh, cfg.ief(concatRef, count, i+1))
		pdus[i].SetUDH(udh)
		pdus[i].UD = chunks[i]
	}
	return pdus
}

// With16BitConcatRef specifies the usage of concat IEs with 16 bit reference
// numbers (ID=8).
//
// By default 8bit reference numbers are used.
var With16BitConcatRef = func(so *segmentationConfig) {
	so.ief = newInfoElement16bit
}

// WithMR provides an MR generator to provide the TP-MR field for TPDUs.
//
// By default the MR is copied from the template TPDU.
func WithMR(mr Counter) SegmentationOption {
	return func(so *segmentationConfig) {
		so.mr = mr
	}
}

// WithConcatRef provides a generator to provide the reference for concatenation IEs.
//
// By default the field is set to 1, which is only suitable for one-off messages.
func WithConcatRef(cr Counter) SegmentationOption {
	return func(so *segmentationConfig) {
		so.cr = cr
	}
}

// SetDCS sets the dcs field and the corresponding bit of the PI.
func (t *TPDU) SetDCS(dcs byte) {
	t.PI |= PiDCS
	t.DCS = DCS(dcs)
}

// SetPID sets the TPDU pid field and the corresponding bit of the PI.
func (t *TPDU) SetPID(pid byte) {
	t.PI |= PiPID
	t.PID = pid
}

// SetVP sets the validity period and the corresponding VPF bits
// in the firstOctet.
func (t *TPDU) SetVP(vp ValidityPeriod) {
	t.FirstOctet &^= FoVPFMask
	t.FirstOctet |= (FirstOctet(vp.Format<<FoVPFShift) & FoVPFMask)
	t.VP = vp
}

// SetUD sets the TPDU ud field and the corresponding bit of the PI.
func (t *TPDU) SetUD(ud UserData) {
	t.UD = ud
	if ud == nil {
		t.PI &^= PiUDL
	} else {
		t.PI |= PiUDL
	}
}

// SetUDH sets the User Data Header of the TPDU and the TP-UDHI flag.
func (t *TPDU) SetUDH(udh UserDataHeader) {
	t.UDH = udh
	if udh == nil {
		t.FirstOctet &^= FoUDHI
	} else {
		t.PI |= PiUDL
		t.FirstOctet |= FoUDHI
	}
}

// SmsType returns the type of SMS-TPDU this TPDU represents.
func (t *TPDU) SmsType() SmsType {
	return smsType(t.FirstOctet.MTI(), t.Direction)
}

// SetSmsType returns the type of SMS-TPDU this TPDU represents.
func (t *TPDU) SetSmsType(st SmsType) error {
	if st < 0 || st > SmsCommand {
		return ErrInvalid
	}
	t.Direction = st.Direction()
	t.FirstOctet = t.FirstOctet.WithMTI(st.MTI())
	return nil
}

// UDBlockSize returns the maximum size of a block of UserData that can fit in
// this TPDU.
//
// The interpretation of the size depends on the encoding - for 7bit encoding
// it is the number of septets. For all other encodings it is the number of
// octets.
func (t *TPDU) UDBlockSize() int {
	var bs int
	switch t.SmsType() {
	case SmsSubmit, SmsDeliver:
		bs = 140
	case SmsCommand:
		bs = 146 // conservative
		// precise answer depends on variable length fields...
	case SmsSubmitReport:
		if t.RPMessage == RPError {
			bs = 151
		} else {
			bs = 152
		}
	case SmsDeliverReport:
		if t.RPMessage == RPError {
			bs = 158
		} else {
			bs = 159
		}
	case SmsStatusReport:
		bs = 131 // conservative
		// precise answer depends on variable length fields...
	}
	alpha := t.Alphabet()
	udhl := t.UDHL()
	if alpha == Alpha7Bit {
		// work in septets
		bs = (bs * 8) / 7
		if udhl == 0 {
			return bs
		}
		// remove septets used by UDH, including UDHL and fill bits
		bs -= ((udhl+1)*8 + 6) / 7
		return bs
	}
	if udhl > 0 {
		bs -= (udhl + 1)
	}
	if alpha == AlphaUCS2 {
		bs &^= 0x1
	}
	return bs
}

// UDHI returns the User Data Header Indicator bit from the SMS TPDU first
// octet, as held.
//
// For an unmarshalled TPDU, or one whose UDH was set with SetUDH, it is set
// exactly when the UDH is not nil. MarshalBinary derives the bit from the UDH
// rather than using it.
func (t *TPDU) UDHI() bool {
	return t.FirstOctet.UDHI()
}

// UDHL returns the encoded length of the UDH, not including the UDHL itself.
func (t *TPDU) UDHL() int {
	return t.UDH.UDHL()
}

// MarshalBinary marshals a SMS TPDU into the corresponding byte array.
//
// The type of TPDU is determined by the TP-MTI of the FirstOctet and the
// Direction, and, for the reports, the RPMessage.
//
// The fields are authoritative, so the flag bits that say whether a field is
// present, or how it is coded, are derived from the field rather than taken
// from the FirstOctet or PI, and the TPDU marshalled is always consistent:
//
//   - TP-UDHI is set if the UDH is not nil, as the UDH is then written, and
//     cleared if the UDH is nil and the UD is not empty, as the UD is then
//     written without a header. An empty, but not nil, UDH is written as a
//     TP-UDHL of 0.
//   - TP-VPF of an SMS-SUBMIT is the Format of the VP.
//   - In the TP-PI of a report, PiPID is set if the PID is not 0, PiDCS if
//     the DCS is not 0, so the UD is never encoded with a DCS the receiver
//     would not see, and PiUDL if there is a UDH or UD. The extension bits
//     are set from the PIExt.
//
// A bit that describes a field that is not written, as TP-UDHI does when
// there is no UD, or a PI bit whose field holds 0, is marshalled as held, as
// are the bits that describe no field, such as the reserved PI bits. So a
// TPDU that was unmarshalled marshals back to the octets it came from, except
// where 3GPP TS 23.040 requires a receiver to ignore or discard part of them.
//
// MarshalBinary does not change the TPDU.
func (t *TPDU) MarshalBinary() (dst []byte, err error) {
	st := smsType(t.FirstOctet.MTI(), t.Direction)
	w := t.withDerivedFlags(st)
	switch st {
	case SmsDeliver:
		dst, err = w.marshalDeliver()
	case SmsDeliverReport:
		dst, err = w.marshalDeliverReport()
	case SmsSubmitReport:
		dst, err = w.marshalSubmitReport()
	case SmsSubmit:
		dst, err = w.marshalSubmit()
	case SmsStatusReport:
		dst, err = w.marshalStatusReport()
	case SmsCommand:
		dst, err = w.marshalCommand()
	default:
		return nil, ErrUnsupportedSmsType(st)
	}
	if err != nil {
		err = NewEncodeError(st.String(), err)
	}
	return
}

// withDerivedFlags returns a copy of the TPDU with the TP-UDHI and TP-PI bits
// derived from the fields they describe, as described for MarshalBinary.
//
// The TP-VPF is derived by marshalSubmit, once the VP has been validated.
func (t *TPDU) withDerivedFlags(st SmsType) *TPDU {
	w := *t
	if st == SmsCommand {
		return &w
	}
	// 3GPP TS 23.040 Section 9.2.3.23: TP-UDHI "1 The beginning of the TP-UD
	// field contains a Header in addition to the short message."
	switch {
	case w.UDH != nil:
		w.FirstOctet |= FoUDHI
	case len(w.UD) > 0:
		w.FirstOctet &^= FoUDHI
	}
	switch st {
	case SmsDeliverReport, SmsSubmitReport, SmsStatusReport:
		// 3GPP TS 23.040 Section 9.2.3.27: "If the TP-UDL bit is set to "1"
		// but the TP-DCS bit is set to "0" then the receiving entity shall
		// for TP-DCS assume a value of 0x00".
		if w.PID != 0 {
			w.PI |= PiPID
		}
		if w.DCS != 0 {
			w.PI |= PiDCS
		}
		if w.UDH != nil || len(w.UD) > 0 {
			w.PI |= PiUDL
		}
	}
	return &w
}

func (t *TPDU) marshalCommand() ([]byte, error) {
	da, err := t.DA.MarshalBinary()
	if err != nil {
		return nil, NewEncodeError("da", err)
	}
	cdl := len(t.UD)
	l := 6 + len(da) + cdl
	b := make([]byte, 0, l)
	b = append(b, byte(t.FirstOctet), t.MR, t.PID, t.CT, t.MN)
	b = append(b, da...)
	b = append(b, byte(cdl))
	b = append(b, t.UD...)
	return b, nil
}

func (t *TPDU) marshalDeliver() ([]byte, error) {
	oa, err := t.OA.MarshalBinary()
	if err != nil {
		return nil, NewEncodeError("oa", err)
	}
	scts, err := t.SCTS.MarshalBinary()
	if err != nil {
		return nil, NewEncodeError("scts", err)
	}
	ud, err := t.encodeUserData()
	if err != nil {
		return nil, NewEncodeError("ud", err)
	}
	l := 3 + len(oa) + len(scts) + len(ud)
	b := make([]byte, 0, l)
	b = append(b, byte(t.FirstOctet))
	b = append(b, oa...)
	b = append(b, t.PID, byte(t.DCS))
	b = append(b, scts...)
	b = append(b, ud...)
	return b, nil
}

func (t *TPDU) marshalDeliverReport() ([]byte, error) {
	fcs, err := t.rpFCS()
	if err != nil {
		return nil, err
	}
	pi := t.piOctets()
	opt, err := t.marshalOptionals(PI(pi[0]))
	if err != nil {
		return nil, err
	}
	b := make([]byte, 0, 1+len(fcs)+len(pi)+len(opt))
	b = append(b, byte(t.FirstOctet))
	b = append(b, fcs...)
	b = append(b, pi...)
	b = append(b, opt...)
	return b, nil
}

// piOctets returns the octets of the TP-PI, which are the PI followed by the
// PIExt.
//
// 3GPP TS 23.040 Section 9.2.3.27: "The most significant bit in octet 1 and
// any other TP-PI octets which may be added later is reserved as an extension
// bit which when set to a 1 shall indicate that another TP-PI octet follows
// immediately afterwards." So the extension bit of each octet is set exactly
// when another octet follows, whatever the PI and PIExt hold.
func (t *TPDU) piOctets() []byte {
	b := make([]byte, 0, 1+len(t.PIExt))
	b = append(b, byte(t.PI))
	b = append(b, t.PIExt...)
	for i := range b {
		b[i] &^= PiExt
		if i < len(b)-1 {
			b[i] |= PiExt
		}
	}
	return b
}

// marshalOptionals returns the TP-PID, TP-DCS, TP-UDL and TP-UD fields of a
// report, those that follow the TP-PI and its TP-SCTS, that are announced by
// the pi.
func (t *TPDU) marshalOptionals(pi PI) ([]byte, error) {
	var b []byte
	if pi.PID() {
		b = append(b, t.PID)
	}
	if pi.DCS() {
		b = append(b, byte(t.DCS))
	}
	if pi.UDL() {
		ud, err := t.encodeUserData()
		if err != nil {
			return nil, NewEncodeError("ud", err)
		}
		b = append(b, ud...)
	}
	return b, nil
}

func (t *TPDU) marshalStatusReport() ([]byte, error) {
	ra, err := t.RA.MarshalBinary()
	if err != nil {
		return nil, NewEncodeError("ra", err)
	}
	scts, err := t.SCTS.MarshalBinary()
	if err != nil {
		return nil, NewEncodeError("scts", err)
	}
	dt, err := t.DT.MarshalBinary()
	if err != nil {
		return nil, NewEncodeError("dt", err)
	}
	pi := t.piOctets()
	opt, err := t.marshalOptionals(PI(pi[0]))
	if err != nil {
		return nil, err
	}
	b := make([]byte, 0, 3+len(ra)+len(scts)+len(dt)+len(pi)+len(opt))
	b = append(b, byte(t.FirstOctet), t.MR)
	b = append(b, ra...)
	b = append(b, scts...)
	b = append(b, dt...)
	b = append(b, t.ST)
	// 3GPP TS 23.040 Section 9.2.2.3: the TP-PI is "Mandatory if any of the
	// optional parameters following TP-PI is present, otherwise optional."
	if len(pi) == 1 && pi[0] == 0 {
		return b, nil
	}
	b = append(b, pi...)
	b = append(b, opt...)
	return b, nil
}

func (t *TPDU) marshalSubmit() ([]byte, error) {
	da, err := t.DA.MarshalBinary()
	if err != nil {
		return nil, NewEncodeError("da", err)
	}
	ud, err := t.encodeUserData()
	if err != nil {
		return nil, NewEncodeError("ud", err)
	}
	// VP.MarshalBinary rejects any Format beyond VpfAbsolute, so the Format
	// fits the TP-VPF, which 3GPP TS 23.040 Section 9.2.3.3 defines as
	// saying whether, and in which format, the TP-VP is present.
	vp, err := t.VP.MarshalBinary()
	if err != nil {
		return nil, NewEncodeError("vp", err)
	}
	fo := t.FirstOctet.WithVPF(t.VP.Format)
	l := 4 + len(da) + len(ud) + len(vp)
	b := make([]byte, 0, l)
	b = append(b, byte(fo), t.MR)
	b = append(b, da...)
	b = append(b, t.PID, byte(t.DCS))
	b = append(b, vp...)
	b = append(b, ud...)
	return b, nil
}

func (t *TPDU) marshalSubmitReport() ([]byte, error) {
	scts, err := t.SCTS.MarshalBinary()
	if err != nil {
		return nil, NewEncodeError("scts", err)
	}
	fcs, err := t.rpFCS()
	if err != nil {
		return nil, err
	}
	pi := t.piOctets()
	opt, err := t.marshalOptionals(PI(pi[0]))
	if err != nil {
		return nil, err
	}
	b := make([]byte, 0, 1+len(fcs)+len(pi)+len(scts)+len(opt))
	b = append(b, byte(t.FirstOctet))
	b = append(b, fcs...)
	b = append(b, pi...)
	b = append(b, scts...)
	b = append(b, opt...)
	return b, nil
}

// UnmarshalBinary unmarshals a SMS TPDU from the corresponding byte array.
//
// The Direction, and for the reports the RPMessage, must be set before
// calling UnmarshalBinary, as the octets alone do not identify the type of
// the TPDU, nor whether a report has a TP-FCS. Every other field is reset
// before decoding, so no field of a TPDU previously held by t survives.
//
// In the case of error the TPDU will be partially unmarshalled, up to the
// point that the decoding error was detected.
func (t *TPDU) UnmarshalBinary(src []byte) (err error) {
	*t = TPDU{Direction: t.Direction, RPMessage: t.RPMessage}
	if len(src) < 1 {
		return NewDecodeError("tpdu.firstOctet", 0, ErrUnderflow)
	}
	t.FirstOctet = FirstOctet(src[0])
	st := smsType(t.FirstOctet.MTI(), t.Direction)
	switch st {
	case SmsDeliver:
		err = t.unmarshalDeliver(src[1:])
	case SmsDeliverReport:
		err = t.unmarshalDeliverReport(src[1:])
	case SmsSubmitReport:
		err = t.unmarshalSubmitReport(src[1:])
	case SmsSubmit:
		err = t.unmarshalSubmit(src[1:])
	case SmsStatusReport:
		err = t.unmarshalStatusReport(src[1:])
	case SmsCommand:
		err = t.unmarshalCommand(src[1:])
	default:
		return NewDecodeError("tpdu.firstOctet", 0, ErrUnsupportedSmsType(st))
	}
	if err != nil {
		return NewDecodeError(st.String(), 1, err)
	}
	return nil
}

func (t *TPDU) unmarshalCommand(src []byte) error {
	ri := 0
	for _, f := range []struct {
		name string
		p    *byte
	}{{"mr", &t.MR}, {"pid", &t.PID}, {"ct", &t.CT}, {"mn", &t.MN}} {
		if len(src) <= ri {
			return NewDecodeError(f.name, ri, ErrUnderflow)
		}
		*f.p = src[ri]
		ri++
	}
	n, err := t.DA.UnmarshalBinary(src[ri:])
	if err != nil {
		return NewDecodeError("da", ri, err)
	}
	ri += n
	t.DCS = Dcs8BitData // force TPDU to interpret UD as 8bit, if not set already
	n, err = t.decodeUserData(src[ri:])
	if err != nil {
		return NewDecodeError("ud", ri, err)
	}
	return checkTrailing(src, ri+n, false)
}

func (t *TPDU) unmarshalDeliver(src []byte) error {
	n, err := t.OA.UnmarshalBinary(src)
	if err != nil {
		return NewDecodeError("oa", 0, err)
	}
	ri := n
	if len(src) <= ri {
		return NewDecodeError("pid", ri, ErrUnderflow)
	}
	t.PID = src[ri]
	ri++
	if len(src) <= ri {
		return NewDecodeError("dcs", ri, ErrUnderflow)
	}
	t.DCS = DCS(src[ri])
	ri++
	if len(src) < ri+7 {
		return NewDecodeError("scts", ri, ErrUnderflow)
	}
	err = t.SCTS.UnmarshalBinary(src[ri : ri+7])
	if err != nil {
		return NewDecodeError("scts", ri, err)
	}
	ri += 7
	n, err = t.decodeUserData(src[ri:])
	if err != nil {
		return NewDecodeError("ud", ri, err)
	}
	return checkTrailing(src, ri+n, false)
}

// checkTrailing returns an error if src holds octets beyond the end of the
// TPDU at ri, unless they are to be discarded.
func checkTrailing(src []byte, ri int, discard bool) error {
	if ri < len(src) && !discard {
		return NewDecodeError("ud", ri, ErrOverlength)
	}
	return nil
}

// unmarshalPI reads the TP-PI, including any extension octets, from src[ri:],
// and returns the index of the octet that follows it.
//
// 3GPP TS 23.040 Section 9.2.3.27: "The most significant bit in octet 1 and
// any other TP-PI octets which may be added later is reserved as an extension
// bit which when set to a 1 shall indicate that another TP-PI octet follows
// immediately afterwards."
func (t *TPDU) unmarshalPI(src []byte, ri int) (int, error) {
	if len(src) <= ri {
		return ri, NewDecodeError("pi", ri, ErrUnderflow)
	}
	t.PI = PI(src[ri])
	ri++
	for ext := t.PI&PiExt != 0; ext; ri++ {
		if len(src) <= ri {
			return ri, NewDecodeError("pi", ri, ErrUnderflow)
		}
		t.PIExt = append(t.PIExt, src[ri])
		ext = src[ri]&PiExt != 0
	}
	return ri, nil
}

// piReserved reports whether any reserved bit of the TP-PI is set.
//
// Bits 3 to 6 of the first octet are reserved, and, as none of their bits are
// defined, so are all the bits of the extension octets, other than the
// extension bit itself, as defined in 3GPP TS 23.040 Section 9.2.3.27.
func (t *TPDU) piReserved() bool {
	if t.PI&PiReserved != 0 {
		return true
	}
	for _, o := range t.PIExt {
		if o&^PiExt != 0 {
			return true
		}
	}
	return false
}

// unmarshalOptionals reads the TP-PID, TP-DCS, TP-UDL and TP-UD fields of a
// report, those that follow the TP-PI and its TP-SCTS, that are announced by
// the TP-PI, starting at src[ri].
func (t *TPDU) unmarshalOptionals(src []byte, ri int) error {
	if t.PI.PID() {
		if len(src) <= ri {
			return NewDecodeError("pid", ri, ErrUnderflow)
		}
		t.PID = src[ri]
		ri++
	}
	// Otherwise the DCS is left 0x00, as 3GPP TS 23.040 Section 9.2.3.27
	// says: "If the TP-UDL bit is set to "1" but the TP-DCS bit is set to "0"
	// then the receiving entity shall for TP-DCS assume a value of 0x00".
	if t.PI.DCS() {
		if len(src) <= ri {
			return NewDecodeError("dcs", ri, ErrUnderflow)
		}
		t.DCS = DCS(src[ri])
		ri++
	}
	if t.PI.UDL() {
		n, err := t.decodeUserData(src[ri:])
		if err != nil {
			return NewDecodeError("ud", ri, err)
		}
		ri += n
	}
	// 3GPP TS 23.040 Section 9.2.3.27: "If a Reserved bit is set to "1" then
	// the receiving entity shall ignore the setting. The setting of this bit
	// shall mean that additional information will follow the TP-User-Data,
	// so a receiving entity shall discard any octets following the
	// TP-User-Data."
	return checkTrailing(src, ri, t.piReserved())
}

// rpFCS returns the TP-FCS of a report, which is only present in a report
// carried by an RP-ERROR, as defined in 3GPP TS 23.040 Sections 9.2.2.1a and
// 9.2.2.2a.
//
// An FCS in a report carried by an RP-ACK cannot be encoded, so is an error
// rather than being silently dropped.
func (t *TPDU) rpFCS() ([]byte, error) {
	switch t.RPMessage {
	case RPError:
		return []byte{t.FCS}, nil
	case RPAck:
		if t.FCS != 0 {
			return nil, NewEncodeError("fcs", ErrInvalid)
		}
		return nil, nil
	default:
		return nil, NewEncodeError("rp", ErrInvalid)
	}
}

// unmarshalFCS reads the TP-FCS of a report carried by an RP-ERROR from the
// start of src, and returns the number of octets read.
func (t *TPDU) unmarshalFCS(src []byte) (int, error) {
	switch t.RPMessage {
	case RPError:
		if len(src) < 1 {
			return 0, NewDecodeError("fcs", 0, ErrUnderflow)
		}
		t.FCS = src[0]
		return 1, nil
	case RPAck:
		return 0, nil
	default:
		return 0, NewDecodeError("rp", 0, ErrInvalid)
	}
}

func (t *TPDU) unmarshalDeliverReport(src []byte) error {
	ri, err := t.unmarshalFCS(src)
	if err != nil {
		return err
	}
	ri, err = t.unmarshalPI(src, ri)
	if err != nil {
		return err
	}
	return t.unmarshalOptionals(src, ri)
}

func (t *TPDU) unmarshalStatusReport(src []byte) error {
	ri := 0
	if len(src) <= ri {
		return NewDecodeError("mr", ri, ErrUnderflow)
	}
	t.MR = src[ri]
	ri++
	n, err := t.RA.UnmarshalBinary(src[ri:])
	if err != nil {
		return NewDecodeError("ra", ri, err)
	}
	ri += n
	if len(src) < ri+7 {
		return NewDecodeError("scts", ri, ErrUnderflow)
	}
	err = t.SCTS.UnmarshalBinary(src[ri : ri+7])
	if err != nil {
		return NewDecodeError("scts", ri, err)
	}
	ri += 7
	if len(src) < ri+7 {
		return NewDecodeError("dt", ri, ErrUnderflow)
	}
	err = t.DT.UnmarshalBinary(src[ri : ri+7])
	if err != nil {
		return NewDecodeError("dt", ri, err)
	}
	ri += 7
	if len(src) <= ri {
		return NewDecodeError("st", ri, ErrUnderflow)
	}
	t.ST = src[ri]
	ri++
	// 3GPP TS 23.040 Section 9.2.2.3: the TP-PI is "Mandatory if any of the
	// optional parameters following TP-PI is present, otherwise optional."
	if len(src) == ri {
		return nil
	}
	ri, err = t.unmarshalPI(src, ri)
	if err != nil {
		return err
	}
	return t.unmarshalOptionals(src, ri)
}

func (t *TPDU) unmarshalSubmit(src []byte) error {
	if len(src) < 1 {
		return NewDecodeError("mr", 0, ErrUnderflow)
	}
	t.MR = src[0]
	ri := 1
	n, err := t.DA.UnmarshalBinary(src[ri:])
	if err != nil {
		return NewDecodeError("da", ri, err)
	}
	ri += n
	if len(src) <= ri {
		return NewDecodeError("pid", ri, ErrUnderflow)
	}
	t.PID = src[ri]
	ri++
	if len(src) <= ri {
		return NewDecodeError("dcs", ri, ErrUnderflow)
	}
	t.DCS = DCS(src[ri])
	ri++
	n, err = t.VP.UnmarshalBinary(src[ri:], t.FirstOctet.VPF())
	if err != nil {
		return NewDecodeError("vp", ri, err)
	}
	ri += n
	n, err = t.decodeUserData(src[ri:])
	if err != nil {
		return NewDecodeError("ud", ri, err)
	}
	return checkTrailing(src, ri+n, false)
}

func (t *TPDU) unmarshalSubmitReport(src []byte) error {
	ri, err := t.unmarshalFCS(src)
	if err != nil {
		return err
	}
	ri, err = t.unmarshalPI(src, ri)
	if err != nil {
		return err
	}
	if len(src) < ri+7 {
		return NewDecodeError("scts", ri, ErrUnderflow)
	}
	err = t.SCTS.UnmarshalBinary(src[ri : ri+7])
	if err != nil {
		return NewDecodeError("scts", ri, err)
	}
	ri += 7
	return t.unmarshalOptionals(src, ri)
}

// decodeUserData decodes the TP-UDL at the start of src, and the TP-UD it
// announces, into the UDH and UD, and returns the number of octets they
// occupy. Any octets that follow are left to the caller.
func (t *TPDU) decodeUserData(src []byte) (int, error) {
	if len(src) < 1 {
		return 0, NewDecodeError("udl", 0, ErrUnderflow)
	}
	udl := int(src[0])
	if udl == 0 {
		// 3GPP TS 23.040 Section 9.2.3.16: "If this field is zero, the
		// TP-User-Data field shall not be present."
		return 1, nil
	}
	var udh UserDataHeader
	sml7 := 0
	ri := 1
	alphabet := t.Alphabet()
	if alphabet == Alpha7Bit {
		sml7 = udl
		// length is septets - convert to octets
		udl = (sml7*7 + 7) / 8
	}
	if len(src) < ri+udl {
		return 0, NewDecodeError("sm", ri, ErrUnderflow)
	}
	src = src[:ri+udl]
	var udhl int // Note that in this context udhl includes itself.
	udhi := t.UDHI()
	if udhi {
		udh = make(UserDataHeader, 0)
		l, err := udh.UnmarshalBinary(src[ri:])
		if err != nil {
			return 0, NewDecodeError("udh", ri, err)
		}
		udhl = l
		ri += udhl
	}
	if ri == len(src) {
		t.UDH = udh
		return len(src), nil
	}
	switch alphabet {
	case Alpha7Bit:
		sm, err := decode7Bit(sml7, udhl, src[ri:])
		if err != nil {
			return 0, NewDecodeError("sm", ri, err)
		}
		t.UD = sm
	case AlphaUCS2:
		if len(src[ri:])&0x01 == 0x01 {
			return 0, NewDecodeError("sm", ri, ErrOddUCS2Length)
		}
		fallthrough
	case Alpha8Bit:
		t.UD = append([]byte(nil), src[ri:]...)
	}
	t.UDH = udh
	return len(src), nil
}

// decode7Bit decodes the GSM7 encoded binary src into a byte array.
//
// sml is the number of septets expected, and udhl is the number of octets in
// the UDH, including the UDHL field.
func decode7Bit(sml, udhl int, src []byte) ([]byte, error) {
	var fillBits int
	if udhl > 0 {
		if dangling := udhl % 7; dangling != 0 {
			fillBits = 7 - dangling
		}
		sml = sml - (udhl*8+fillBits)/7
	}
	if sml < 0 {
		return nil, ErrUnderflow
	}
	sm := gsm7.Unpack7Bit(src, fillBits)
	// this is a double check on the math and should never trip...
	surplus := len(sm) - sml
	if surplus < 0 {
		return nil, ErrUnderflow
	}
	if surplus > 0 {
		if surplus > 1 || sm[len(sm)-1] != 0 {
			return nil, ErrOverlength
		}
		// drop trailing 0 septet
		sm = sm[:sml]
	}
	return sm, nil
}

// encodeUserData marshals the User Data into binary.
//
// The User Data Header is also encoded if present.
// If Alphabet is GSM7 then the User Data is assumed to be unpacked GSM7
// septets and is packed prior to encoding.
// For other alphabet values the User Data is encoded as is.
// No checks of encoded size are performed here as that depends on concrete
// TPDU type, and that can check the length of the returned b.
func (t *TPDU) encodeUserData() (b []byte, err error) {
	udh, err := t.UDH.MarshalBinary()
	if err != nil {
		return nil, NewEncodeError("udh", err)
	}
	ud := t.UD
	alphabet := t.Alphabet()
	udl := len(t.UD) // assume octets
	switch alphabet {
	case Alpha7Bit:
		fillBits := 0
		if dangling := len(udh) % 7; dangling != 0 {
			fillBits = 7 - dangling
		}
		ud, err = gsm7.Pack7Bit(t.UD, fillBits)
		if err != nil {
			return nil, NewEncodeError("sm", err)
		}
		// udl is in septets so convert
		if udl > 0 {
			udl = udl + (len(udh)*8+fillBits)/7
		} else {
			udl = (len(udh) * 8) / 7
		}
	case AlphaUCS2:
		if udl&0x01 == 0x01 {
			return nil, NewEncodeError("sm", ErrOddUCS2Length)
		}
		fallthrough
	case Alpha8Bit:
		// udl is in octets
		udl = udl + len(udh)
	}
	b = make([]byte, 0, 1+len(udh)+len(ud))
	b = append(b, byte(udl))
	b = append(b, udh...)
	b = append(b, ud...)
	return b, nil
}

// MaxUDL is the maximum number of octets that can be encoded into the UD.
// Note that for 7bit encoding this can result in up to 160 septets.
const MaxUDL = 140

// MessageType identifies the type of TPDU encoded in a binary stream, as
// defined in 3GPP TS 23.040 Section 9.2.3.1.
// Note that the direction of the TPDU must also be known to determine how to
// interpret the TPDU.
type MessageType int

const (
	// MtDeliver identifies the message as a SMS-Deliver or SMS-Deliver-Report
	// TPDU.
	MtDeliver MessageType = iota

	// MtSubmit identifies the message as a SMS-Submit or SMS-Submit-Report
	// TPDU.
	MtSubmit

	// MtCommand identifies the message as a SMS-Command or SMS-Status-Report
	// TPDU.
	MtCommand

	// MtReserved identifies the message as an unknown type of SMS TPDU.
	MtReserved
)

// ApplyTPDUOption sets the TPDU MTI.
//
// An error is returned if the MessageType does not fit the 2 bit TP-MTI.
func (mti MessageType) ApplyTPDUOption(t *TPDU) error {
	if mti < MtDeliver || mti > MtReserved {
		return ErrInvalid
	}
	t.FirstOctet = t.FirstOctet.WithMTI(mti)
	return nil
}

func (mti MessageType) String() string {
	switch mti {
	case MtDeliver:
		return "Deliver"
	case MtSubmit:
		return "Submit"
	case MtCommand:
		return "Command"
	default:
		return "Unknown"
	}
}

// Direction indicates the direction that the SMS TPDU is carried.
type Direction int

const (
	// MT indicates that the SMS TPDU is intended to be received by the MS.
	MT Direction = iota

	// MO indicates that the SMS TPDU is intended to be sent by the MS.
	MO
)

// ApplyTPDUOption sets the direction of the TPDU.
func (d Direction) ApplyTPDUOption(t *TPDU) error {
	t.Direction = d
	return nil
}

// RPMessage identifies the RP message that carries an SMS-DELIVER-REPORT or
// an SMS-SUBMIT-REPORT.
//
// 3GPP TS 23.040 Sections 9.2.2.1a and 9.2.2.2a define a report for RP-ERROR,
// the negative acknowledgement, which has a TP-FCS, and a report for RP-ACK,
// the positive acknowledgement, which does not. Nothing in the TPDU says which
// it is: the RP layer knows.
type RPMessage int

const (
	// RPAck indicates the report is carried by an RP-ACK, so has no TP-FCS.
	RPAck RPMessage = iota

	// RPError indicates the report is carried by an RP-ERROR, so has a
	// TP-FCS.
	RPError
)

// ApplyTPDUOption sets the RPMessage of the TPDU.
func (m RPMessage) ApplyTPDUOption(t *TPDU) error {
	if m != RPAck && m != RPError {
		return ErrInvalid
	}
	t.RPMessage = m
	return nil
}

func (m RPMessage) String() string {
	switch m {
	case RPAck:
		return "RP-ACK"
	case RPError:
		return "RP-ERROR"
	default:
		return "Unknown"
	}
}

// SmsType indicates the type of SMS TPDU type represented by the TPDU.
type SmsType int

const (
	// SmsDeliver indiates the TPDU represents a SMS-DELIVER
	SmsDeliver SmsType = iota

	// SmsDeliverReport indiates the TPDU represents a SMS-DELIVER-REPORT
	SmsDeliverReport

	// SmsSubmitReport indiates the TPDU represents a SMS-SUBMIT-REPORT
	SmsSubmitReport

	// SmsSubmit indiates the TPDU represents a SMS-SUBMIT
	SmsSubmit

	// SmsStatusReport indiates the TPDU represents a SMS-STATUS-REPORT
	SmsStatusReport

	// SmsCommand indiates the TPDU represents a SMS-COMMAND
	SmsCommand
)

// MTI returns the MessageType corresponding to the SmsType.
func (st SmsType) MTI() MessageType {
	return MessageType(st >> 1)
}

// Direction returns the direction corresponding to the SmsType.
func (st SmsType) Direction() Direction {
	return Direction(st & 0x01)
}

func (st SmsType) String() string {
	switch st {
	case SmsDeliver:
		return "SmsDeliver"
	case SmsDeliverReport:
		return "SmsDeliverReport"
	case SmsSubmitReport:
		return "SmsSubmitReport"
	case SmsSubmit:
		return "SmsSubmit"
	case SmsStatusReport:
		return "SmsStatusReport"
	case SmsCommand:
		return "SmsCommand"
	default:
		return "Unknown"
	}
}

// ApplyTPDUOption sets the TPDU direction and MTI to match the SmsType.
func (st SmsType) ApplyTPDUOption(t *TPDU) error {
	return t.SetSmsType(st)
}

// smsType returns the type of TPDU identified by the TP-MTI in the direction.
//
// 3GPP TS 23.040 Section 9.2.3.1: "If an MS receives a TPDU with a "Reserved"
// value in the TP-MTI it shall process the message as if it were an
// "SMS-DELIVER" but store the message exactly as received." So the Reserved
// value in the MT direction is an SMS-DELIVER, while the first octet keeps
// the received TP-MTI, and is marshalled as received.
func smsType(mt MessageType, dir Direction) SmsType {
	if mt == MtReserved && dir == MT {
		return SmsDeliver
	}
	return SmsType(byte(mt<<1) | byte(dir))
}

func newInfoElement(msgCount, segCount, segment int) InformationElement {
	ie := InformationElement{}
	ie.ID = 0
	ie.Data = []byte{byte(msgCount), byte(segCount), byte(segment)}
	return ie
}

func newInfoElement16bit(msgCount, segCount, segment int) InformationElement {
	ie := InformationElement{}
	ie.ID = 8
	ie.Data = []byte{0, 0, byte(segCount), byte(segment)}
	binary.BigEndian.PutUint16(ie.Data, uint16(msgCount))
	return ie
}

const (
	esc byte = 0x1b
)

// chunk splits a message into chunks that are not larger than bs.
func chunk(msg []byte, alpha Alphabet, bs int) [][]byte {
	switch alpha {
	default: // default to 7Bit
		return chunk7Bit(msg, bs)
	case AlphaUCS2:
		return chunkUCS2(msg, bs)
	case Alpha8Bit:
		return chunk8Bit(msg, bs)
	}
}

// chunk7Bit splits a GSM7 message into chunks that are not larger than bs.
//
// Escaped characters are not split across blocks, so the resulting blocks may
// be one septet shorter than bs.
func chunk7Bit(msg []byte, bs int) [][]byte {
	if len(msg) == 0 {
		return nil
	}
	count := 1 + len(msg)/bs
	chunks := make([][]byte, 0, count)
	bstart := 0
	bend := bs
	for bend < len(msg) {
		// don't split escapes
		if msg[bend-1] == esc && msg[bend-2] != esc {
			bend--
		}
		chunks = append(chunks, msg[bstart:bend])
		bstart = bend
		bend = bstart + bs
	}
	chunks = append(chunks, msg[bstart:])
	return chunks
}

// chunk8Bit splits a raw 8bit message into chunks that are bs, except for the
// last segment which contains any residual bytes.
func chunk8Bit(msg []byte, bs int) [][]byte {
	if len(msg) == 0 {
		return nil
	}
	count := 1 + len(msg)/bs
	chunks := make([][]byte, 0, count)
	bstart := 0
	bend := bs
	for bend < len(msg) {
		chunks = append(chunks, msg[bstart:bend])
		bstart = bend
		bend = bstart + bs
	}
	chunks = append(chunks, msg[bstart:])
	return chunks
}

const (
	surrHighStart = 0xd800
	surrLowStart  = 0xdc00
)

// chunkUCS2 splits a UCS2/UTF-16 message into chunks that are not larger than bs.
//
// bs should be even, but if odd is reduced by one.
// To allow for reassemblers that cannot handle split surrogate pairs, they are
// not split during chunking, so the resulting blocks may be slightly smaller
// than bs whenever a surrogate pair would span a block boundary.
// While the msg should have even length for UCS2, the chunker does not enforce
// this, and if an odd length message is presented then the final chunk will
// have an odd length.
func chunkUCS2(msg []byte, bs int) [][]byte {
	if len(msg) == 0 {
		return nil
	}
	bs &^= 0x1
	// rough count of blocks - may be off due to not splitting surrogates, but
	// not worth working out the precise count in advance.
	count := 1 + len(msg)/bs
	chunks := make([][]byte, 0, count)
	bstart := 0
	bend := bstart + bs
	for bend < len(msg) {
		// check last uint16 is a high surrogate, if so then leave for later
		r := binary.BigEndian.Uint16(msg[bend-2 : bend])
		if surrHighStart <= r && r < surrLowStart {
			bend = bend - 2
		}
		chunks = append(chunks, msg[bstart:bend])
		bstart = bend
		bend = bstart + bs
	}
	chunks = append(chunks, msg[bstart:])
	return chunks
}
