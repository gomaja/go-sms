// SPDX-License-Identifier: MIT

package cbs

// DCS is the CBS Data Coding Scheme octet of 3GPP TS 23.038 V20.0.0
// Section 5: "The CBS Data Coding Scheme indicates the intended handling
// of the message at the MS, the character set/coding, and the language
// (when applicable)."
type DCS byte

const (
	// GSM7 is codepoint 00001111, "Language unspecified", using the
	// "GSM 7 bit default alphabet" (TS 23.038 Section 5, group 0000).
	// A receiving entity assumes this coding for every reserved coding.
	GSM7 DCS = 0x0f
	// UCS2 is uncompressed general data coding without a message class,
	// "UCS2 (16 bit)" (TS 23.038 Section 5, group 01xx).
	UCS2 DCS = 0x48
)

// Group identifies the coding group of TS 23.038 Section 5: "The octet
// is used according to a coding group which is indicated in bits 7..4."
// Its values are categories, not the numeric values of those four bits.
type Group int

const (
	// GroupLanguage is 0000: "Language using the GSM 7 bit default
	// alphabet" (TS 23.038 Section 5).
	GroupLanguage Group = iota
	// GroupLanguageIndication is 0001: "message preceded by language
	// indication" (TS 23.038 Section 5).
	GroupLanguageIndication
	// GroupLanguage2 is 0010: "Czech", "Hebrew", "Arabic", "Russian",
	// "Icelandic", and reserved languages (TS 23.038 Section 5).
	GroupLanguage2
	// GroupLanguage3 is 0011: "Reserved for other languages using the
	// GSM 7 bit default alphabet" (TS 23.038 Section 5).
	GroupLanguage3
	// GroupGeneralDataCoding is 01xx: "General Data Coding indication"
	// (TS 23.038 Section 5).
	GroupGeneralDataCoding
	// GroupUserDataHeader is 1001: "Message with User Data Header (UDH)
	// structure" (TS 23.038 Section 5).
	GroupUserDataHeader
	// GroupI1Protocol is 1101: "I1 protocol message defined in 3GPP
	// TS 24.294" (TS 23.038 Section 5).
	GroupI1Protocol
	// GroupWAP is 1110: "Defined by the WAP Forum" (TS 23.038 Section 5).
	GroupWAP
	// GroupDataCoding is 1111: "Data coding / message handling"
	// (TS 23.038 Section 5).
	GroupDataCoding
	// GroupReserved is 1000 or 1010..1100: "Reserved coding groups"
	// (TS 23.038 Section 5).
	GroupReserved
)

// Group returns the "coding group which is indicated in bits 7..4"
// (TS 23.038 Section 5), including for reserved codings within a group.
func (d DCS) Group() Group {
	switch d >> 4 {
	case 0:
		return GroupLanguage
	case 1:
		return GroupLanguageIndication
	case 2:
		return GroupLanguage2
	case 3:
		return GroupLanguage3
	case 4, 5, 6, 7:
		return GroupGeneralDataCoding
	case 9:
		return GroupUserDataHeader
	case 13:
		return GroupI1Protocol
	case 14:
		return GroupWAP
	case 15:
		return GroupDataCoding
	default:
		return GroupReserved
	}
}

// Charset is the "character set/coding" indicated by the CBS Data Coding
// Scheme (TS 23.038 Section 5).
type Charset int

const (
	// CharsetGSM7 is the "GSM 7 bit default alphabet" of TS 23.038
	// Section 5, including its extension table (Sections 6.2.1, 6.2.1.1).
	CharsetGSM7 Charset = iota
	// Charset8Bit is "8 bit data" (TS 23.038 Section 5).
	Charset8Bit
	// CharsetUCS2 is "UCS2 (16 bit)" (TS 23.038 Section 5).
	CharsetUCS2
	// CharsetUnknown applies to groups 1101 and 1110, whose coding is
	// specified elsewhere: "I1 protocol message defined in 3GPP TS 24.294"
	// and "Defined by the WAP Forum" (TS 23.038 Section 5).
	CharsetUnknown
)

// Charset returns the character set of d. TS 23.038 Section 5 requires:
// "Any reserved codings shall be assumed to be the GSM 7 bit default
// alphabet (the same as codepoint 00001111) by a receiving entity."
func (d DCS) Charset() Charset {
	if d.Reserved() {
		return CharsetGSM7
	}
	switch d.Group() {
	case GroupLanguageIndication:
		if d == 0x11 || d == 0x12 {
			return CharsetUCS2
		}
	case GroupGeneralDataCoding, GroupUserDataHeader:
		return Charset((d >> 2) & 3)
	case GroupI1Protocol, GroupWAP:
		return CharsetUnknown
	case GroupDataCoding:
		if d&4 != 0 {
			return Charset8Bit
		}
	}
	return CharsetGSM7
}

// Reserved reports a reserved coding in the TS 23.038 Section 5 table.
// "Any reserved codings shall be assumed to be the GSM 7 bit default
// alphabet (the same as codepoint 00001111) by a receiving entity."
// This includes the reserved alphabet of groups 01xx and 1001, and group
// 1111 with bit 3 set ("Bit 3 is reserved, set to 0"). Bits 1..0 in group
// 01xx without bit 4 merely "have no message class meaning" and are ignored.
func (d DCS) Reserved() bool {
	switch d.Group() {
	case GroupLanguageIndication:
		return d > 0x12
	case GroupLanguage2:
		return d > 0x24
	case GroupLanguage3, GroupReserved:
		return true
	case GroupGeneralDataCoding, GroupUserDataHeader:
		return d&0x0c == 0x0c
	case GroupDataCoding:
		return d&8 != 0
	default:
		return false
	}
}

// Compressed reports compression in a non-reserved general data coding.
// TS 23.038 Section 5, group 01xx: "Bit 5, if set to 1, indicates the text
// is compressed using the compression algorithm defined in 3GPP TS 23.042".
// All other groups and reserved codings return false.
func (d DCS) Compressed() bool {
	return !d.Reserved() && d.Group() == GroupGeneralDataCoding && d&0x20 != 0
}

// MessageClass is the "Message Class" indicated by TS 23.038 Section 5.
type MessageClass int

const (
	// Class0 is "Class 0" (TS 23.038 Section 5): display immediately,
	// without automatic storage, when the MS can display CBS messages.
	Class0 MessageClass = iota
	// Class1 is "Class 1 Default meaning: ME-specific" in groups 01xx
	// and 1001, and "Class 1 user defined" in 1111 (TS 23.038 Section 5).
	Class1
	// Class2 is "Class 2 (U)SIM specific message" in groups 01xx and
	// 1001, and "Class 2 user defined" in 1111 (TS 23.038 Section 5).
	Class2
	// Class3 is "Class 3 Default meaning: TE-specific" (TS 23.038 Section 5).
	Class3
	// NoClass means "No message class" (TS 23.038 Section 5, group 1111),
	// or a coding with no message class indication.
	NoClass
)

// Class returns the message class, or NoClass for reserved codings and
// codings without a class. TS 23.038 Section 5, group 01xx: "Bit 4, if set
// to 1, indicates that bits 1 to 0 have a message class meaning". Group
// 1001 always has a class; group 1111 maps 00 to "No message class",
// unlike the SMS Data Coding Scheme of Section 4.
func (d DCS) Class() MessageClass {
	if d.Reserved() {
		return NoClass
	}
	switch d.Group() {
	case GroupGeneralDataCoding:
		if d&0x10 != 0 {
			return MessageClass(d & 3)
		}
	case GroupUserDataHeader:
		return MessageClass(d & 3)
	case GroupDataCoding:
		if d&3 != 0 {
			return MessageClass(d & 3)
		}
	}
	return NoClass
}

// Language returns the ISO 639-1 code for the language in groups 0000 and
// 0010, where "Bits 3..0 indicate the language" (TS 23.038 Section 5).
// "Language unspecified", a reserved coding, and every other group return
// "", false. Section 5 NOTE 2 cautions: "Message text in Hebrew, Arabic
// and Russian cannot be encoded in the GSM 7-bit default alphabet. For
// these languages UCS2 encoding shall be used."
func (d DCS) Language() (string, bool) {
	if d <= 0x0e {
		return [...]string{"de", "en", "it", "fr", "es", "nl", "sv", "da", "pt", "fi", "no", "el", "tr", "hu", "pl"}[d], true
	}
	if d >= 0x20 && d <= 0x24 {
		return [...]string{"cs", "he", "ar", "ru", "is"}[d-0x20], true
	}
	return "", false
}

// LanguageIndication identifies the prefix of group 0001, "message
// preceded by language indication" (TS 23.038 Section 5).
type LanguageIndication int

const (
	// NoLanguageIndication means no "language indication" prefix from
	// TS 23.038 Section 5, group 0001, is present.
	NoLanguageIndication LanguageIndication = iota
	// LanguageIndicationGSM7 is 00010000: "a two-character representation
	// of the language encoded according to ISO 639 [12], followed by a CR
	// character" (TS 23.038 Section 5), then GSM7 text.
	LanguageIndicationGSM7
	// LanguageIndicationUCS2 is 00010001: "two GSM 7-bit default alphabet
	// character representation of the language", "padded to the octet
	// boundary with two bits set to 0" (TS 23.038 Section 5), then UCS2 text.
	LanguageIndicationUCS2
	// LanguageIndicationUCS2Letters is 00010010: "The first 15 bits of
	// the two octets are used to represent 3 letters" (TS 23.038 Section 5,
	// figure 5-1 and table 5-1), then UCS2 text.
	LanguageIndicationUCS2Letters
)

// LanguageIndication returns the form of "language indication" in
// TS 23.038 Section 5, group 0001, or NoLanguageIndication for every other
// coding, including reserved codings.
func (d DCS) LanguageIndication() LanguageIndication {
	switch d {
	case 0x10:
		return LanguageIndicationGSM7
	case 0x11:
		return LanguageIndicationUCS2
	case 0x12:
		return LanguageIndicationUCS2Letters
	default:
		return NoLanguageIndication
	}
}
