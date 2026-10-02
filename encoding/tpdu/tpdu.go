// SPDX-License-Identifier: MIT

// Package tpdu provides the TPDU type and conversions to and from its binary
// form.
package tpdu

import (
	"bytes"
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
	//
	// Any other value is of no TPDU type, so SmsType returns a type that is
	// not supported, and UnmarshalBinary, MarshalBinary and Segment return an
	// ErrUnsupportedSmsType.
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

	// FCS contains the TP-FCS Failure Cause field, as received.
	//
	// A receiver takes it as FailureCause returns it, which is 0xFF,
	// "Unspecified error cause", for a report with an unused bit set in its
	// first octet.
	//
	// Only applies to an SMS-DELIVER-REPORT or SMS-SUBMIT-REPORT with an
	// RPMessage of RPError. MarshalBinary returns an error if it is set for
	// one with an RPMessage of RPAck, which has no TP-FCS.
	FCS byte

	// MR contains the TP-MR Message Reference field.
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
	// The TP-PI of an SMS-STATUS-REPORT is optional unless a field it
	// announces is present (Section 9.2.2.3), so one whose PI and PIExt
	// are empty is marshalled without a TP-PI, unless it was unmarshalled
	// with a TP-PI of 0.
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

	// UD contains the short message from the User Data, or the TP-CD of an
	// SMS-COMMAND.
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
	//  Use the ucs2 package to convert to runes.
	// For Alpha8Bit, UD contains the raw octets.
	// For compressed data, as indicated by the DCS, UD contains the compressed
	// octets, as defined in 3GPP TS 23.042, whatever the Alphabet.
	UD UserData

	// ignoredUDH holds the octets of a UDH, including its UDHL, that was
	// ignored when unmarshalling, leaving UDH empty. "Irrespective of whether
	// any part of the User Data Header is ignored or discarded, the MS shall
	// always store the entire TPDU exactly as received" (3GPP TS 23.040
	// Section 9.2.3.24), so these octets are marshalled while UDH is left
	// empty, and dropped by SetUDH.
	ignoredUDH []byte

	// unexamined holds the octets that follow the TP-FCS of an
	// SMS-DELIVER-REPORT or SMS-SUBMIT-REPORT for RP-ERROR that was
	// unmarshalled with an unused bit set in its first octet, whose fields
	// a receiver "shall not examine" (3GPP TS 23.040 Sections 9.2.2.1a and
	// 9.2.2.2a). They are marshalled in place of the fields while those are
	// left empty, as described for keepsUnexamined.
	unexamined []byte

	// zeroPI is set for an SMS-STATUS-REPORT that was unmarshalled with a
	// TP-PI of 0, which is then marshalled although it is optional, so the
	// TPDU marshals back to the octets it came from.
	zeroPI bool
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

// Alphabet returns the alphabet of the UD, as given by the DCS of the SMS
// TPDU.
//
// An SMS-COMMAND has no TP-DCS, and its TP-CD is octets, as 3GPP TS 23.040
// Section 9.2.3.20 says: "The TP-Command-Data-Length field is used to
// indicate the number of octets contained within the TP-Command-Data field".
// So for an SMS-COMMAND Alphabet returns Alpha8Bit, whatever the DCS.
func (t *TPDU) Alphabet() Alphabet {
	if t.SmsType() == SmsCommand {
		return Alpha8Bit
	}
	return t.DCS.Alphabet()
}

// udCoding returns the coding of the TP-UD, or the TP-CD of an SMS-COMMAND,
// which determines how its length is counted and whether it is packed:
// Alpha7Bit for septets, and Alpha8Bit or AlphaUCS2 for octets.
//
// Compressed data is octets, whatever the alphabet: 3GPP TS 23.040 Section
// 9.2.3.16 says "If the TP-User-Data is coded using compressed GSM 7 bit
// default alphabet or compressed 8 bit data or compressed UCS2 [24] data, the
// TP-User-Data-Length field gives an integer representation of the number of
// octets after compression within the TP-User-Data field to follow."
func (t *TPDU) udCoding() Alphabet {
	if t.DCS.Compressed() {
		return Alpha8Bit
	}
	return t.Alphabet()
}

// ConcatInfo extracts the segmentation info contained in the provided User
// Data Header.
func (t *TPDU) ConcatInfo() (ConcatInfo, bool) {
	return t.UDH.ConcatInfo()
}

// IsSingleSegment returns true unless the TPDU is part of a multi-part
// message.
//
// A TPDU without a valid concatenation IE, or with one whose total is 1, is
// the only segment of its message.
func (t *TPDU) IsSingleSegment() bool {
	ci, ok := t.ConcatInfo()
	return !ok || ci.Total == 1
}

// MTI returns the MessageType from the first octet of the SMS TPDU.
func (t *TPDU) MTI() MessageType {
	return t.FirstOctet.MTI()
}

// Counter provides a reference counter that is incremented every time Count
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
// the message, and the TP-MR. The message must be coded for the UD of the
// template: GSM 7 bit septets, one per byte and unpacked, octets for 8 bit
// data, compressed data and the TP-CD of an SMS-COMMAND, and UTF-16, big
// endian, for UCS2.
//
// A message that fits in the UD of the template, including an empty message,
// results in a single TPDU. A longer message is split into segments, whose
// UDH is that of the template extended by a concatenation IE, as defined in
// 3GPP TS 23.040 Section 9.2.3.24.1, or 9.2.3.24.8 With16BitConcatRef. The
// template UDH must not contain a concatenation IE (ID 0 or 8) or the
// resulting TPDUs will be non-conformant. The segments do not split an escape
// sequence, as required by Section 9.2.3.24.1: "A character represented by an
// escape-sequence shall not be split in the middle", nor a UCS2 character or
// surrogate pair.
//
// Only an SMS-SUBMIT or SMS-DELIVER is split. Section 9.2.3.24.1 says "This
// facility allows short messages to be concatenated to form a longer
// message", and a short message is conveyed by an SMS-SUBMIT or an
// SMS-DELIVER (Section 9.2.2), while "SMS-COMMANDs identify messages by TP-MR
// and therefore apply to only one segment of a concatenated message", so an
// SMS-COMMAND split in segments would be as many commands. A message longer
// than the UD of a template of another type returns an ErrOverlength, in an
// EncodeError for the UD of the type, such as SmsCommand.ud, as MarshalBinary
// returns for such a TPDU.
//
// The TP-MR of an SMS-SUBMIT or SMS-COMMAND is allocated by its originator,
// which "increments TP-Message-Reference by 1 for each SMS-SUBMIT or
// SMS-COMMAND being submitted" (Section 9.2.3.6). For those types, each TPDU
// draws its TP-MR from the counter given by WithMR, and without it the TP-MR
// of each segment is the TP-MR of the template incremented by its position
// in the message, as Section 9.2.3.24.1 says: "TP-MR must be incremented for
// every segment of a concatenated message as defined in clause 9.2.3.6."
//
// Every TPDU of another type has the TP-MR of the template, and no counter
// is drawn from for it. The TP-MR of an SMS-STATUS-REPORT is that of the
// SMS-SUBMIT or SMS-COMMAND it reports on: "The value sent to the MS shall be
// the same as the TP-Message-Reference value generated by the MS in the
// earlier SMS-SUBMIT or SMS-COMMAND to which the status report relates"
// (Section 9.2.3.6). The other types have no TP-MR.
//
// An error is returned, rather than any TPDU, if the message is not valid for
// the coding, which is a byte above 0x7f for GSM 7 bit and an odd length for
// UCS2, if it is too long for a template that is not split, if the template
// UDH leaves no room for the message and a concatenation IE, or if the
// message needs more than the 255 segments that a concatenation IE can count.
// No counter is drawn from on error.
//
// The template, including the backing array of its UDH, is not changed.
func (t TPDU) Segment(msg []byte, options ...SegmentationOption) ([]TPDU, error) {
	cfg := segmentationConfig{newInfoElement, nil, nil}
	for _, o := range options {
		o(&cfg)
	}
	st := t.SmsType()
	switch st {
	case SmsDeliver, SmsDeliverReport, SmsSubmitReport, SmsSubmit,
		SmsStatusReport, SmsCommand:
	default:
		return nil, ErrUnsupportedSmsType(st)
	}
	coding := t.udCoding()
	if err := checkMessage(msg, coding); err != nil {
		return nil, err
	}
	// A template UDH that alone does not fit leaves a negative block size,
	// so even an empty message goes on to fail the check below.
	bs := t.UDBlockSize()
	if len(msg) <= bs {
		p := t
		p.UD = nil
		if len(msg) > 0 {
			p.UD = msg
		}
		if cfg.mr != nil && allocatesMR(st) {
			p.MR = byte(cfg.mr.Count())
		}
		return []TPDU{p}, nil
	}
	if st != SmsSubmit && st != SmsDeliver {
		return nil, NewEncodeError(st.String(), NewEncodeError("ud", ErrOverlength))
	}
	// the room left by the template UDH, which may be none, and the
	// concatenation IE.
	udho := udhOctets(t.UDH)
	if udho == 0 {
		udho = 1
	}
	bs = t.blockSize(udho + cfg.ief(0, 0, 0).marshalledLen())
	if bs < minBlockSize(msg, coding) {
		return nil, NewEncodeError("udh", ErrOverlength)
	}
	chunks := chunk(msg, coding, bs)
	count := len(chunks)
	// 3GPP TS 23.040 Sections 9.2.3.24.1 and 9.2.3.24.8: the total number of
	// short messages is an octet, "in the range 0 to 255", and a total of 0
	// is ignored.
	if count > maxSegments {
		return nil, NewEncodeError("sm", ErrTooManySegments)
	}
	concatRef := 1
	if cfg.cr != nil {
		concatRef = cfg.cr.Count()
	}
	pdus := make([]TPDU, count)
	for i := range chunks {
		p := t
		switch {
		case !allocatesMR(st):
		case cfg.mr != nil:
			p.MR = byte(cfg.mr.Count())
		default:
			p.MR = t.MR + byte(i)
		}
		udh := make(UserDataHeader, 0, len(t.UDH)+1)
		udh = append(udh, t.UDH...)
		udh = append(udh, cfg.ief(concatRef, count, i+1))
		p.SetUDH(udh)
		p.UD = chunks[i]
		pdus[i] = p
	}
	return pdus, nil
}

// allocatesMR reports whether the TP-MR of a TPDU of the type is allocated by
// its originator, which is so for an SMS-SUBMIT and an SMS-COMMAND (3GPP TS
// 23.040 Section 9.2.3.6).
func allocatesMR(st SmsType) bool {
	return st == SmsSubmit || st == SmsCommand
}

// checkMessage returns an error if the message is not valid for the coding.
func checkMessage(msg []byte, coding Alphabet) error {
	switch coding {
	case Alpha7Bit:
		for i, s := range msg {
			if s > 0x7f {
				return NewEncodeError("sm", gsm7.ErrInvalidSeptet{Offset: i, Septet: s})
			}
		}
	case AlphaUCS2:
		if len(msg)&0x1 == 0x1 {
			return NewEncodeError("sm", ErrOddUCS2Length)
		}
	}
	return nil
}

// minBlockSize returns the smallest block that can hold every character of
// the message, as "A character represented by an escape-sequence shall not be
// split in the middle" and "A UCS2 character shall not be split in the
// middle" (3GPP TS 23.040 Section 9.2.3.24.1). That is 2 septets if a GSM 7
// bit message has an escape and 1 otherwise, 4 octets if a UCS2 message has a
// high surrogate, which may start a surrogate pair, and 2 otherwise, and an
// octet for other data.
func minBlockSize(msg []byte, coding Alphabet) int {
	switch coding {
	case Alpha7Bit:
		if bytes.IndexByte(msg, esc) >= 0 {
			return 2
		}
		return 1
	case AlphaUCS2:
		for i := 0; i+1 < len(msg); i += 2 {
			if u := int(msg[i])<<8 | int(msg[i+1]); u >= surrHighStart && u < surrLowStart {
				return 4
			}
		}
		return 2
	default:
		return 1
	}
}

// maxSegments is the most segments a concatenated message can have, as the
// number of segments is carried in an octet of the concatenation IE.
const maxSegments = 255

// With16BitConcatRef specifies the usage of concat IEs with 16 bit reference
// numbers (ID=8).
//
// By default 8bit reference numbers are used.
var With16BitConcatRef = func(so *segmentationConfig) {
	so.ief = newInfoElement16bit
}

// WithMR provides an MR generator to provide the TP-MR field for TPDUs.
//
// It is drawn from once for each SMS-SUBMIT or SMS-COMMAND, whose TP-MR the
// originator allocates, and never for a TPDU of another type, which keeps the
// TP-MR of the template, as described for Segment.
//
// Without it, the MR of each segment of an SMS-SUBMIT or SMS-COMMAND is the
// MR of the template TPDU incremented by the position of the segment, so a
// single segment has the MR of the template.
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
//
// It replaces any UDH that was ignored when unmarshalling.
func (t *TPDU) SetUDH(udh UserDataHeader) {
	t.UDH = udh
	t.ignoredUDH = nil
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

// SetSmsType sets the type of SMS-TPDU this TPDU represents, which is its
// Direction and TP-MTI.
func (t *TPDU) SetSmsType(st SmsType) error {
	if st < 0 || st > SmsCommand {
		return ErrInvalid
	}
	t.Direction = st.Direction()
	t.FirstOctet = t.FirstOctet.WithMTI(st.MTI())
	return nil
}

// UDBlockSize returns the maximum size of a block of UserData that can fit in
// this TPDU, given its UDH, which is the most UD that MarshalBinary accepts.
//
// The interpretation of the size depends on the encoding - for 7bit encoding
// it is the number of septets. For all other encodings, including compressed
// data and the TP-CD of an SMS-COMMAND, it is the number of octets, and for
// UCS2 it is even.
//
// The size is negative if the UDH alone does not fit in the TPDU.
func (t *TPDU) UDBlockSize() int {
	if t.keepsIgnoredUDH() {
		return t.blockSize(len(t.ignoredUDH))
	}
	return t.blockSize(udhOctets(t.UDH))
}

// keepsIgnoredUDH reports whether the TPDU marshals the octets of a UDH that
// was ignored when unmarshalling, which it does while UDH is left empty.
func (t *TPDU) keepsIgnoredUDH() bool {
	return t.ignoredUDH != nil && t.UDH != nil && len(t.UDH) == 0
}

// marshalUDH returns the octets of the UDH, including its UDHL, which are
// those of a UDH that was ignored when unmarshalling if it is kept.
func (t *TPDU) marshalUDH() ([]byte, error) {
	if t.keepsIgnoredUDH() {
		return append([]byte(nil), t.ignoredUDH...), nil
	}
	return t.UDH.MarshalBinary()
}

// udhOctets returns the number of octets taken in the TP-UD by the UDH,
// including its UDHL, which is none for a nil UDH.
func udhOctets(udh UserDataHeader) int {
	if udh == nil {
		return 0
	}
	return udh.UDHL() + 1
}

// blockSize returns the room left for the short message in the TP-UD, in
// septets for 7bit encoding and in octets otherwise, by a header that takes
// udho octets.
func (t *TPDU) blockSize(udho int) int {
	room := t.maxUDOctets()
	switch t.udCoding() {
	case Alpha7Bit:
		// 3GPP TS 23.040 Section 9.2.3.16: the TP-UDL counts the septets
		// of the header, including its fill bits, and the short message,
		// and the octets they take must fit the room.
		return room*8/7 - (udho*8+6)/7
	case AlphaUCS2:
		// 3GPP TS 23.040 Section 9.2.3.24.1: "A UCS2 character shall not
		// be split in the middle".
		return (room - udho) &^ 0x1
	default:
		return room - udho
	}
}

// maxUDOctets returns the most octets the TP-UD, or the TP-CD of an
// SMS-COMMAND, including any header, can take in the TPDU.
//
// For SMS-SUBMIT and SMS-DELIVER, 3GPP TS 23.040 Section 3.1 says "The text
// messages to be transferred by means of the SM MT or SM MO contain up to 140
// octets", as does 3GPP TS 23.038 Section 4.
//
// For the others it is the TP-UD, or TP-CD, of the layout in 3GPP TS 23.040
// Section 9.2.2: "0 to 159" for an SMS-DELIVER-REPORT for RP-ACK and "0 to
// 158" for RP-ERROR (9.2.2.1a), and "0 to 152" and "0 to 151" for an
// SMS-SUBMIT-REPORT (9.2.2.2a). The layouts show a TP-PI of one octet, and
// any extension octets are taken from the TP-UD.
//
// For an SMS-STATUS-REPORT, Section 9.2.2.3 gives "0 to 143" and says "In
// order to achieve the maximum stated above (143 octets), the TP-RA field
// must have a length of 2 octets and TP-PID and TP-DCS must not be present".
// For an SMS-COMMAND, Section 9.2.2.4 gives "0 to 156" and says "In order to
// achieve the maximum stated above (156 octets), the TP-DA field must have a
// length of 2 octets". So each octet of those fields is taken from the TP-UD
// or TP-CD. Section 9.2.3.21 gives a TP-CD maximum of 157 octets, which no
// TP-DA, of at least 2 octets, leaves room for.
func (t *TPDU) maxUDOctets() int {
	var room int
	switch t.SmsType() {
	case SmsSubmit, SmsDeliver:
		room = MaxUDL
	case SmsDeliverReport:
		room = 159 - len(t.PIExt)
		if t.RPMessage == RPError {
			room = 158 - len(t.PIExt)
		}
	case SmsSubmitReport:
		room = 152 - len(t.PIExt)
		if t.RPMessage == RPError {
			room = 151 - len(t.PIExt)
		}
	case SmsStatusReport:
		room = 143 - (addressOctets(&t.RA) - 2) - len(t.PIExt)
		if t.PI.PID() || t.PID != 0 {
			room--
		}
		if t.PI.DCS() || t.DCS != 0 {
			room--
		}
	case SmsCommand:
		room = 156 - (addressOctets(&t.DA) - 2)
	}
	// The room is negative if the TP-PI extension octets alone make the
	// TPDU longer than the 164 octets a TPDU may be (3GPP TS 27.005 Section
	// 2.5.2.6, "The Short Message is of variable length, 6-164 octets").
	return room
}

// addressOctets returns the number of octets the address takes in a TPDU,
// which is the maximum of 12 if it cannot be marshalled.
func addressOctets(a *Address) int {
	b, err := a.MarshalBinary()
	if err != nil {
		return 12
	}
	return len(b)
}

// FailureCause returns the TP-FCS of an SMS-DELIVER-REPORT or
// SMS-SUBMIT-REPORT as a receiver is to take it.
//
// For a report for RP-ERROR whose first octet has any of bits 7 and 5 to 2
// set, it is 0xFF, "Unspecified error cause" (3GPP TS 23.040 Section
// 9.2.3.22), whatever the FCS holds, as Sections 9.2.2.1a and 9.2.2.2a say:
// "Bits 7 and 5 - 2 in octet 1 are presently unused and the sender shall set
// them to zero. If any of these bits is non-zero, the receiver shall not
// examine the other field and shall treat the TP-Failure-Cause as
// "Unspecified error cause"". Otherwise it is the FCS, reserved values
// included.
func (t *TPDU) FailureCause() byte {
	if t.unusedReportBits() {
		return 0xff
	}
	return t.FCS
}

// foReportUnused masks the unused bits, 7 and 5 to 2, of the first octet of
// an SMS-DELIVER-REPORT or SMS-SUBMIT-REPORT (3GPP TS 23.040 Sections
// 9.2.2.1a and 9.2.2.2a).
const foReportUnused FirstOctet = 0xbc

// unusedReportBits reports whether the TPDU is an SMS-DELIVER-REPORT or
// SMS-SUBMIT-REPORT for RP-ERROR with an unused bit of its first octet set,
// whose fields other than the TP-FCS a receiver does not examine. In a report
// for RP-ACK the receiver ignores those bits instead.
func (t *TPDU) unusedReportBits() bool {
	switch t.SmsType() {
	case SmsDeliverReport, SmsSubmitReport:
		return t.RPMessage == RPError && t.FirstOctet&foReportUnused != 0
	default:
		return false
	}
}

// keepsUnexamined reports whether the TPDU marshals the octets that followed
// the TP-FCS of the report it was unmarshalled from, rather than its fields,
// which it does while it is still a report whose fields are not examined,
// and those fields are left empty.
//
// It is called on the TPDU with its flags derived, as described for
// MarshalBinary, whose PI is not 0 if the PID, DCS, UDH or UD is set.
func (t *TPDU) keepsUnexamined() bool {
	return t.unexamined != nil && t.unusedReportBits() &&
		t.PI == 0 && len(t.PIExt) == 0 && t.SCTS == (Timestamp{})
}

// withUnexamined returns the octets of a report whose fields are not
// examined: its first octet, its TP-FCS and the octets that followed it.
func (t *TPDU) withUnexamined(fcs []byte) []byte {
	b := make([]byte, 0, 1+len(fcs)+len(t.unexamined))
	b = append(b, byte(t.FirstOctet))
	b = append(b, fcs...)
	return append(b, t.unexamined...)
}

// UDHI returns the User Data Header Indicator bit from the SMS TPDU first
// octet, as held.
//
// After SetUDH it is set exactly when the UDH is not nil. After
// UnmarshalBinary it is the bit as received, which, for a TPDU that has a
// TP-UD, is set exactly when the UDH is not nil. A TPDU that has no TP-UD,
// such as one with a TP-UDL of 0, a report whose TP-PI announces no TP-UDL,
// or one whose fields are not examined, as described for UnmarshalBinary,
// has no header to decode, so its UDH is nil whatever the bit. To
// find whether a TPDU has a header, test the UDH rather than this bit.
//
// MarshalBinary derives the bit from the UDH and UD where they are written,
// and otherwise marshals it as held.
func (t *TPDU) UDHI() bool {
	return t.FirstOctet.UDHI()
}

// UDHL returns the encoded length of the UDH, not including the UDHL itself.
//
// For a UDH that was ignored when unmarshalling it is the UDHL as received.
func (t *TPDU) UDHL() int {
	if t.keepsIgnoredUDH() {
		return len(t.ignoredUDH) - 1
	}
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
// are the bits that describe no field, such as the reserved PI bits.
//
// So a TPDU that was unmarshalled, and not changed since, marshals back to
// the octets it came from, other than these, which are marshalled in the
// form 3GPP TS 23.040 has a sender use:
//
//   - The octets that follow the TP-UD of a report with a reserved TP-PI
//     bit set, which were discarded (Section 9.2.3.27).
//   - The fill bits that follow a UDH in GSM 7 bit user data, and the unused
//     bits that follow its last septet, which a receiver ignores, and are
//     marshalled as zero (Sections 9.2.2.1 and 9.2.3.24).
//   - The TP-UDL of GSM 7 bit user data that holds only a UDH, if it counts
//     fewer septets than the UDH and its fill bits, which is marshalled as
//     the count Section 9.2.3.16 gives.
//   - An address in another form than the one it marshals to: one with a
//     fill semi-octet, 1111, before its last, or with a last fill semi-octet
//     other than 1111 (Section 9.1.2.3), or an alphanumeric one whose
//     Address-Length counts more semi-octets than its septets use, whose
//     unused bits are not zero (Section 9.1.2.5), or that has an escape with
//     no character, which is decoded as a substitute (3GPP TS 23.038
//     Sections 6.2.1 and 6.2.1.1), as Address.UnmarshalBinary describes.
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
	// The TP-CDL and TP-CD are coded as a TP-UDL and TP-UD of 8 bit data, so
	// the TP-CD may start with a header, as 3GPP TS 23.040 Section 9.2.2.4
	// says of the TP-UDHI: "Parameter indicating that the TP-CD field
	// contains a Header".
	cd, err := t.encodeUserData()
	if err != nil {
		return nil, NewEncodeError("ud", err)
	}
	b := make([]byte, 0, 5+len(da)+len(cd))
	b = append(b, byte(t.FirstOctet), t.MR, t.PID, t.CT, t.MN)
	b = append(b, da...)
	b = append(b, cd...)
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
	if t.keepsUnexamined() {
		return t.withUnexamined(fcs), nil
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
	if t.maxUDOctets() < 0 {
		return nil, NewEncodeError("pi", ErrOverlength)
	}
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
	if len(pi) == 1 && pi[0] == 0 && !t.zeroPI {
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
	if t.keepsUnexamined() {
		return t.withUnexamined(fcs), nil
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
// Of an SMS-DELIVER-REPORT or SMS-SUBMIT-REPORT for RP-ERROR with any of
// bits 7 and 5 to 2 of its first octet set, only the first octet and the
// TP-FCS are decoded, as "the receiver shall not examine the other field"
// (3GPP TS 23.040 Sections 9.2.2.1a and 9.2.2.2a), so whatever follows the
// TP-FCS is accepted, and FailureCause returns 0xFF. Those octets are kept,
// and MarshalBinary writes them back while the fields they would have
// decoded to are left empty.
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
	if t.maxUDOctets() < 0 {
		return ri, NewDecodeError("pi", ri, ErrOverlength)
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

// maxTPDULen is the length of the longest TPDU: "The Short Message is of
// variable length, 6-164 octets" (3GPP TS 27.005 Section 2.5.2.6).
const maxTPDULen = 164

// unmarshalUnexamined keeps the octets that follow the TP-FCS, at src[ri:],
// of a report whose fields are not examined, and reports whether it is one.
//
// src follows the first octet, so the report, which is not examined, must
// still be no longer than a TPDU can be.
func (t *TPDU) unmarshalUnexamined(src []byte, ri int) (bool, error) {
	if !t.unusedReportBits() {
		return false, nil
	}
	if 1+len(src) > maxTPDULen {
		// the octets past the limit are in the fields that start with the
		// TP-PI, which are not examined.
		return true, NewDecodeError("pi", ri, ErrOverlength)
	}
	t.unexamined = append([]byte{}, src[ri:]...)
	return true, nil
}

func (t *TPDU) unmarshalDeliverReport(src []byte) error {
	ri, err := t.unmarshalFCS(src)
	if err != nil {
		return err
	}
	if unexamined, err := t.unmarshalUnexamined(src, ri); unexamined {
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
	// A TP-PI of 0 has no extension bit, so it has no extension octets.
	t.zeroPI = t.PI == 0
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
	if unexamined, err := t.unmarshalUnexamined(src, ri); unexamined {
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
	alphabet := t.udCoding()
	if alphabet == Alpha7Bit {
		sml7 = udl
		// length is septets - convert to octets
		udl = (sml7*7 + 7) / 8
	}
	if udl > t.maxUDOctets() {
		return 0, NewDecodeError("udl", 0, ErrOverlength)
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
		if len(udh) == 0 && l > 1 {
			// the UDH was ignored, but its octets are kept
			t.ignoredUDH = append([]byte(nil), src[ri:ri+l]...)
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
		if len(sm) > 0 {
			t.UD = sm
		}
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
		if surplus > 1 {
			return nil, ErrOverlength
		}
		// The last octet has 7 spare bits, which unpack as a septet. 3GPP
		// TS 23.040 Section 9.2.2.1: "Any unused bits shall be set to zero
		// by the sending entity and shall be ignored by the receiving
		// entity", so it is dropped, whatever its value.
		sm = sm[:sml]
	}
	return sm, nil
}

// encodeUserData marshals the TP-UDL and the TP-UD, with the UDH, if not nil,
// and the UD.
//
// If the UD is coded as GSM 7 bit then it is assumed to be unpacked septets,
// which are packed after the UDH and its fill bits. Otherwise, including for
// compressed data, the UD is encoded as is.
func (t *TPDU) encodeUserData() (b []byte, err error) {
	udh, err := t.marshalUDH()
	if err != nil {
		return nil, NewEncodeError("udh", err)
	}
	ud := t.UD
	alphabet := t.udCoding()
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
		// 3GPP TS 23.040 Section 9.2.3.16: "If a TP-User-Data-Header field
		// is present, then the TP-User-Data-Length value is the sum of the
		// number of septets in the TP-User-Data-Header field (including any
		// padding) and the number of septets in the TP-User-Data field which
		// follows."
		udl += (len(udh)*8 + fillBits) / 7
		// With no septets to pack, the fill bits of the header still
		// occupy an octet, which Pack7Bit does not provide.
		if pad := (udl*7+7)/8 - len(udh) - len(ud); pad > 0 {
			ud = append(ud, make([]byte, pad)...)
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
	if len(udh)+len(ud) > t.maxUDOctets() {
		return nil, ErrOverlength
	}
	b = make([]byte, 0, 1+len(udh)+len(ud))
	b = append(b, byte(udl))
	b = append(b, udh...)
	b = append(b, ud...)
	return b, nil
}

// MaxUDL is the maximum number of octets of the TP-UD of an SMS-SUBMIT or
// SMS-DELIVER, including any header, as defined in 3GPP TS 23.040 Section 3.1.
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
//
// An error is returned if the Direction is neither MT nor MO.
func (d Direction) ApplyTPDUOption(t *TPDU) error {
	if d != MT && d != MO {
		return ErrInvalid
	}
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
//
// A direction other than MT and MO is of no type, rather than being combined
// with the TP-MTI, which would make it that of another type.
func smsType(mt MessageType, dir Direction) SmsType {
	switch dir {
	case MT:
		if mt == MtReserved {
			return SmsDeliver
		}
	case MO:
	default:
		return noSmsType
	}
	return SmsType(byte(mt<<1) | byte(dir))
}

// noSmsType is the SmsType of a TPDU that has a Direction of neither MT nor
// MO, which is of no type.
const noSmsType SmsType = -1

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
//
// The bs must be at least minBlockSize, and chunk returns nil if it is not.
func chunk(msg []byte, alpha Alphabet, bs int) [][]byte {
	if bs < minBlockSize(msg, alpha) {
		return nil
	}
	switch alpha {
	default: // default to 7Bit
		return chunk7Bit(msg, bs)
	case AlphaUCS2:
		return chunkUCS2(msg, bs)
	case Alpha8Bit:
		return chunk8Bit(msg, bs)
	}
}

// chunk7Bit splits a GSM7 message into chunks that are not larger than bs,
// which must be at least 2 if the message has an escape, and at least 1.
//
// Escape sequences are not split across blocks, so the resulting blocks may
// be one septet shorter than bs. An escape followed by an escape is itself an
// escape sequence, as 3GPP TS 23.038 Section 6.2.1.1 reserves it "for the
// extension to another extension table", so the escapes of a run pair up
// from its start, and a chunk that ends with an odd number of escapes ends
// with the first septet of a sequence.
func chunk7Bit(msg []byte, bs int) [][]byte {
	if len(msg) == 0 {
		return nil
	}
	chunks := make([][]byte, 0, 1+len(msg)/bs)
	bstart := 0
	for len(msg)-bstart > bs {
		bend := bstart + bs
		// Each chunk starts at the start of a character, so the escapes
		// at its end pair up from the first of the run in the chunk.
		n := 0
		for i := bend - 1; i >= bstart && msg[i] == esc; i-- {
			n++
		}
		if n%2 == 1 && bend-1 > bstart {
			bend--
		}
		chunks = append(chunks, msg[bstart:bend])
		bstart = bend
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
	chunks := make([][]byte, 0, 1+len(msg)/bs)
	bstart := 0
	for len(msg)-bstart > bs {
		chunks = append(chunks, msg[bstart:bstart+bs])
		bstart += bs
	}
	chunks = append(chunks, msg[bstart:])
	return chunks
}

const (
	surrHighStart = 0xd800
	surrLowStart  = 0xdc00
)

// chunkUCS2 splits a UCS2/UTF-16 message into chunks that are not larger than
// bs, which must be at least 2, and should be at least 4.
//
// bs should be even, but if odd is reduced by one.
// To allow for reassemblers that cannot handle split surrogate pairs, they are
// not split during chunking, so the resulting blocks may be slightly smaller
// than bs whenever a surrogate pair would span a block boundary, unless bs is
// 2, which cannot hold a pair.
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
	chunks := make([][]byte, 0, 1+len(msg)/bs)
	bstart := 0
	for len(msg)-bstart > bs {
		bend := bstart + bs
		// check last uint16 is a high surrogate, if so then leave for later
		r := binary.BigEndian.Uint16(msg[bend-2 : bend])
		if surrHighStart <= r && r < surrLowStart && bend-2 > bstart {
			bend -= 2
		}
		chunks = append(chunks, msg[bstart:bend])
		bstart = bend
	}
	chunks = append(chunks, msg[bstart:])
	return chunks
}
