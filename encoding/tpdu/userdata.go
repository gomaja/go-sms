// SPDX-License-Identifier: MIT

package tpdu

import (
	"encoding/binary"
	"errors"
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

func (ie InformationElement) marshalledLen() int {
	return 2 + len(ie.Data)
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
			return nil, EncodeError("ied", ErrOverlength)
		}
	}
	udhl := udh.UDHL()
	if udhl > maxLengthOctet {
		return nil, EncodeError("udhl", ErrOverlength)
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
		if udhl < ri+2 || udhl < ri+2+int(src[ri+1]) {
			// too few or too many octets in the final IE
			*udh = UserDataHeader{}
			return udhl, nil
		}
		var ie InformationElement
		ie.ID = src[ri]
		ri++
		iedl := int(src[ri])
		ri++
		ie.Data = append([]byte(nil), src[ri:ri+iedl]...)
		ri += iedl
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
	// concatenated short message, from 1 to Total.
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
func (udh UserDataHeader) ConcatInfo() (ci ConcatInfo, ok bool) {
	for i := len(udh) - 1; i >= 0; i-- {
		if ci, ok = udh[i].concatInfo(); ok {
			return ci, ok
		}
	}
	return ConcatInfo{}, false
}

// concatInfo returns the segmentation info carried by the IE, if it is a valid
// concatenation IE.
func (ie InformationElement) concatInfo() (ConcatInfo, bool) {
	var ci ConcatInfo
	switch {
	case ie.ID == IEIConcat8Bit && len(ie.Data) == 3:
		ci.Ref = int(ie.Data[0])
		ci.Total = int(ie.Data[1])
		ci.Seqno = int(ie.Data[2])
	case ie.ID == IEIConcat16Bit && len(ie.Data) == 4:
		ci.Ref = int(binary.BigEndian.Uint16(ie.Data[0:2]))
		ci.Ref16Bit = true
		ci.Total = int(ie.Data[2])
		ci.Seqno = int(ie.Data[3])
	default:
		return ConcatInfo{}, false
	}
	if ci.Total == 0 || ci.Seqno == 0 || ci.Seqno > ci.Total {
		return ConcatInfo{}, false
	}
	return ci, true
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

// DecodeUserData converts TPDU UD into the corresponding UTF8 message.
//
// The UD is expected to be unpacked, as stored in TPDU UD. If the UD is GSM7
// encoded then it is translated to UTF8 with the default character set, or
// with the character set specified in the UDH, assuming the corresponding
// language has been registered with the UDDecoder. If the UDH specifies a
// character set that has not been registered then the translation will fall
// back to the default character set.
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
		if ie, ok := udh.IE(IEINationalLanguageLockingShift); ok {
			if len(ie.Data) >= 1 {
				nli := int(ie.Data[0])
				if _, ok := cfg.locking[nli]; ok {
					options = append(options, gsm7.WithCharset(nli))
				}
			}
		}
		if ie, ok := udh.IE(IEINationalLanguageSingleShift); ok {
			if len(ie.Data) >= 1 {
				nli := int(ie.Data[0])
				if _, ok := cfg.shift[nli]; ok {
					options = append(options, gsm7.WithExtCharset(nli))
				}
			}
		}
		return gsm7.Decode(ud, options...)
	}
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
	e.locking = make([]int, charset.Size)
	for nli := charset.Start; nli < charset.End; nli++ {
		e.locking[nli-1] = nli
	}
	e.shift = e.locking
	return e
}

// WithAllCharsets makes all possible character sets available to encode or
// decode.
//
// This is equivalent to calling WithCharset with all possible
// NationalLanguageIdentifiers, in increasing order.
var WithAllCharsets = AllCharsetsOption{}

// WithCharset sets the set of character sets available to encode or decode.
//
// These are in addition to the default character set.
func WithCharset(nli ...int) CharsetOption {
	return CharsetOption{nli}
}

// WithLockingCharset sets the set of locking character sets available to
// encode or decode.
//
// These are in addition to the default character set.
func WithLockingCharset(nli ...int) LockingCharsetOption {
	return LockingCharsetOption{nli}
}

// WithShiftCharset sets the set of shift character sets available to
// encode or decode.
//
// These are in addition to the default character set.
func WithShiftCharset(nli ...int) ShiftCharsetOption {
	return ShiftCharsetOption{nli}
}

// Information Element Identifiers interpreted by this package.
//
// 3GPP TS 23.040 Section 9.2.3.24 lists the IEI values in hex.
const (
	// IEIConcat8Bit identifies the Concatenated short messages, 8-bit
	// reference number IE, as defined in 3GPP TS 23.040 Section 9.2.3.24.1.
	IEIConcat8Bit byte = 0x00

	// IEIConcat16Bit identifies the Concatenated short messages, 16-bit
	// reference number IE, as defined in 3GPP TS 23.040 Section 9.2.3.24.8.
	IEIConcat16Bit byte = 0x08

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
// tries combinations of supported language character sets, in the order they
// were added to the UDEncoder.
//
// This is not optimal as it performs language selection on the whole message,
// rather than determining the best for each segment in turn. (which is totally
// allowed as stated in 3GPP TS 23.040 9.2.3.24.15 + 16), but this may be a
// safer approach - to allow for the decoder being non-compliant, and the
// benefit of per-segment language encoding is minimal. In most cases there is
// no benefit at all.
//
// Failing GSM7 conversion it falls back to UCS2/UTF16.
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

	// fallback to ucs-2
	enc = ucs2.Encode([]rune(string(msg)))
	return enc, nil, AlphaUCS2, nil
}

// ErrInvalidUTF8 indicates that a message to be encoded is not valid UTF8.
var ErrInvalidUTF8 = errors.New("invalid UTF8")
