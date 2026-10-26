// SPDX-License-Identifier: MIT

package tpdu

import (
	"encoding/binary"
	"slices"
)

// PortAddress is the application port addressing information defined in
// 3GPP TS 23.040 Sections 9.2.3.24.3 and 9.2.3.24.4. Values are retained
// as received; Valid reports whether both ports have defined values.
type PortAddress struct {
	// Dest identifies the "receiving port, i.e. application, in the receiving
	// device", as defined in 3GPP TS 23.040 Sections 9.2.3.24.3 and 9.2.3.24.4.
	Dest int

	// Orig identifies the "sending port, i.e. application, in the sending
	// device", as defined in 3GPP TS 23.040 Sections 9.2.3.24.3 and 9.2.3.24.4.
	Orig int

	// Port16Bit indicates the 16-bit addressing scheme of 3GPP TS 23.040
	// Section 9.2.3.24.4, rather than the 8-bit scheme of Section 9.2.3.24.3.
	Port16Bit bool
}

// Valid reports whether both ports are defined by 3GPP TS 23.040 Sections
// 9.2.3.24.3 and 9.2.3.24.4 and fit in their addressing space.
// The 8-bit table labels 0 to 239 "Reserved" and 240 to 255 "Available for
// allocation by applications". The 16-bit table labels 0 to 15999 and
// 17000 to 49151 "UDP/TCP port numbers assigned by IANA", 16000 to 16999
// "Available for allocation by SMS applications without the need to refer
// to 3GPP or IANA", and 49152 "Trigger to establish a PDN connection of
// non-IP PDN type using the default APN." Values 49153 to 65535 are
// "Reserved for future allocation by 3GPP."
func (p PortAddress) Valid() bool {
	minPort, maxPort := 240, 255
	if p.Port16Bit {
		minPort, maxPort = 0, 49152
	}
	return p.Dest >= minPort && p.Dest <= maxPort && p.Orig >= minPort && p.Orig <= maxPort
}

// PortAddress decodes an application port addressing IE without checking its
// ranges, as defined in 3GPP TS 23.040 Sections 9.2.3.24.3 and 9.2.3.24.4.
// The 16-bit ports use the integer representation of Section 9.1.2.1.
// Another IEI returns a DecodeError for iei at offset 0 wrapping ErrInvalid.
// Data shorter or longer than 2 octets (8-bit) or 4 octets (16-bit) returns
// a DecodeError for ied at offset 2 wrapping ErrUnderflow or ErrOverlength,
// respectively. Every error returns a zero PortAddress.
func (ie InformationElement) PortAddress() (PortAddress, error) {
	length := 2
	if ie.ID == IEIPort16Bit {
		length = 4
	} else if ie.ID != IEIPort8Bit {
		return PortAddress{}, NewDecodeError("iei", 0, ErrInvalid)
	}
	if err := ie.dataLength(length); err != nil {
		return PortAddress{}, err
	}
	if ie.ID == IEIPort16Bit {
		return PortAddress{
			Dest:      int(binary.BigEndian.Uint16(ie.Data[:2])),
			Orig:      int(binary.BigEndian.Uint16(ie.Data[2:])),
			Port16Bit: true,
		}, nil
	}
	return PortAddress{Dest: int(ie.Data[0]), Orig: int(ie.Data[1])}, nil
}

// PortAddress returns the last application port addressing IE that decodes
// and is Valid, or a zero PortAddress and false if there is none.
// 3GPP TS 23.040 Sections 9.2.3.24.3 and 9.2.3.24.4 state: "A receiving
// entity shall ignore (i.e. skip over and commence processing at the next
// information element) any information element where the value of the
// Information-Element-Data is Reserved or not supported."
// Section 9.2.3.24 states: "In the event that two or more IEs occur which
// have mutually exclusive meanings (e.g. an 8bit port address and a 16bit
// port address), then the last occurring IE shall be used."
// IEs with the wrong length are skipped. Application support for a port
// remains the caller's responsibility.
func (udh UserDataHeader) PortAddress() (PortAddress, bool) {
	for i := len(udh) - 1; i >= 0; i-- {
		if p, err := udh[i].PortAddress(); err == nil && p.Valid() {
			return p, true
		}
	}
	return PortAddress{}, false
}

// SpecialMessageType is the basic message indication type in bits 0 and 1
// of octet 1, as defined in 3GPP TS 23.040 Section 9.2.3.24.2.
type SpecialMessageType int

const (
	// SpecialMessageVoice means "Voice Message Waiting", as defined in
	// 3GPP TS 23.040 Section 9.2.3.24.2.
	SpecialMessageVoice SpecialMessageType = 0
	// SpecialMessageFax means "Fax Message Waiting", as defined in
	// 3GPP TS 23.040 Section 9.2.3.24.2.
	SpecialMessageFax SpecialMessageType = 1
	// SpecialMessageEmail means "Electronic Mail Message Waiting", as
	// defined in 3GPP TS 23.040 Section 9.2.3.24.2.
	SpecialMessageEmail SpecialMessageType = 2
	// SpecialMessageExtended means "Extended Message Type Waiting
	// (equivalent to \"other\" in 3GPP TS 23.038 [9])", as defined in
	// 3GPP TS 23.040 Section 9.2.3.24.2.
	SpecialMessageExtended SpecialMessageType = 3

	// SpecialMessageNoExtendedType means "No extended message indication
	// type", as defined in 3GPP TS 23.040 Section 9.2.3.24.2.
	SpecialMessageNoExtendedType = 0
	// SpecialMessageVideo means "Video Message Waiting", as defined in
	// 3GPP TS 23.040 Section 9.2.3.24.2. It is defined only with the basic
	// type SpecialMessageExtended.
	SpecialMessageVideo = 1
)

// SpecialMessageIndication is the message waiting information defined in
// 3GPP TS 23.040 Section 9.2.3.24.2. Reserved values are retained; Valid
// reports whether all values are defined and fit in the element.
type SpecialMessageIndication struct {
	// Type is the basic message indication type in bits 0 and 1 of octet 1,
	// as defined in 3GPP TS 23.040 Section 9.2.3.24.2.
	Type SpecialMessageType
	// ExtendedType is bits 4, 3 and 2 of octet 1, as defined in
	// 3GPP TS 23.040 Section 9.2.3.24.2. Zero means "No extended message
	// indication type"; one means "Video Message Waiting" when Type is
	// SpecialMessageExtended. Other combinations are Reserved.
	ExtendedType int
	// Profile is the profile ID, 1 to 4, encoded in bits 6 and 5 of octet 1,
	// as defined in 3GPP TS 23.040 Section 9.2.3.24.2.
	Profile int
	// Store is bit 7 of octet 1: "Store message after updating indication"
	// when set, or "Discard message after updating indication" when clear,
	// as defined in 3GPP TS 23.040 Section 9.2.3.24.2. In a conflict with
	// the DCS, the message is stored if either requests storage.
	Store bool
	// Count is octet 2, the number of messages waiting, from 0 to 255.
	// 3GPP TS 23.040 Section 9.2.3.24.2 states: "The value 255 shall be
	// taken to mean 255 or greater."
	Count int
}

// Valid reports whether the values are defined and fit in the element of
// 3GPP TS 23.040 Section 9.2.3.24.2. The basic type is 0 to 3, the profile
// is 1 to 4, and the count is 0 to 255. Extended type zero means no extended
// type. Extended type one is defined only with basic type three; the other
// extended type combinations are Reserved.
func (s SpecialMessageIndication) Valid() bool {
	return s.Type >= SpecialMessageVoice && s.Type <= SpecialMessageExtended &&
		(s.ExtendedType == SpecialMessageNoExtendedType || s.Type == SpecialMessageExtended && s.ExtendedType == SpecialMessageVideo) &&
		s.Profile >= 1 && s.Profile <= 4 && s.Count >= 0 && s.Count <= 255
}

// SpecialMessageIndication decodes the two octets defined in 3GPP TS 23.040
// Section 9.2.3.24.2 without rejecting Reserved values. Another IEI returns
// a DecodeError for iei at offset 0 wrapping ErrInvalid. Shorter or longer
// data returns a DecodeError for ied at offset 2 wrapping ErrUnderflow or
// ErrOverlength, respectively. Every error returns a zero value.
func (ie InformationElement) SpecialMessageIndication() (SpecialMessageIndication, error) {
	if ie.ID != IEISpecialMessageIndication {
		return SpecialMessageIndication{}, NewDecodeError("iei", 0, ErrInvalid)
	}
	if err := ie.dataLength(2); err != nil {
		return SpecialMessageIndication{}, err
	}
	b := ie.Data[0]
	return SpecialMessageIndication{
		Type:         SpecialMessageType(b & 0x03),
		ExtendedType: int(b >> 2 & 0x07),
		Profile:      int(b>>5&0x03) + 1,
		Store:        b&0x80 != 0,
		Count:        int(ie.Data[1]),
	}, nil
}

// SpecialMessageIndications returns every indication that decodes, in order,
// including Reserved values. 3GPP TS 23.040 Section 9.2.3.24.2 states:
// "Terminals should be capable of receiving any values in octet 1, including
// those marked as Reserved." Section 9.2.3.24 lists this IE as repeatable.
// IEs with the wrong length are skipped; if none decode the result is nil.
func (udh UserDataHeader) SpecialMessageIndications() []SpecialMessageIndication {
	var indications []SpecialMessageIndication
	for _, ie := range udh {
		if s, err := ie.SpecialMessageIndication(); err == nil {
			indications = append(indications, s)
		}
	}
	return indications
}

// SMSCControlParameters contains octet 1, Selective Status Report, as
// defined in 3GPP TS 23.040 Section 9.2.3.24.5. "The TP-SRR must be set in
// order for the Selective Status Report to be enabled."
type SMSCControlParameters struct {
	// ReportCompleted is bit 0: "Status Report for short message transaction
	// completed", as defined in 3GPP TS 23.040 Section 9.2.3.24.5.
	ReportCompleted bool
	// ReportPermanentError is bit 1: "Status Report for permanent error
	// when SC is not making any more transfer attempts", as defined in
	// 3GPP TS 23.040 Section 9.2.3.24.5.
	ReportPermanentError bool
	// ReportTemporaryErrorNoRetry is bit 2: "Status Report for temporary
	// error when SC is not making any more transfer attempts", as defined
	// in 3GPP TS 23.040 Section 9.2.3.24.5.
	ReportTemporaryErrorNoRetry bool
	// ReportTemporaryErrorRetry is bit 3: "Status Report for temporary error
	// when SC is still trying to transfer SM", as defined in
	// 3GPP TS 23.040 Section 9.2.3.24.5.
	ReportTemporaryErrorRetry bool
	// Reserved retains bits 4 and 5 in their original positions (mask 0x30).
	// They are "reserved for future use" in 3GPP TS 23.040 Section 9.2.3.24.5.
	Reserved int
	// CancelRemainingReports is bit 6, as defined in 3GPP TS 23.040 Section
	// 9.2.3.24.5: "A Status Report generated by this Short Message, due to a
	// permanent error or last temporary error, cancels the SRR of the rest
	// of the Short Messages in a concatenated message." This requires a
	// service centre aware of the segmentation.
	CancelRemainingReports bool
	// IncludeOriginalUDH is bit 7: "Include original UDH into the Status
	// Report", as defined in 3GPP TS 23.040 Section 9.2.3.24.5.
	IncludeOriginalUDH bool
}

// Valid reports whether Reserved is zero, as bits 4 and 5 are reserved for
// future use by 3GPP TS 23.040 Section 9.2.3.24.5.
func (s SMSCControlParameters) Valid() bool { return s.Reserved == 0 }

// SMSCControlParameters decodes the octet defined in 3GPP TS 23.040 Section
// 9.2.3.24.5, retaining reserved bits. Another IEI returns a DecodeError for
// iei at offset 0 wrapping ErrInvalid. Shorter or longer data returns a
// DecodeError for ied at offset 2 wrapping ErrUnderflow or ErrOverlength,
// respectively. Every error returns a zero value.
func (ie InformationElement) SMSCControlParameters() (SMSCControlParameters, error) {
	if ie.ID != IEISMSCControlParameters {
		return SMSCControlParameters{}, NewDecodeError("iei", 0, ErrInvalid)
	}
	if err := ie.dataLength(1); err != nil {
		return SMSCControlParameters{}, err
	}
	b := ie.Data[0]
	return SMSCControlParameters{
		ReportCompleted:             b&0x01 != 0,
		ReportPermanentError:        b&0x02 != 0,
		ReportTemporaryErrorNoRetry: b&0x04 != 0,
		ReportTemporaryErrorRetry:   b&0x08 != 0,
		Reserved:                    int(b & 0x30),
		CancelRemainingReports:      b&0x40 != 0,
		IncludeOriginalUDH:          b&0x80 != 0,
	}, nil
}

// SMSCControlParameters returns the last IE that decodes, whether or not
// Valid, or a zero value and false if there is none. IEs with the wrong
// length are skipped. 3GPP TS 23.040 Section 9.2.3.24 states: "In the event
// that IEs determined as not repeatable are duplicated, the last occurrence
// of the IE shall be used." Its table lists SMSC Control Parameters as not
// repeatable. It also states: "If any reserved values are received within
// the content of any Information Element then that part of the Information
// Element shall be ignored." The caller therefore ignores Reserved when
// applying the parameters; reserved bits do not discard the entire IE.
func (udh UserDataHeader) SMSCControlParameters() (SMSCControlParameters, bool) {
	for i := len(udh) - 1; i >= 0; i-- {
		if s, err := udh[i].SMSCControlParameters(); err == nil {
			return s, true
		}
	}
	return SMSCControlParameters{}, false
}

// UDHSource identifies the source of the following part of a User Data
// Header, as defined in 3GPP TS 23.040 Section 9.2.3.24.6.
type UDHSource int

const (
	// UDHSourceOriginalSender means "The following part of the UDH is
	// created by the original sender (valid in case of Status Report)",
	// as defined in 3GPP TS 23.040 Section 9.2.3.24.6.
	UDHSourceOriginalSender UDHSource = 1
	// UDHSourceOriginalReceiver means "The following part of the UDH is
	// created by the original receiver (valid in case of Status Report)",
	// as defined in 3GPP TS 23.040 Section 9.2.3.24.6.
	UDHSourceOriginalReceiver UDHSource = 2
	// UDHSourceSMSC means "The following part of the UDH is created by the
	// SMSC (can occur in any message or report)", as defined in
	// 3GPP TS 23.040 Section 9.2.3.24.6.
	UDHSourceSMSC UDHSource = 3
)

// Valid reports whether the source is one of the three values defined in
// 3GPP TS 23.040 Section 9.2.3.24.6.
func (s UDHSource) Valid() bool { return s >= UDHSourceOriginalSender && s <= UDHSourceSMSC }

// UDHSource decodes the octet defined in 3GPP TS 23.040 Section 9.2.3.24.6,
// retaining undefined source values. Another IEI returns a DecodeError for
// iei at offset 0 wrapping ErrInvalid. Shorter or longer data returns a
// DecodeError for ied at offset 2 wrapping ErrUnderflow or ErrOverlength,
// respectively. Every error returns a zero source.
func (ie InformationElement) UDHSource() (UDHSource, error) {
	if ie.ID != IEIUDHSource {
		return 0, NewDecodeError("iei", 0, ErrInvalid)
	}
	if err := ie.dataLength(1); err != nil {
		return 0, err
	}
	return UDHSource(ie.Data[0]), nil
}

// UDHSection is a part of a header belonging to one source, as defined in
// 3GPP TS 23.040 Section 9.2.3.24.6.
type UDHSection struct {
	// Source is the source indicator value from 3GPP TS 23.040 Section
	// 9.2.3.24.6, or zero for the elements before the first indicator,
	// which no indicator attributes to a source.
	Source UDHSource
	// Elements is the content up to the next source indicator or the end
	// of the header, excluding the indicator itself, as defined in
	// 3GPP TS 23.040 Section 9.2.3.24.6. It is nil for an empty section.
	Elements UserDataHeader
}

// Sections splits the header according to 3GPP TS 23.040 Section 9.2.3.24.6:
// "The Source Indicator is placed in front of the content inserted by the
// source. The indicated content (one or more Information-Elements) ends at
// the next UDH-Source-Indicator, or at the end of the UDH."
//
// Leading elements form a section with zero Source. Each indicator that
// decodes starts a section, even if its source is undefined or its content
// is empty. Indicators with the wrong length remain among the elements.
// A header without indicators forms one section; an empty header gives nil.
// The sections' Elements slices and their Data share memory with the header.
// Each slice's capacity is limited to its length, so appending does not
// overwrite the next source indicator. Empty sections have nil Elements.
func (udh UserDataHeader) Sections() []UDHSection {
	var sections []UDHSection
	var source UDHSource
	start := 0
	appendSection := func(end int) {
		section := UDHSection{Source: source}
		if start < end {
			section.Elements = udh[start:end:end]
		}
		sections = append(sections, section)
	}
	for i, ie := range udh {
		if s, err := ie.UDHSource(); err == nil {
			if i > 0 {
				appendSection(i)
			}
			source, start = s, i+1
		}
	}
	if len(udh) > 0 {
		appendSection(len(udh))
	}
	return sections
}

// dataLength returns the DecodeError for data of other than length octets,
// the length the IE is coded with, or nil.
func (ie InformationElement) dataLength(length int) error {
	if len(ie.Data) < length {
		return NewDecodeError("ied", 2, ErrUnderflow)
	}
	if len(ie.Data) > length {
		return NewDecodeError("ied", 2, ErrOverlength)
	}
	return nil
}

// EmailHeaderLength decodes the IETF RFC 5322 E-Mail Header length indicator
// in 3GPP TS 23.040 Section 9.2.3.24.11. It counts the header, or its fraction
// in this segment, at the beginning of the short message's data part.
// The section specifies "the number of septets" for the GSM 7 bit default
// alphabet, "the number of octets" for 8-bit data, and "the number of UCS2
// characters (consisting of 2 octets)" for UCS2. For compressed data the
// value is "based on the amount of uncompressed data, i.e. before compression
// is performed." Every octet value is defined.
// Another IEI returns a DecodeError for iei at offset 0 wrapping ErrInvalid.
// Shorter or longer data returns a DecodeError for ied at offset 2 wrapping
// ErrUnderflow or ErrOverlength, respectively. Every error returns zero.
func (ie InformationElement) EmailHeaderLength() (int, error) {
	if ie.ID != IEIEmailHeader {
		return 0, NewDecodeError("iei", 0, ErrInvalid)
	}
	if err := ie.dataLength(1); err != nil {
		return 0, err
	}
	return int(ie.Data[0]), nil
}

// EmailHeaderLength returns the last IETF RFC 5322 E-Mail Header IE that
// decodes, or zero and false if there is none. IEs with the wrong length
// are skipped. 3GPP TS 23.040 Section 9.2.3.24 lists this IE as not repeatable
// and states: "In the event that IEs determined as not repeatable are
// duplicated, the last occurrence of the IE shall be used."
// Section 9.2.3.24.11 places the header or its fraction at the start of the
// short message in each segment; InformationElement.EmailHeaderLength
// describes the units of the length.
func (udh UserDataHeader) EmailHeaderLength() (int, bool) {
	for i := len(udh) - 1; i >= 0; i-- {
		if n, err := udh[i].EmailHeaderLength(); err == nil {
			return n, true
		}
	}
	return 0, false
}

// Hyperlink is the Hyperlink format element defined in 3GPP TS 23.040
// Section 9.2.3.24.12. Values are retained as received.
type Hyperlink struct {
	// Position is the absolute character position in the entire text of
	// the concatenated message: "the first character is numbered character 1."
	Position int
	// TitleLength counts characters in the hyperlink title.
	TitleLength int
	// URLLength counts characters in the URL. "A space character shall be
	// inserted between the hyperlink title and the URL."
	URLLength int
}

// Valid reports whether Position is 1 to 65535 and both lengths are 0 to
// 255. 3GPP TS 23.040 Section 9.2.3.24.12 defines no other reserved values.
// It does not check the hyperlink against the message text.
func (h Hyperlink) Valid() bool {
	return h.Position >= 1 && h.Position <= 65535 &&
		h.TitleLength >= 0 && h.TitleLength <= 255 && h.URLLength >= 0 && h.URLLength <= 255
}

// Hyperlink decodes the four octets of 3GPP TS 23.040 Section 9.2.3.24.12
// without checking ranges. Position uses the integer representation of
// Section 9.1.2.1, most significant octet first.
// Another IEI returns a DecodeError for iei at offset 0 wrapping ErrInvalid.
// Shorter or longer data returns a DecodeError for ied at offset 2 wrapping
// ErrUnderflow or ErrOverlength, respectively. Every error returns a zero value.
func (ie InformationElement) Hyperlink() (Hyperlink, error) {
	if ie.ID != IEIHyperlink {
		return Hyperlink{}, NewDecodeError("iei", 0, ErrInvalid)
	}
	if err := ie.dataLength(4); err != nil {
		return Hyperlink{}, err
	}
	return Hyperlink{
		Position:    int(binary.BigEndian.Uint16(ie.Data[:2])),
		TitleLength: int(ie.Data[2]),
		URLLength:   int(ie.Data[3]),
	}, nil
}

// Hyperlinks returns every Hyperlink format element that decodes, in order,
// including undefined positions. IEs with the wrong length are skipped;
// if none decode the result is nil. 3GPP TS 23.040 Section 9.2.3.24 lists
// this IE as repeatable and states: "If any reserved values are received
// within the content of any Information Element then that part of the
// Information Element shall be ignored."
func (udh UserDataHeader) Hyperlinks() []Hyperlink {
	var links []Hyperlink
	for _, ie := range udh {
		if h, err := ie.Hyperlink(); err == nil {
			links = append(links, h)
		}
	}
	return links
}

// ReplyAddress decodes the Reply Address Element of 3GPP TS 23.040 Section
// 9.2.3.24.10.1.17 as an address field of Section 9.1.2.5 filling the data.
// Another IEI returns a DecodeError for iei at offset 0 wrapping ErrInvalid.
// Short or surplus data returns a DecodeError for ied wrapping ErrUnderflow
// or ErrOverlength at offset 2 plus the start of the affected parameter
// within the data (zero for this address). An Address.UnmarshalBinary error
// is nested with NewDecodeError("ied", 2+offset, err). Every error returns
// a zero Address.
func (ie InformationElement) ReplyAddress() (Address, error) {
	if ie.ID != IEIReplyAddress {
		return Address{}, NewDecodeError("iei", 0, ErrInvalid)
	}
	r := ieDataReader{data: ie.Data}
	a := r.address()
	if err := r.finish(); err != nil {
		return Address{}, err
	}
	return a, nil
}

// ReplyAddress returns the last Reply Address Element that decodes, or a
// zero Address and false if there is none. Malformed IEs are skipped.
// 3GPP TS 23.040 Section 9.2.3.24.10.1.17 states: "Only one alternate Reply
// Address Element can be integrated in a message." In a concatenated SM it
// occurs only in the first segment. Section 9.2.3.24 states: "In the event
// that IEs determined as not repeatable are duplicated, the last occurrence
// of the IE shall be used." Section 9.2.3.24.10.1.17 states: "When this IE is
// received in a message, replies to this message should take place by default
// using the address specified in this IE instead of the regular message TP-OA."
func (udh UserDataHeader) ReplyAddress() (Address, bool) {
	for i := len(udh) - 1; i >= 0; i-- {
		if a, err := udh[i].ReplyAddress(); err == nil {
			return a, true
		}
	}
	return Address{}, false
}

// ieDataReader reads whole parameters, retaining the first error and the
// last parameter's start so surplus octets can be attributed to it.
type ieDataReader struct {
	data      []byte
	pos, last int
	err       error
}

// take returns the next n octets, or nil after recording an underflow at
// the parameter that starts at the current position.
func (r *ieDataReader) take(n int) []byte {
	if r.err != nil {
		return nil
	}
	r.last = r.pos
	if len(r.data)-r.pos < n {
		r.err = NewDecodeError("ied", 2+r.pos, ErrUnderflow)
		return nil
	}
	b := r.data[r.pos : r.pos+n : r.pos+n]
	r.pos += n
	return b
}

// address decodes the address field of Section 9.1.2.5 that starts at the
// current position.
func (r *ieDataReader) address() Address {
	var a Address
	if r.err != nil {
		return a
	}
	start := r.pos
	if len(r.data)-start < 2 {
		r.take(2)
		return a
	}
	// Let Address report invalid lengths, preserving its nested error.
	if int(r.data[start]) <= maxAddressLength {
		if r.take(2+(int(r.data[start])+1)/2) == nil {
			return a
		}
	}
	_, err := a.UnmarshalBinary(r.data[start:])
	if err != nil {
		r.err = NewDecodeError("ied", 2+start, err)
	}
	return a
}

// finish returns the first error, or an overlength at the last parameter if
// octets follow it.
func (r *ieDataReader) finish() error {
	if r.err != nil {
		return r.err
	}
	if r.pos != len(r.data) {
		return NewDecodeError("ied", 2+r.last, ErrOverlength)
	}
	return nil
}

// EnhancedVoiceMail is the Enhanced Voice Mail Information of 3GPP TS 23.040
// Section 9.2.3.24.13. Values, including reserved bits, are retained as
// received; Valid reports whether they are defined in this release.
type EnhancedVoiceMail struct {
	// DeleteConfirmation is ENHANCED_VOICE_MAIL_PDU_TYPE, bit 0 of octet 1:
	// false selects Notification (.13.1), true Delete Confirmation (.13.2).
	DeleteConfirmation bool
	// Reserved retains RESERVED_FOR_FUTURE_USE, bit 1 of octet 1, in its
	// original position (mask 0x02).
	Reserved int
	// Profile is MULTIPLE_SUBSCRIBER_PROFILE, 1 to 4. Section 9.2.3.24.13.1
	// labels it "Bits 2..3 Octet 1", while .13.2 says "Bits 3..2 Octet 1"
	// and refers to .13.1 for coding: "00 profile ID 1", "10 profile ID 2",
	// "01 profile ID 3", "11 profile ID 4". Reading those codes bit 2 first
	// gives the integer value of bits 3 and 2 plus 1, as for the profile
	// coding in the Special SMS Message Indication.
	Profile int
	// Store is SM_STORAGE, bit 4: store the SM after evaluating its contents.
	Store bool
	// AlmostFull and Full are VM_MAILBOX_ALMOST_FULL and VM_MAILBOX_FULL,
	// bits 5 and 6 of octet 1. The almost-full threshold is system specific.
	AlmostFull, Full bool
	// MailboxExtensionIndicator is VM_MAILBOX_STATUS_EXTENSION_INDICATOR,
	// bit 7 of octet 1. "In this release, this parameter shall be set to 0."
	MailboxExtensionIndicator bool
	// MailboxExtension is VM_MAILBOX_STATUS_EXTENSION_DATA. Its length is
	// VM_MAILBOX_STATUS_EXTENSION_LENGTH, "in the range 0 to 255". It is
	// nil when the indicator is clear, non-nil (possibly empty) when set.
	// It is copied from the IE data.
	MailboxExtension []byte
	// AccessAddress is VM_MAILBOX_ACCESS_ADDRESS, an address field of
	// Section 9.1.2.5, taking precedence over Mailbox Dialing Numbers on (U)SIM.
	AccessAddress Address
	// VoiceMessages is NUMBER_OF_VOICE_MESSAGES, "in the range 0 to 255".
	// "The value 255 shall be taken to mean 255 or greater."
	VoiceMessages int
	// CountReserved retains RESERVED_FOR_FUTURE_USE, bits 5..7 of the octet
	// holding NUMBER_OF_VM_NOTIFICATIONS or NUMBER_OF_VM_DELETES, in their
	// original positions (mask 0xe0).
	CountReserved int
	// Notifications holds NUMBER_OF_VM_NOTIFICATIONS entries, decoded from
	// bits 0..4 of the count octet. Section 9.2.3.24.13.1 says "This parameter
	// has a range 0 to 15." Counts 16 to 31 are retained but are not Valid.
	// It is nil for a zero count or a Delete Confirmation.
	Notifications []VoiceMailNotification
	// Deletes holds NUMBER_OF_VM_DELETES entries. Section 9.2.3.24.13.2
	// labels the count "Bits 0..4" but says it "has a range 0 to 63."
	// Decoding follows the five-bit field; bits 5..7 are CountReserved.
	// Valid uses the stated range 0 to 63 for this slice's length.
	// It is nil for a zero count or a Notification.
	Deletes []VoiceMailDelete
}

// VoiceMailNotification holds the repeated message parameters of 3GPP TS
// 23.040 Section 9.2.3.24.13.1, in their table order.
type VoiceMailNotification struct {
	// ID is VM_MESSAGE_ID, "a range 0 to 65535, modulus 65536".
	ID int
	// Length is VM_MESSAGE_LENGTH, in seconds, "a range 0 to 255".
	// For messages longer than 255 seconds it is set to 255.
	Length int
	// RetentionDays is VM_MESSAGE_RETENTION_DAYS, "a range 0 to 31",
	// measured from TP-SCTS. Longer retention times are represented by 31.
	RetentionDays int
	// Reserved retains RESERVED_FOR_FUTURE_USE bit 5 of the retention
	// octet in its original position (mask 0x20).
	Reserved int
	// Urgent is VM_MESSAGE_PRIORITY_INDICATION, bit 6 of the retention octet.
	Urgent bool
	// ExtensionIndicator is VM_MESSAGE_EXTENSION_INDICATOR, bit 7 of the
	// retention octet. "In this release, this parameter shall be set to 0."
	ExtensionIndicator bool
	// CallingLineIdentity is VM_MESSAGE_CALLING_LINE_IDENTITY, an address
	// field of Section 9.1.2.5. Where CLI is unavailable, "the coding of this
	// parameter shall indicate that there is no address", with its length
	// indicator zero. Type-of-address is still part of the address field.
	CallingLineIdentity Address
	// Extension is VM_MESSAGE_EXTENSION_DATA. Its length is
	// VM_MESSAGE_EXTENSION_LENGTH, "in the range 0 to 255". It is nil
	// without the indicator, and non-nil (possibly empty) and copied from
	// the IE data with it.
	Extension []byte
}

// VoiceMailDelete holds the repeated message parameters of 3GPP TS 23.040
// Section 9.2.3.24.13.2, in their table order.
type VoiceMailDelete struct {
	// ID is VM_MESSAGE_ID. Its range is that of Section 9.2.3.24.13.1:
	// "0 to 65535, modulus 65536".
	ID int
	// Reserved retains RESERVED_FOR_FUTURE_USE bits 0..6 of the octet
	// following ID in their original positions (mask 0x7f).
	Reserved int
	// ExtensionIndicator is VM_MESSAGE_EXTENSION_INDICATOR, bit 7 of the
	// octet following ID. "In this release, this parameter shall be set to 0."
	ExtensionIndicator bool
	// Extension is VM_MESSAGE_EXTENSION_DATA, whose length parameter has
	// "a value in the range 0 to 255". It is nil without the indicator, and
	// non-nil (possibly empty) and copied from the IE data with it.
	Extension []byte
}

// Valid reports whether the values are defined by 3GPP TS 23.040 Sections
// 9.2.3.24.13.1 and .13.2: all reserved bits and extension indicators are
// zero, Profile is 1 to 4, VoiceMessages is 0 to 255, notification count is
// 0 to 15, delete count is 0 to 63, IDs are 0 to 65535, message lengths are
// 0 to 255 seconds and retention is 0 to 31 days. The extension length
// range is 0 to 255, but no extension is defined in this release, so any
// extension indicator or nonempty extension data is not Valid. Only the
// list selected by DeleteConfirmation may contain entries. Address coding
// is handled by Address.UnmarshalBinary; Valid does not validate addresses.
func (v EnhancedVoiceMail) Valid() bool {
	if v.Reserved != 0 || v.CountReserved != 0 || v.Profile < 1 || v.Profile > 4 ||
		v.VoiceMessages < 0 || v.VoiceMessages > 255 ||
		v.MailboxExtensionIndicator || len(v.MailboxExtension) != 0 {
		return false
	}
	if v.DeleteConfirmation {
		if len(v.Notifications) != 0 || len(v.Deletes) > 63 {
			return false
		}
		for _, d := range v.Deletes {
			if d.ID < 0 || d.ID > 65535 || d.Reserved != 0 || d.ExtensionIndicator || len(d.Extension) != 0 {
				return false
			}
		}
		return true
	}
	if len(v.Deletes) != 0 || len(v.Notifications) > 15 {
		return false
	}
	for _, n := range v.Notifications {
		if n.ID < 0 || n.ID > 65535 || n.Length < 0 || n.Length > 255 ||
			n.RetentionDays < 0 || n.RetentionDays > 31 || n.Reserved != 0 ||
			n.ExtensionIndicator || len(n.Extension) != 0 {
			return false
		}
	}
	return true
}

// EnhancedVoiceMail decodes the Notification or Delete Confirmation of
// 3GPP TS 23.040 Sections 9.2.3.24.13.1 and .13.2 without range checks.
// Both tables require parameters "in strict order following the IEDL".
// Conditional extension lengths and data are read whenever their indicator
// is set, even though "In this release, this parameter shall be set to 0."
// Counts and extension lengths are retained as the lengths of their slices.
//
// Another IEI returns a DecodeError for iei at offset 0 wrapping ErrInvalid.
// A short parameter returns a DecodeError for ied wrapping ErrUnderflow at
// offset 2 plus that parameter's start within the data. Surplus octets return
// ErrOverlength at the start of the final parameter, also relative to 2.
// An address rejected by Address.UnmarshalBinary has its error nested with
// NewDecodeError("ied", 2+offset, err), where offset is the address's start.
// Every error returns a zero EnhancedVoiceMail.
func (ie InformationElement) EnhancedVoiceMail() (EnhancedVoiceMail, error) {
	if ie.ID != IEIEnhancedVoiceMail {
		return EnhancedVoiceMail{}, NewDecodeError("iei", 0, ErrInvalid)
	}
	r := ieDataReader{data: ie.Data}
	first := r.octet()
	v := EnhancedVoiceMail{
		DeleteConfirmation:        first&1 != 0,
		Reserved:                  int(first & 2),
		Profile:                   int(first>>2&3) + 1,
		Store:                     first&0x10 != 0,
		AlmostFull:                first&0x20 != 0,
		Full:                      first&0x40 != 0,
		MailboxExtensionIndicator: first&0x80 != 0,
	}
	v.AccessAddress = r.address()
	v.VoiceMessages = int(r.octet())
	count := r.octet()
	v.CountReserved = int(count & 0xe0)
	v.MailboxExtension = r.extension(v.MailboxExtensionIndicator)
	for i := 0; i < int(count&0x1f) && r.err == nil; i++ {
		id := r.uint16()
		if v.DeleteConfirmation {
			flags := r.octet()
			d := VoiceMailDelete{ID: id, Reserved: int(flags & 0x7f), ExtensionIndicator: flags&0x80 != 0}
			d.Extension = r.extension(d.ExtensionIndicator)
			v.Deletes = append(v.Deletes, d)
		} else {
			n := VoiceMailNotification{ID: id, Length: int(r.octet())}
			flags := r.octet()
			n.RetentionDays = int(flags & 0x1f)
			n.Reserved = int(flags & 0x20)
			n.Urgent = flags&0x40 != 0
			n.ExtensionIndicator = flags&0x80 != 0
			n.CallingLineIdentity = r.address()
			n.Extension = r.extension(n.ExtensionIndicator)
			v.Notifications = append(v.Notifications, n)
		}
	}
	if err := r.finish(); err != nil {
		return EnhancedVoiceMail{}, err
	}
	return v, nil
}

// EnhancedVoiceMail returns the last Enhanced Voice Mail Information IE
// that decodes, whether Valid or not, or a zero value and false if none do.
// Malformed IEs are skipped. 3GPP TS 23.040 Section 9.2.3.24 lists this IE
// as not repeatable and states: "In the event that IEs determined as not
// repeatable are duplicated, the last occurrence of the IE shall be used."
// It also states: "If any reserved values are received within the content
// of any Information Element then that part of the Information Element
// shall be ignored." Reserved values therefore remain available to the
// caller, who ignores those parts when applying the notification.
// Section 9.2.3.24.13.1 gives this notification precedence over contradictory
// DCS or Special SMS Message Indication voice-message-waiting indications.
func (udh UserDataHeader) EnhancedVoiceMail() (EnhancedVoiceMail, bool) {
	for i := len(udh) - 1; i >= 0; i-- {
		if v, err := udh[i].EnhancedVoiceMail(); err == nil {
			return v, true
		}
	}
	return EnhancedVoiceMail{}, false
}

// octet returns the next octet, or 0 after an error.
func (r *ieDataReader) octet() byte {
	if b := r.take(1); b != nil {
		return b[0]
	}
	return 0
}

// uint16 returns the next two octets as a big-endian integer, or 0 after an
// error.
func (r *ieDataReader) uint16() int {
	if b := r.take(2); b != nil {
		return int(binary.BigEndian.Uint16(b))
	}
	return 0
}

// extension returns a copy of the extension data that follows its length
// octet when indicator is set, and nil otherwise.
func (r *ieDataReader) extension(indicator bool) []byte {
	if !indicator {
		return nil
	}
	length := int(r.octet())
	return slices.Clone(r.take(length))
}
