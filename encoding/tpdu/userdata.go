// SPDX-License-Identifier: MIT

package tpdu

import (
	"encoding/binary"
	"errors"
	"slices"
	"unicode/utf8"

	"github.com/gomaja/go-sms/encoding/gsm7"
	"github.com/gomaja/go-sms/encoding/gsm7/charset"
	"github.com/gomaja/go-sms/encoding/ucs2"
)

// UserData represents the User Data field as defined in 3GPP TS 23.040 Section
// 9.2.3.24.
//
// The UserData is comprised of an optional User Data Header and a short
// message field.
type UserData []byte

// UserDataHeader represents the header section of the User Data as defined in
// 3GPP TS 23.040 Section 9.2.3.24.
type UserDataHeader []InformationElement

// InformationElement represents one of the information elements contained in
// the User Data Header.
type InformationElement struct {
	ID   byte
	Data []byte
}

// UnmarshalBinary decodes one information element from the start of src: its
// IEI, its IEDL and IEDL octets of data, as laid out by 3GPP TS 23.040
// Section 9.2.3.24, in which "The "Length-of-Information-Element" fields
// shall be the integer representation of the number of octets within its
// associated "Information-Element-Data" field which follows and shall not
// include itself in its count value."
//
// It returns the number of octets decoded, 2+IEDL, and does not read the
// octets after them. The data is copied from src, and is nil if IEDL is 0.
//
// If src ends before the element does, a DecodeError for the iei, iedl or
// ied wrapping ErrUnderflow is returned with a count of 0, and the
// information element is not changed.
func (ie *InformationElement) UnmarshalBinary(src []byte) (int, error) {
	if len(src) < 1 {
		return 0, NewDecodeError("iei", 0, ErrUnderflow)
	}
	if len(src) < 2 {
		return 0, NewDecodeError("iedl", 1, ErrUnderflow)
	}
	n := 2 + int(src[1])
	if len(src) < n {
		return 0, NewDecodeError("ied", 2, ErrUnderflow)
	}
	*ie = InformationElement{ID: src[0], Data: append([]byte(nil), src[2:n]...)}
	return n, nil
}

func (ie InformationElement) marshalledLen() int {
	return 2 + len(ie.Data)
}

// clone returns a copy of the UDH that shares no memory with it. A nil UDH,
// which is not marshalled, stays nil, and an empty one, which is marshalled
// as a TP-UDHL of 0, stays empty.
func (udh UserDataHeader) clone() UserDataHeader {
	if udh == nil {
		return nil
	}
	c := make(UserDataHeader, len(udh))
	for i, ie := range udh {
		c[i] = InformationElement{ID: ie.ID, Data: slices.Clone(ie.Data)}
	}
	return c
}

// UDHL returns the encoded length of the UDH, not including the UDHL itself.
func (udh UserDataHeader) UDHL() int {
	udhl := 0
	for _, ie := range udh {
		udhl += ie.marshalledLen()
	}
	return udhl
}

// maxLengthOctet is the largest length a single length octet can encode.
const maxLengthOctet = 0xff

// MarshalBinary marshals the User Data Header, including the UDHL, into
// binary.
//
// A nil UDH indicates that no header is present and marshals to nil. An empty,
// but not nil, UDH is a header without IEs and marshals to the single UDHL
// octet 0.
//
// An error is returned if the data of an IE, or the whole header, is too long
// for its length octet, as defined in 3GPP TS 23.040 Section 9.2.3.24.
func (udh UserDataHeader) MarshalBinary() ([]byte, error) {
	if udh == nil {
		return nil, nil
	}
	for _, ie := range udh {
		if len(ie.Data) > maxLengthOctet {
			return nil, NewEncodeError("ied", ErrOverlength)
		}
	}
	udhl := udh.UDHL()
	if udhl > maxLengthOctet {
		return nil, NewEncodeError("udhl", ErrOverlength)
	}
	b := make([]byte, 0, udhl+1)
	b = append(b, byte(udhl))
	for _, ie := range udh {
		b = append(b, ie.ID, byte(len(ie.Data)))
		b = append(b, ie.Data...)
	}
	return b, nil
}

// UnmarshalBinary reads the InformationElements from the binary User Data
// Header.
//
// The src contains the complete UDH, including the UDHL and all IEs, and may
// be followed by the short message.
// The function returns the number of bytes read from src, which is the UDHL
// plus one unless an error is returned, and any error detected while
// unmarshalling.
//
// A UDHL of 0 results in an empty, but not nil, UDH.
//
// If the IEs do not exactly fill the UDHL then the whole UDH is ignored, as
// required by 3GPP TS 23.040 Section 9.2.3.24: "If the length of the User Data
// Header is such that there are too few or too many octets in the final
// Information Element then the whole User Data Header shall be ignored."
// The UDH is then left empty, although the UDHL octets are still read, so an
// empty UDH with a returned length greater than one indicates an ignored
// header.
func (udh *UserDataHeader) UnmarshalBinary(src []byte) (int, error) {
	if len(src) < 1 {
		return 0, NewDecodeError("udhl", 0, ErrUnderflow)
	}
	udhl := int(src[0])
	udhl++ // so it includes itself
	ri := 1
	if len(src) < udhl {
		return ri, NewDecodeError("ie", ri, ErrUnderflow)
	}
	ies := UserDataHeader{}
	for ri < udhl {
		var ie InformationElement
		n, err := ie.UnmarshalBinary(src[ri:udhl])
		if err != nil {
			// too few or too many octets in the final IE
			*udh = UserDataHeader{}
			return udhl, nil
		}
		ri += n
		ies = append(ies, ie)
	}
	*udh = ies
	return udhl, nil
}

// IE returns the last instance of the IE with the given id in the UDH.
//
// If no such IE is found then the function returns false.
func (udh UserDataHeader) IE(id byte) (InformationElement, bool) {
	for i := len(udh) - 1; i >= 0; i-- {
		if udh[i].ID == id {
			return udh[i], true
		}
	}
	return InformationElement{}, false
}

// IEs returns all instances of the IEs with the given id in the UDH.
func (udh UserDataHeader) IEs(id byte) []InformationElement {
	ies := []InformationElement(nil)
	for _, ie := range udh {
		if ie.ID == id {
			ies = append(ies, ie)
		}
	}
	return ies
}

// ConcatInfo is the segmentation information carried by a Concatenated short
// messages IE, as defined in 3GPP TS 23.040 Sections 9.2.3.24.1 (8-bit
// reference number) and 9.2.3.24.8 (16-bit reference number).
//
// The values may be out of range, as InformationElement.ConcatInfo does not
// check them; Valid reports whether they are in range.
type ConcatInfo struct {
	// Ref is the concatenated short message reference number.
	Ref int

	// Ref16Bit indicates that Ref is a 16-bit reference number, from an IE
	// with IEIConcat16Bit, rather than an 8-bit one, from an IE with
	// IEIConcat8Bit.
	//
	// An 8-bit and a 16-bit reference number with the same value identify
	// different concatenated short messages.
	Ref16Bit bool

	// Total is the number of short messages in the concatenated short
	// message.
	Total int

	// Seqno is the sequence number of the short message within the
	// concatenated short message, from 1 to Total when Valid returns true.
	Seqno int
}

// ConcatInfo extracts the segmentation info contained in the provided User
// Data Header.
//
// If the UDH contains no valid concatenation IE then ok is false and a zero
// ConcatInfo is returned.
//
// A concatenation IE with a total of zero, or a sequence number of zero or
// greater than the total, is ignored, as required by 3GPP TS 23.040 Sections
// 9.2.3.24.1 and 9.2.3.24.8, as is one with the wrong length. Ignoring an IE
// means skipping over it, so of the remaining 8-bit and 16-bit concatenation
// IEs, which are mutually exclusive, the last occurring one is used, as
// required by 3GPP TS 23.040 Section 9.2.3.24.
//
// InformationElement.ConcatInfo decodes a concatenation IE whatever its
// values, so a caller can tell a header whose concatenation IE is ignored
// from one without any.
func (udh UserDataHeader) ConcatInfo() (ci ConcatInfo, ok bool) {
	for i := len(udh) - 1; i >= 0; i-- {
		if ci, err := udh[i].ConcatInfo(); err == nil && ci.Valid() {
			return ci, true
		}
	}
	return ConcatInfo{}, false
}

// ConcatInfo decodes the values carried by a concatenation IE, as defined in
// 3GPP TS 23.040 Sections 9.2.3.24.1 (8-bit reference number) and 9.2.3.24.8
// (16-bit reference number), without checking their ranges, so unlike
// UserDataHeader.ConcatInfo it also returns those of an IE that a receiving
// entity ignores. Valid reports whether a receiving entity uses them.
//
// An IE with another IEI returns a DecodeError for the iei wrapping
// ErrInvalid. Data other than the 3 octets of the 8-bit reference IE, or the
// 4 of the 16-bit one, returns a DecodeError for the ied wrapping
// ErrUnderflow if it is shorter, or ErrOverlength if it is longer. The
// ConcatInfo is then zero.
func (ie InformationElement) ConcatInfo() (ConcatInfo, error) {
	var length int
	switch ie.ID {
	case IEIConcat8Bit:
		length = 3
	case IEIConcat16Bit:
		length = 4
	default:
		return ConcatInfo{}, NewDecodeError("iei", 0, ErrInvalid)
	}
	if err := ie.dataLength(length); err != nil {
		return ConcatInfo{}, err
	}
	var ci ConcatInfo
	if ie.ID == IEIConcat8Bit {
		ci.Ref = int(ie.Data[0])
		ci.Total = int(ie.Data[1])
		ci.Seqno = int(ie.Data[2])
	} else {
		ci.Ref = int(binary.BigEndian.Uint16(ie.Data[0:2]))
		ci.Ref16Bit = true
		ci.Total = int(ie.Data[2])
		ci.Seqno = int(ie.Data[3])
	}
	return ci, nil
}

// Valid reports whether every value fits in a concatenation IE and can be
// used by a receiving entity. Ref must fit in 8 bits, or 16 bits when Ref16Bit
// is true; Total must be from 1 to 255 and Seqno from 1 to Total.
//
// 3GPP TS 23.040 Section 9.2.3.24.1 states for the total: "If the value is
// zero then the receiving entity shall ignore the whole Information Element."
// For the sequence number it states: "If the value is zero or the value is
// greater than the value in octet 2 then the receiving entity shall ignore
// the whole Information Element." Section 9.2.3.24.8 applies the same rules
// to the 16-bit reference IE, whose total is in octet 3.
func (ci ConcatInfo) Valid() bool {
	maxRef := 255
	if ci.Ref16Bit {
		maxRef = 65535
	}
	return ci.Ref >= 0 && ci.Ref <= maxRef &&
		ci.Total >= 1 && ci.Total <= maxSegments &&
		ci.Seqno >= 1 && ci.Seqno <= ci.Total
}

type udDecodeConfig struct {
	locking map[int]bool
	shift   map[int]bool
}

// UDDecodeOption provides behavioural modifiers for DecodeUserData,
// specifically the character sets available to decode GSM7.
type UDDecodeOption interface {
	applyDecodeOption(udDecodeConfig) udDecodeConfig
}

// DecodeUserData returns the message in TPDU UD. UCS2 is decoded with
// ucs2.Decode and returned as UTF-8, with that function's surrogate handling
// and errors. 8-bit data is returned unchanged, sharing the input slice:
// 3GPP TS 23.038 Section 6.2.2 states "8 bit data is user defined".
// GSM7, and any other alphabet value, is decoded as described below.
//
// The UD is expected to be unpacked, as stored in TPDU UD. If the UD is GSM7
// encoded then it is translated to UTF8 with the default character set, or
// with the character set specified in the UDH, assuming the corresponding
// language has been enabled by the options. If the UDH specifies a
// character set that has not been enabled then that IE is ignored, as
// required by 3GPP TS 23.038 Section 6.2.1.2.5, and the translation uses the
// character set of an earlier IE of the same type, or failing that falls back
// to the default character set.
func DecodeUserData(ud UserData, udh UserDataHeader, alpha Alphabet, options ...UDDecodeOption) ([]byte, error) {
	switch alpha {
	case AlphaUCS2:
		m, err := ucs2.Decode(ud)
		return []byte(string(m)), err
	case Alpha8Bit:
		return ud, nil
	case Alpha7Bit:
		fallthrough
	default:
		cfg := udDecodeConfig{locking: map[int]bool{}, shift: map[int]bool{}}
		for _, option := range options {
			cfg = option.applyDecodeOption(cfg)
		}
		options := []gsm7.DecoderOption{}
		if nli, ok := udh.nationalLanguage(IEINationalLanguageLockingShift, cfg.locking); ok {
			options = append(options, gsm7.WithCharset(nli))
		}
		if nli, ok := udh.nationalLanguage(IEINationalLanguageSingleShift, cfg.shift); ok {
			options = append(options, gsm7.WithExtCharset(nli))
		}
		return gsm7.Decode(ud, options...)
	}
}

// nationalLanguage returns the NLI of the last national language IE with the
// given id whose NLI identifies a language and is in the supported set.
//
// An IE indicating "a reserved value or a value that is not supported by the
// receiving entity" is ignored, as per 3GPP TS 23.038 Section 6.2.1.2.5, i.e.
// skipped over, as per 3GPP TS 23.040 Sections 9.2.3.24.15 and 9.2.3.24.16,
// and of the remaining IEs "the last occurrence of the IE" is used. NLI 0 is
// reserved (3GPP TS 23.038 Table 6.2.1.2.4.1), so it is ignored even if
// charset.Default is in the supported set.
func (udh UserDataHeader) nationalLanguage(id byte, supported map[int]bool) (int, bool) {
	for i := len(udh) - 1; i >= 0; i-- {
		ie := udh[i]
		if ie.ID != id || len(ie.Data) < 1 {
			continue
		}
		nli := int(ie.Data[0])
		if nli >= charset.Start && nli < charset.End && supported[nli] {
			return nli, true
		}
	}
	return 0, false
}

type udEncodeConfig struct {
	locking []int // locking charsets in order
	shift   []int // shift charsets in order
}

// UDEncodeOption provides behavioural modifiers for EncodeUserData,
// specifically the locking and shift character sets available, in addition to
// the default character set.
type UDEncodeOption interface {
	applyEncodeOption(udEncodeConfig) udEncodeConfig
}

// CharsetOption adds the locking and shift character sets available for
// encoding and decoding.
//
// These are in addition to the default character set.
type CharsetOption struct {
	nli []int
}

func (o CharsetOption) applyDecodeOption(d udDecodeConfig) udDecodeConfig {
	for _, n := range o.nli {
		d.locking[n] = true
		d.shift[n] = true
	}
	return d
}

func (o CharsetOption) applyEncodeOption(e udEncodeConfig) udEncodeConfig {
	e.locking = append(e.locking, o.nli...)
	e.shift = append(e.shift, o.nli...)
	return e
}

// LockingCharsetOption adds to the locking character sets available for
// encoding and decoding.
//
// These are in addition to the default character set.
type LockingCharsetOption struct {
	nli []int
}

func (o LockingCharsetOption) applyDecodeOption(d udDecodeConfig) udDecodeConfig {
	for _, n := range o.nli {
		d.locking[n] = true
	}
	return d
}

func (o LockingCharsetOption) applyEncodeOption(e udEncodeConfig) udEncodeConfig {
	e.locking = append(e.locking, o.nli...)
	return e
}

// ShiftCharsetOption adds the shift character sets available for encoding
// and decoding.
//
// These are in addition to the default character set.
type ShiftCharsetOption struct {
	nli []int
}

func (o ShiftCharsetOption) applyDecodeOption(d udDecodeConfig) udDecodeConfig {
	for _, n := range o.nli {
		d.shift[n] = true
	}
	return d
}

func (o ShiftCharsetOption) applyEncodeOption(e udEncodeConfig) udEncodeConfig {
	e.shift = append(e.shift, o.nli...)
	return e
}

// AllCharsetsOption specifies that all character sets are available for
// encoding and decoding.
type AllCharsetsOption struct{}

func (o AllCharsetsOption) applyDecodeOption(d udDecodeConfig) udDecodeConfig {
	for nli := charset.Start; nli < charset.End; nli++ {
		d.locking[nli] = true
		d.shift[nli] = true
	}
	return d
}

func (o AllCharsetsOption) applyEncodeOption(e udEncodeConfig) udEncodeConfig {
	for nli := charset.Start; nli < charset.End; nli++ {
		e.locking = append(e.locking, nli)
		e.shift = append(e.shift, nli)
	}
	return e
}

// WithAllCharsets makes all possible character sets available to encode or
// decode.
//
// This is equivalent to calling WithCharset with all possible
// NationalLanguageIdentifiers, in increasing order, so in each pass of
// EncodeUserData the tables of earlier options are tried first.
var WithAllCharsets = AllCharsetsOption{}

// WithDefaultCharset makes only the default character set available to
// encode or decode, removing national language tables enabled by earlier
// options. Later charset options add tables again.
//
// A national language IE is then unsupported and ignored, as required by
// 3GPP TS 23.038 Section 6.2.1.2.5: "the receiving entity shall ignore the IE".
var WithDefaultCharset = DefaultCharsetOption{}

// DefaultCharsetOption removes the national language tables available for
// encoding and decoding. Later charset options add tables again.
type DefaultCharsetOption struct{}

func (o DefaultCharsetOption) applyEncodeOption(e udEncodeConfig) udEncodeConfig {
	e.locking = nil
	e.shift = nil
	return e
}

// applyDecodeOption replaces the maps rather than clearing them, as they may
// be shared with the config an earlier option returned.
func (o DefaultCharsetOption) applyDecodeOption(d udDecodeConfig) udDecodeConfig {
	d.locking = map[int]bool{}
	d.shift = map[int]bool{}
	return d
}

// WithCharset adds character sets available to encode or decode.
//
// These are in addition to the default character set. The identifiers are
// copied, so the caller may go on to change its slice.
func WithCharset(nli ...int) CharsetOption {
	return CharsetOption{slices.Clone(nli)}
}

// WithLockingCharset adds locking character sets available to
// encode or decode.
//
// These are in addition to the default character set. The identifiers are
// copied, so the caller may go on to change its slice.
func WithLockingCharset(nli ...int) LockingCharsetOption {
	return LockingCharsetOption{slices.Clone(nli)}
}

// WithShiftCharset adds shift character sets available to
// encode or decode.
//
// These are in addition to the default character set. The identifiers are
// copied, so the caller may go on to change its slice.
func WithShiftCharset(nli ...int) ShiftCharsetOption {
	return ShiftCharsetOption{slices.Clone(nli)}
}

// Information Element Identifiers interpreted by this package.
//
// 3GPP TS 23.040 Section 9.2.3.24 lists the IEI values in hex.
const (
	// IEIConcat8Bit identifies the Concatenated short messages, 8-bit
	// reference number IE, as defined in 3GPP TS 23.040 Section 9.2.3.24.1.
	IEIConcat8Bit byte = 0x00

	// IEISpecialMessageIndication identifies the Special SMS Message Indication
	// IE, as defined in 3GPP TS 23.040 Section 9.2.3.24.2.
	IEISpecialMessageIndication byte = 0x01

	// IEIPort8Bit identifies the Application port addressing scheme, 8 bit
	// address IE, as defined in 3GPP TS 23.040 Section 9.2.3.24.3.
	IEIPort8Bit byte = 0x04

	// IEIPort16Bit identifies the Application port addressing scheme, 16 bit
	// address IE, as defined in 3GPP TS 23.040 Section 9.2.3.24.4.
	IEIPort16Bit byte = 0x05

	// IEISMSCControlParameters identifies the SMSC Control Parameters IE,
	// as defined in 3GPP TS 23.040 Section 9.2.3.24.5.
	IEISMSCControlParameters byte = 0x06

	// IEIUDHSource identifies the UDH Source Indicator IE, as defined in
	// 3GPP TS 23.040 Section 9.2.3.24.6.
	IEIUDHSource byte = 0x07

	// IEIConcat16Bit identifies the Concatenated short messages, 16-bit
	// reference number IE, as defined in 3GPP TS 23.040 Section 9.2.3.24.8.
	IEIConcat16Bit byte = 0x08

	// IEIEmailHeader identifies the IETF RFC 5322 E-Mail Header IE, as
	// defined in 3GPP TS 23.040 Section 9.2.3.24.11.
	IEIEmailHeader byte = 0x20

	// IEIHyperlink identifies the Hyperlink format element, as defined in
	// 3GPP TS 23.040 Section 9.2.3.24.12.
	IEIHyperlink byte = 0x21

	// IEIReplyAddress identifies the Reply Address Element, as defined in
	// 3GPP TS 23.040 Section 9.2.3.24.10.1.17.
	IEIReplyAddress byte = 0x22

	// IEIEnhancedVoiceMail identifies the Enhanced Voice Mail Information
	// IE, as defined in 3GPP TS 23.040 Section 9.2.3.24.13.
	IEIEnhancedVoiceMail byte = 0x23

	// IEINationalLanguageSingleShift identifies the National Language Single
	// Shift IE, as defined in 3GPP TS 23.040 Section 9.2.3.24.15.
	IEINationalLanguageSingleShift byte = 0x24

	// IEINationalLanguageLockingShift identifies the National Language
	// Locking Shift IE, as defined in 3GPP TS 23.040 Section 9.2.3.24.16.
	IEINationalLanguageLockingShift byte = 0x25
)

// EncodeUserData converts a UTF8 message into corresponding TPDU User Data.
//
// Note that the UD size is not limited to the size available in a single TPDU,
// and so may need to be segmented into several concatenated messages. Encode
// attempts to pick the most compact alphabet for the given message. It assumes
// GSM7 is the most compact, and, if the default character set is insufficient,
// tries the national language tables the options make available, in three
// passes: each locking shift table, with the default extension table, then
// each single shift table, with the default alphabet, then each table enabled
// both as a locking shift and as a single shift table, as both. Each pass
// takes the tables in the order the options added them, the third in that of
// the locking shift tables, and the first that codes the whole message is
// used.
//
// This is not optimal as it performs language selection on the whole message,
// rather than determining the best for each segment in turn. (which is totally
// allowed as stated in 3GPP TS 23.040 9.2.3.24.15 + 16), but this may be a
// safer approach - to allow for the decoder being non-compliant, and the
// benefit of per-segment language encoding is minimal. In most cases there is
// no benefit at all.
//
// Failing GSM7 conversion it falls back to UCS2, coded as UTF-16 by
// ucs2.EncodeUTF16, so a character above U+FFFF, such as an emoji, is sent as
// a surrogate pair, as handsets send it. That is an extension beyond the UCS2
// alphabet of 3GPP TS 23.038 Section 6.2.3, which has 16 bits per character.
//
// ErrInvalidUTF8 is returned if the message is not valid UTF8, rather than
// replacing the invalid octets with U+FFFD.
func EncodeUserData(msg []byte, options ...UDEncodeOption) (UserData, UserDataHeader, Alphabet, error) {
	if !utf8.Valid(msg) {
		return nil, nil, Alpha7Bit, ErrInvalidUTF8
	}
	enc, err := gsm7.Encode([]byte(msg)) // default charset
	if err == nil {
		return enc, nil, Alpha7Bit, nil
	}
	cfg := udEncodeConfig{}
	for _, option := range options {
		cfg = option.applyEncodeOption(cfg)
	}
	// try locking tables with default shift
	for _, nli := range cfg.locking {
		enc, err = gsm7.Encode(msg, gsm7.WithCharset(nli))
		if err == nil {
			return enc, UserDataHeader{
					InformationElement{ID: IEINationalLanguageLockingShift, Data: []byte{byte(nli)}},
				},
				Alpha7Bit, nil
		}
	}
	// try default with language shift tables
	for _, nli := range cfg.shift {
		enc, err = gsm7.Encode(msg, gsm7.WithExtCharset(nli))
		if err == nil {
			return enc, UserDataHeader{
					InformationElement{ID: IEINationalLanguageSingleShift, Data: []byte{byte(nli)}},
				},
				Alpha7Bit, nil
		}
	}
	// try combination of locking AND shift for same charset
	for _, nli := range cfg.locking {
		for _, snli := range cfg.shift {
			if nli != snli {
				continue
			}
			enc, err = gsm7.Encode(msg, gsm7.WithCharset(nli), gsm7.WithExtCharset(nli))
			if err == nil {
				return enc, UserDataHeader{
						InformationElement{ID: IEINationalLanguageLockingShift, Data: []byte{byte(nli)}},
						InformationElement{ID: IEINationalLanguageSingleShift, Data: []byte{byte(nli)}},
					},
					Alpha7Bit, nil
			}
		}
	}
	// could also try other combos of locking AND shift, but unlikely to help??...

	// fallback to UCS2, as UTF-16, so a character above U+FFFF is coded as
	// a surrogate pair. As msg is valid UTF-8 its runes are all Unicode
	// scalar values, which EncodeUTF16 always codes, but should it fail its
	// error is returned rather than dropped.
	enc, err = ucs2.EncodeUTF16([]rune(string(msg)))
	if err != nil {
		return nil, nil, Alpha7Bit, err
	}
	return enc, nil, AlphaUCS2, nil
}

// ErrInvalidUTF8 indicates that a message, or an alphanumeric address, to be
// encoded is not valid UTF-8.
var ErrInvalidUTF8 = errors.New("invalid UTF8")
