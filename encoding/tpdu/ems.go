// SPDX-License-Identifier: MIT

package tpdu

import (
	"encoding/binary"
	"slices"
)

// TextFormatting is the text format of 3GPP TS 23.040 Section
// 9.2.3.24.10.1.1. Values are retained as received; Valid checks defined
// values, without checking positions against a particular message.
//
// Callers walk UserDataHeader in order to interpret EMS elements: most are
// repeatable and their order across element types matters. Section
// 9.2.3.24.10.1.1 states: "Conflicting overlapping text formatting
// instructions shall be resolved by applying the formatting instructions
// in their sequential order." Section 9.2.3.24 lists Compression Control
// and Extended Object Data Request Command as not repeatable. There are no
// EMS convenience methods on UserDataHeader.
type TextFormatting struct {
	// Start counts characters before the formatting starts (octet 1).
	Start int
	// Length is octet 2. Section 9.2.3.24.10.1.1 states: "A text formatting
	// length value of 0 indicates that the text format shall be used as a
	// default text format for the current SM." This default applies to all
	// text in a concatenated SM unless temporarily overridden by a non-zero
	// length. A later zero-length format redefines it for subsequent text.
	Length int
	// Alignment is bits 0..1 of octet 3.
	Alignment TextAlignment
	// FontSize is bits 2..3 of octet 3; value 3 is reserved.
	FontSize TextFontSize
	// Bold, Italic, Underlined and Strikethrough are bits 4, 5, 6 and 7.
	Bold, Italic, Underlined, Strikethrough bool
	// HasColour reports the presence of optional octet 4.
	HasColour bool
	// Foreground and Background are the low and high nibbles of octet 4.
	// Section 9.2.3.24.10.1.1 defines all sixteen colour values.
	Foreground, Background int
}

// TextAlignment is the alignment in 3GPP TS 23.040 Section 9.2.3.24.10.1.1.
type TextAlignment int

const (
	// TextAlignLeft selects left alignment.
	TextAlignLeft TextAlignment = 0
	// TextAlignCenter selects centered alignment.
	TextAlignCenter TextAlignment = 1
	// TextAlignRight selects right alignment.
	TextAlignRight TextAlignment = 2
	// TextAlignLanguage selects language-dependent alignment (the default).
	TextAlignLanguage TextAlignment = 3
)

// TextFontSize is the font size in 3GPP TS 23.040 Section 9.2.3.24.10.1.1.
type TextFontSize int

const (
	// TextFontNormal selects normal size (the default).
	TextFontNormal TextFontSize = 0
	// TextFontLarge selects large size.
	TextFontLarge TextFontSize = 1
	// TextFontSmall selects small size.
	TextFontSmall TextFontSize = 2
)

// Valid reports whether the fields fit their representations and the font
// size is not reserved, per 3GPP TS 23.040 Section 9.2.3.24.10.1.1.
func (f TextFormatting) Valid() bool {
	return f.Start >= 0 && f.Start <= 255 && f.Length >= 0 && f.Length <= 255 &&
		f.Alignment >= TextAlignLeft && f.Alignment <= TextAlignLanguage &&
		f.FontSize >= TextFontNormal && f.FontSize <= TextFontSmall &&
		(!f.HasColour || f.Foreground >= 0 && f.Foreground <= 15 && f.Background >= 0 && f.Background <= 15)
}

// TextFormatting decodes Section 9.2.3.24.10.1.1 of 3GPP TS 23.040 without
// range checks. Another IEI returns a DecodeError for iei at offset 0
// wrapping ErrInvalid. Data shorter than 3 or longer than 4 octets returns
// a DecodeError for ied at offset 2 wrapping ErrUnderflow or ErrOverlength.
// Every error returns a zero TextFormatting.
func (ie InformationElement) TextFormatting() (TextFormatting, error) {
	if ie.ID != IEITextFormatting {
		return TextFormatting{}, NewDecodeError("iei", 0, ErrInvalid)
	}
	length := 3
	if len(ie.Data) >= 4 {
		length = 4
	}
	if err := ie.dataLength(length); err != nil {
		return TextFormatting{}, err
	}
	mode := ie.Data[2]
	f := TextFormatting{Start: int(ie.Data[0]), Length: int(ie.Data[1]),
		Alignment: TextAlignment(mode & 3), FontSize: TextFontSize(mode >> 2 & 3),
		Bold: mode&0x10 != 0, Italic: mode&0x20 != 0, Underlined: mode&0x40 != 0, Strikethrough: mode&0x80 != 0,
		HasColour: len(ie.Data) == 4}
	if f.HasColour {
		f.Foreground = int(ie.Data[3] & 15)
		f.Background = int(ie.Data[3] >> 4)
	}
	return f, nil
}

// PredefinedObject is a predefined sound or animation, as defined in
// 3GPP TS 23.040 Sections 9.2.3.24.10.1.2 and 9.2.3.24.10.1.4.
type PredefinedObject struct {
	// ID is IEIPredefinedSound or IEIPredefinedAnimation.
	ID byte
	// Position is octet 1, counting characters before the object is played.
	Position int
	// Number is octet 2, the sound or animation number. Section
	// 9.2.3.24.10.3 defines sounds 0..9 and animations 0..14.
	Number int
}

// Valid reports whether the ID, position and number are defined and fit
// their fields, per 3GPP TS 23.040 Sections 9.2.3.24.10.1.2, .4 and .10.3.
// It does not check Position against the message text.
func (o PredefinedObject) Valid() bool {
	limit := 10
	if o.ID == IEIPredefinedAnimation {
		limit = 15
	} else if o.ID != IEIPredefinedSound {
		return false
	}
	return o.Position >= 0 && o.Position <= 255 && o.Number >= 0 && o.Number < limit
}

// PredefinedSound decodes 3GPP TS 23.040 Section 9.2.3.24.10.1.2 without
// range checks. Another IEI returns a DecodeError for iei at offset 0
// wrapping ErrInvalid. Data shorter or longer than 2 octets returns a
// DecodeError for ied at offset 2 wrapping ErrUnderflow or ErrOverlength.
// Every error returns a zero PredefinedObject.
func (ie InformationElement) PredefinedSound() (PredefinedObject, error) {
	return ie.predefinedObject(IEIPredefinedSound)
}

// PredefinedAnimation decodes 3GPP TS 23.040 Section 9.2.3.24.10.1.4 without
// range checks, with the error contract of InformationElement.PredefinedSound.
func (ie InformationElement) PredefinedAnimation() (PredefinedObject, error) {
	return ie.predefinedObject(IEIPredefinedAnimation)
}

func (ie InformationElement) predefinedObject(id byte) (PredefinedObject, error) {
	if ie.ID != id {
		return PredefinedObject{}, NewDecodeError("iei", 0, ErrInvalid)
	}
	if err := ie.dataLength(2); err != nil {
		return PredefinedObject{}, err
	}
	return PredefinedObject{ID: ie.ID, Position: int(ie.Data[0]), Number: int(ie.Data[1])}, nil
}

// EMSObject is a positioned opaque object from 3GPP TS 23.040 Sections
// 9.2.3.24.10.1.3, .5 through .8, .11 and .12.
type EMSObject struct {
	// ID identifies the sound, animation, picture or WVG format.
	ID byte
	// Position is octet 1, counting characters before the object is played
	// or displayed. Its relation to the message text is the caller's concern.
	Position int
	// Data is a copy of octets 2..n: iMelody, bitmap frames or a WVG bit
	// stream. Its internal format is not interpreted.
	Data []byte
}

// Valid reports whether ID, Position and the data length are defined by
// 3GPP TS 23.040 Sections 9.2.3.24 and 9.2.3.24.10.3. User Defined Sound
// is at most 128 bytes; large animation and picture data are 128 bytes,
// small animation and picture data are 32 bytes. WVG data is opaque.
func (o EMSObject) Valid() bool {
	if o.Position < 0 || o.Position > 255 {
		return false
	}
	switch o.ID {
	case IEIUserDefinedSound:
		return len(o.Data) <= 128
	case IEILargeAnimation, IEILargePicture:
		return len(o.Data) == 128
	case IEISmallAnimation, IEISmallPicture:
		return len(o.Data) == 32
	case IEIStandardWVGObject, IEICharacterSizeWVGObject:
		return true
	default:
		return false
	}
}

// EMSObject decodes a User Defined Sound, Large/Small Animation,
// Large/Small Picture, Standard WVG Object or Character Size WVG Object,
// per 3GPP TS 23.040 Sections 9.2.3.24.10.1.3, .5 through .8, .11 and .12.
// Object data is copied without interpreting it or checking sound length.
// The IEI table in Section 9.2.3.24 specifies 128-byte large objects and
// 32-byte small objects, plus one position octet. Section 9.2.3.24.10.3
// specifies one bit per picture pixel and four sequential animation frames.
// Another IEI returns a DecodeError for iei at offset 0 wrapping ErrInvalid.
// Missing position or short/surplus fixed-size data returns a DecodeError
// for ied at offset 2 wrapping ErrUnderflow or ErrOverlength.
// Every error returns a zero EMSObject.
func (ie InformationElement) EMSObject() (EMSObject, error) {
	length := 1
	switch ie.ID {
	case IEIUserDefinedSound, IEIStandardWVGObject, IEICharacterSizeWVGObject:
		if len(ie.Data) > length {
			length = len(ie.Data)
		}
	case IEILargeAnimation, IEILargePicture:
		length = 129
	case IEISmallAnimation, IEISmallPicture:
		length = 33
	default:
		return EMSObject{}, NewDecodeError("iei", 0, ErrInvalid)
	}
	if err := ie.dataLength(length); err != nil {
		return EMSObject{}, err
	}
	return EMSObject{ID: ie.ID, Position: int(ie.Data[0]), Data: slices.Clone(ie.Data[1:])}, nil
}

// VariablePicture is the bitmap layout of 3GPP TS 23.040 Section
// 9.2.3.24.10.1.9. Bitmap content remains opaque.
type VariablePicture struct {
	// Position is octet 1, counting characters before display.
	Position int
	// Width is octet 2 multiplied by 8, in pixels; Height is octet 3 in pixels.
	Width, Height int
	// Data is a copy of the bitmap, one bit per pixel, top left to bottom
	// right, as defined in Section 9.2.3.24.10.3.2.
	Data []byte
}

// VariablePicture decodes 3GPP TS 23.040 Section 9.2.3.24.10.1.9. Octet 2
// "shall contain the horizontal number of 8 pixels i.e. this value shall
// be multiplied by 8 to get the whole number of horizontal pixels."
// Section 9.2.3.24.10.3.2 assigns one bit per pixel, so data length is
// Width * Height / 8 octets (octet 2 * octet 3). Zero dimensions require
// no bitmap octets. Message/segment capacity is the caller's concern.
// Another IEI returns a DecodeError for iei at offset 0 wrapping ErrInvalid.
// Short or surplus data returns a DecodeError for ied wrapping ErrUnderflow
// or ErrOverlength at offset 2 plus the affected parameter's data offset.
// Every error returns a zero VariablePicture.
func (ie InformationElement) VariablePicture() (VariablePicture, error) {
	if ie.ID != IEIVariablePicture {
		return VariablePicture{}, NewDecodeError("iei", 0, ErrInvalid)
	}
	r := ieDataReader{data: ie.Data}
	p := VariablePicture{Position: int(r.octet()), Width: int(r.octet()) * 8, Height: int(r.octet())}
	p.Data = slices.Clone(r.take(p.Width * p.Height / 8))
	if err := r.finish(); err != nil {
		return VariablePicture{}, err
	}
	return p, nil
}

// UserPromptIndicator decodes the number of corresponding objects in
// 3GPP TS 23.040 Section 9.2.3.24.10.1.10. The IE must immediately precede
// its objects; matching and stitching them is the caller's responsibility.
// Another IEI returns a DecodeError for iei at offset 0 wrapping ErrInvalid.
// Data shorter or longer than 1 octet returns a DecodeError for ied at
// offset 2 wrapping ErrUnderflow or ErrOverlength. Every error returns 0.
func (ie InformationElement) UserPromptIndicator() (int, error) {
	if ie.ID != IEIUserPromptIndicator {
		return 0, NewDecodeError("iei", 0, ErrInvalid)
	}
	if err := ie.dataLength(1); err != nil {
		return 0, err
	}
	return int(ie.Data[0]), nil
}

// ExtendedObjectType is the format code in 3GPP TS 23.040 Section
// 9.2.3.24.10.1.13. The formats themselves are defined in Annex E.
type ExtendedObjectType int

const (
	// ExtendedObjectPredefinedSound identifies a predefined sound.
	ExtendedObjectPredefinedSound ExtendedObjectType = 0x00
	// ExtendedObjectIMelody identifies an iMelody sound.
	ExtendedObjectIMelody ExtendedObjectType = 0x01
	// ExtendedObjectBlackWhiteBitmap identifies a black and white bitmap.
	ExtendedObjectBlackWhiteBitmap ExtendedObjectType = 0x02
	// ExtendedObjectGreyscaleBitmap identifies a 2-bit greyscale bitmap.
	ExtendedObjectGreyscaleBitmap ExtendedObjectType = 0x03
	// ExtendedObjectColourBitmap identifies a 6-bit colour bitmap.
	ExtendedObjectColourBitmap ExtendedObjectType = 0x04
	// ExtendedObjectPredefinedAnimation identifies a predefined animation.
	ExtendedObjectPredefinedAnimation ExtendedObjectType = 0x05
	// ExtendedObjectBlackWhiteAnimation identifies black and white bitmap animation.
	ExtendedObjectBlackWhiteAnimation ExtendedObjectType = 0x06
	// ExtendedObjectGreyscaleAnimation identifies 2-bit greyscale bitmap animation.
	ExtendedObjectGreyscaleAnimation ExtendedObjectType = 0x07
	// ExtendedObjectColourAnimation identifies 6-bit colour bitmap animation.
	ExtendedObjectColourAnimation ExtendedObjectType = 0x08
	// ExtendedObjectVCard identifies a vCard.
	ExtendedObjectVCard ExtendedObjectType = 0x09
	// ExtendedObjectVCalendar identifies a vCalendar.
	ExtendedObjectVCalendar ExtendedObjectType = 0x0a
	// ExtendedObjectStandardWVG identifies a Standard WVG object.
	ExtendedObjectStandardWVG ExtendedObjectType = 0x0b
	// ExtendedObjectPolyphonicMelody identifies a polyphonic melody.
	ExtendedObjectPolyphonicMelody ExtendedObjectType = 0x0c
	// ExtendedObjectDataFormatDeliveryRequest identifies a Data Format Delivery Request.
	ExtendedObjectDataFormatDeliveryRequest ExtendedObjectType = 0xff
)

// ExtendedObject is the first IE of an extended object, as defined in
// 3GPP TS 23.040 Section 9.2.3.24.10.1.13. "The subsequent Extended Object
// IEs shall only contain Extended Object data"; callers use their raw Data
// and reassemble in the sequence order of the 16-bit concatenation IEs.
// Reassembly across segments and decompression are not performed here.
type ExtendedObject struct {
	// Reference is octet 1; Length is the total object length in octets 2..3.
	Reference, Length int
	// DoNotForward and UserPrompt are control octet 4 bits 0 and 1.
	DoNotForward, UserPrompt bool
	// Reserved retains control bits 2..7 in their received bit positions.
	Reserved int
	// Type is octet 5. Values 0x0d..0xfe are reserved.
	Type ExtendedObjectType
	// Position is octets 6..7, relative to the entire concatenated message:
	// "the first character is numbered character 1" (Section 9.2.3.24.10.1.13).
	Position int
	// Data is a copy of the start of the object, octets 8..n.
	Data []byte
}

// Valid checks representable fields, zero reserved control bits, a defined
// type, a position of at least 1 and no more data than Length, per
// 3GPP TS 23.040 Section 9.2.3.24.10.1.13. It does not inspect object data
// or require that this first fragment contains the entire object.
func (o ExtendedObject) Valid() bool {
	return o.Reference >= 0 && o.Reference <= 255 && o.Length >= 0 && o.Length <= 65535 &&
		o.Reserved == 0 && (o.Type >= 0 && o.Type <= ExtendedObjectPolyphonicMelody || o.Type == ExtendedObjectDataFormatDeliveryRequest) &&
		o.Position >= 1 && o.Position <= 65535 && len(o.Data) <= o.Length
}

// ExtendedObject decodes only the first IE layout of 3GPP TS 23.040 Section
// 9.2.3.24.10.1.13, without range checks. Integer fields are big-endian per
// Section 9.1.2.1. Later IEs of the object contain only data and must not be
// passed to this decoder. Another IEI returns a DecodeError for iei at
// offset 0 wrapping ErrInvalid. A truncated header returns a DecodeError
// for ied wrapping ErrUnderflow at offset 2 plus the parameter's data
// offset. Every error returns a zero ExtendedObject. Data beyond Length
// is retained and makes Valid false.
func (ie InformationElement) ExtendedObject() (ExtendedObject, error) {
	if ie.ID != IEIExtendedObject {
		return ExtendedObject{}, NewDecodeError("iei", 0, ErrInvalid)
	}
	r := ieDataReader{data: ie.Data}
	o := ExtendedObject{Reference: int(r.octet()), Length: r.uint16()}
	control := r.octet()
	o.DoNotForward = control&1 != 0
	o.UserPrompt = control&2 != 0
	o.Reserved = int(control & 0xfc)
	o.Type = ExtendedObjectType(r.octet())
	o.Position = r.uint16()
	o.Data = slices.Clone(r.take(len(r.data) - r.pos))
	if err := r.finish(); err != nil {
		return ExtendedObject{}, err
	}
	return o, nil
}

// ReusedExtendedObject identifies a previously defined extended object and
// its new position, per 3GPP TS 23.040 Section 9.2.3.24.10.1.14.
type ReusedExtendedObject struct {
	// Reference is octet 1, the reference number of the object to reuse.
	Reference int
	// Position is octets 2..3, the absolute character position in the
	// concatenated message after which the object is played or displayed.
	Position int
}

// ReusedExtendedObject decodes 3GPP TS 23.040 Section 9.2.3.24.10.1.14
// without range checks, using Section 9.1.2.1 big-endian integer order.
// Another IEI returns a DecodeError for iei at offset 0 wrapping ErrInvalid.
// Data shorter or longer than 3 octets returns a DecodeError for ied at
// offset 2 wrapping ErrUnderflow or ErrOverlength. Every error returns a
// zero ReusedExtendedObject.
func (ie InformationElement) ReusedExtendedObject() (ReusedExtendedObject, error) {
	if ie.ID != IEIReusedExtendedObject {
		return ReusedExtendedObject{}, NewDecodeError("iei", 0, ErrInvalid)
	}
	if err := ie.dataLength(3); err != nil {
		return ReusedExtendedObject{}, err
	}
	return ReusedExtendedObject{Reference: int(ie.Data[0]), Position: int(binary.BigEndian.Uint16(ie.Data[1:]))}, nil
}

// CompressionLZSS is algorithm 0000 in 3GPP TS 23.040 Section 9.2.3.24.10.1.15.
const CompressionLZSS = 0

// CompressionControl is the first IE of a compressed sequence, per
// 3GPP TS 23.040 Section 9.2.3.24.10.1.15. The compression information and
// length occur in the first IE; subsequent IEs carry continuations of the
// compressed data. Callers reassemble their raw Data in segment order.
// This type neither reassembles segments nor decompresses the data.
type CompressionControl struct {
	// Algorithm is bits 0..3 of octet 1; only LZSS (0) is defined.
	Algorithm int
	// Parameters is bits 4..7 of octet 1, which "Shall be set 0."
	Parameters int
	// Length is octets 2..3, the total compressed data length across IEs.
	Length int
	// Data is a copy of octets 4..n, the start of the compressed sequence.
	Data []byte
}

// Valid reports whether the algorithm is LZSS, parameters are zero, Length
// fits two octets and Data is no longer than Length, per 3GPP TS 23.040
// Section 9.2.3.24.10.1.15. It does not inspect the compressed data.
func (c CompressionControl) Valid() bool {
	return c.Algorithm == CompressionLZSS && c.Parameters == 0 && c.Length >= 0 && c.Length <= 65535 && len(c.Data) <= c.Length
}

// CompressionControl decodes only the first IE layout of 3GPP TS 23.040
// Section 9.2.3.24.10.1.15 without range checks. Length is big-endian per
// Section 9.1.2.1. Later IEs contain only compressed data and must not be
// passed to this decoder. Another IEI returns a DecodeError for iei at
// offset 0 wrapping ErrInvalid. A truncated header returns a DecodeError
// for ied wrapping ErrUnderflow at offset 2 plus the parameter's data
// offset. Every error returns a zero CompressionControl. Data beyond
// Length is retained and makes Valid false.
func (ie InformationElement) CompressionControl() (CompressionControl, error) {
	if ie.ID != IEICompressionControl {
		return CompressionControl{}, NewDecodeError("iei", 0, ErrInvalid)
	}
	r := ieDataReader{data: ie.Data}
	info := r.octet()
	c := CompressionControl{Algorithm: int(info & 15), Parameters: int(info >> 4), Length: r.uint16()}
	c.Data = slices.Clone(r.take(len(r.data) - r.pos))
	if err := r.finish(); err != nil {
		return CompressionControl{}, err
	}
	return c, nil
}

// ObjectDistributionIndicator is the distribution control of 3GPP TS
// 23.040 Section 9.2.3.24.10.1.16.
type ObjectDistributionIndicator struct {
	// Count is octet 1, the number of following information elements.
	// "If set to 0 the Distribution Attributes shall apply to all
	// information elements until either the end of the message or another
	// Object Distribution Indicator IE is received."
	Count int
	// DoNotForward is bit 0 of octet 2: objects shall not be forwarded by SMS.
	DoNotForward bool
	// Reserved retains octet 2 bits 1..7 in their received bit positions.
	Reserved int
}

// Valid checks Count fits an octet and the reserved distribution bits are
// zero, per 3GPP TS 23.040 Section 9.2.3.24.10.1.16.
func (d ObjectDistributionIndicator) Valid() bool {
	return d.Count >= 0 && d.Count <= 255 && d.Reserved == 0
}

// ObjectDistributionIndicator decodes 3GPP TS 23.040 Section
// 9.2.3.24.10.1.16 without range checks. Another IEI returns a DecodeError
// for iei at offset 0 wrapping ErrInvalid. Data shorter or longer than 2
// octets returns a DecodeError for ied at offset 2 wrapping ErrUnderflow or
// ErrOverlength. Every error returns a zero ObjectDistributionIndicator.
func (ie InformationElement) ObjectDistributionIndicator() (ObjectDistributionIndicator, error) {
	if ie.ID != IEIObjectDistributionIndicator {
		return ObjectDistributionIndicator{}, NewDecodeError("iei", 0, ErrInvalid)
	}
	if err := ie.dataLength(2); err != nil {
		return ObjectDistributionIndicator{}, err
	}
	return ObjectDistributionIndicator{Count: int(ie.Data[0]), DoNotForward: ie.Data[1]&1 != 0, Reserved: int(ie.Data[1] & 0xfe)}, nil
}
