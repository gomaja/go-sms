// SPDX-License-Identifier: MIT

package tpdu

import "encoding/binary"

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
