// SPDX-License-Identifier: MIT

package tpdu

import "fmt"

// DCS represents the SMS TP-Data-Coding-Scheme field specified by 3GPP TS
// 23.040 Section 9.2.3.10 and defined by 3GPP TS 23.038 Section 4.
type DCS byte

// Alphabet defines the encoding of the SMS User Data, as defined in 3GPP TS
// 23.038 Section 4.
type Alphabet int

const (
	// Alpha7Bit indicates that the UD is encoded using GSM 7 bit encoding.
	// The character set used for the decoding is determined from the UDH.
	Alpha7Bit Alphabet = iota

	// Alpha8Bit indicates that the UD is encoded as raw 8bit data.
	Alpha8Bit

	// AlphaUCS2 indicates that the UD is encoded as UCS-2 (16bit) characters.
	AlphaUCS2

	// AlphaReserved is the reserved character set of the general data coding
	// groups (bits 3..2 set to 11).
	//
	// DCS.Alphabet never returns it, as a receiving entity treats reserved
	// codings as the GSM 7 bit default alphabet, and DCS.WithAlphabet does not
	// accept it.
	AlphaReserved
)

// Alphabet returns the alphabet used to encode the User Data according to the DCS.
//
// The DCS is interpreted as per 3GPP TS 23.038 Section 4, which requires a
// receiving entity to assume that any reserved coding, including the reserved
// coding groups 1000..1011 and the reserved character set of groups 00xx and
// 01xx, is the GSM 7 bit default alphabet.
func (d DCS) Alphabet() Alphabet {
	switch {
	case d&0x80 == 0x00: // 0xxx
		if alpha := Alphabet((d >> 2) & 0x3); alpha != AlphaReserved {
			return alpha
		}
	case d&0xf0 == 0xe0: // 1110
		return AlphaUCS2
	case d&0xf0 == 0xf0: // 1111
		if d&0x04 == 0x04 {
			return Alpha8Bit
		}
	}
	// 110x, and the reserved codings including the 10xx coding groups.
	return Alpha7Bit
}

// ApplyTPDUOption applies the DCS value to the TPDU DCS field.
func (d DCS) ApplyTPDUOption(t *TPDU) error {
	t.SetDCS(byte(d))
	return nil
}

func (d DCS) String() string {
	str := fmt.Sprintf("0x%02x", int(d))
	switch d.Alphabet() {
	case Alpha7Bit:
		str += " 7bit"
	case Alpha8Bit:
		str += " 8bit"
	case AlphaUCS2:
		str += " UCS-2"
	}
	return str
}

// WithAlphabet sets the Alphabet bits of the DCS, given the state of the other
// bits.
//
// An error is returned if the alphabet is not one of Alpha7Bit, Alpha8Bit or
// AlphaUCS2, or if the state is incompatible with setting the alphabet.
func (d DCS) WithAlphabet(a Alphabet) (DCS, error) {
	if a < Alpha7Bit || a >= AlphaReserved {
		return d, ErrInvalid
	}
	switch {
	case d&0x80 == 0x00: // 0xxx
		return d&^0x0c | (DCS(a) << 2), nil
	case d&0xe0 == 0xc0 && a == Alpha7Bit: // 110x is 7Bit
		return d, nil
	case d&0xf0 == 0xe0 && a == AlphaUCS2: // 1110 is UCS2
		return d, nil
	case d&0xf0 == 0xf0 && a <= Alpha8Bit: // 1111, with reserved bit 3 cleared
		return d&^0x0c | (DCS(a) << 2), nil
	default: // 110x or 1110 with another alphabet, and 10xx reserved coding groups
		return d, ErrInvalid
	}
}

const (
	// Dcs8BitData is a DCS indicating 8 bit data
	Dcs8BitData DCS = 0x04

	// DcsUCS2Data is a DCS indicating UCS2 data
	DcsUCS2Data DCS = 0x08
)

// MessageClass indicates the class of the message as specified in 3GPP TS
// 23.038 Section 4.
type MessageClass int

const (
	// MClass0 is a flash message which is not to be stored in memory.
	MClass0 MessageClass = iota

	// MClass1 is an ME specific message.
	MClass1

	// MClass2 is a SIM/USIM specific message.
	MClass2

	// MClass3 is a TE specific message.
	MClass3

	// MClassUnknown indicates no message class is set.
	MClassUnknown
)

// Class returns the MessageClass indicated by the DCS, or MClassUnknown if the
// DCS carries no message class.
//
// The DCS is interpreted as per 3GPP TS 23.038 Section 4. Only groups 00xx and
// 01xx with bit 4 set, and group 1111, carry a message class. A reserved
// coding is assumed to be 00000000, which has no message class.
func (d DCS) Class() MessageClass {
	if d&0x90 == 0x10 || d&0xf0 == 0xf0 { // 0xx1 and 1111
		return MessageClass(d & 0x3)
	}
	return MClassUnknown
}

// WithClass sets the MessageClass bits of the DCS, given the state of the
// other bits.
//
// MClassUnknown removes the message class, which is only possible in the
// groups 00xx and 01xx, and is a no-op for the groups 110x and 1110, which
// never carry a class.
//
// An error is returned if the class is out of range, or if the state is
// incompatible with setting the message class.
func (d DCS) WithClass(c MessageClass) (DCS, error) {
	switch {
	case c < MClass0 || c > MClassUnknown:
		return d, ErrInvalid
	case d&0x80 == 0x00: // 0xxx
		if c == MClassUnknown {
			// bit 4 cleared, so bits 1..0 are reserved and set to 0.
			return d &^ 0x13, nil
		}
		return d&^0x03 | 0x10 | DCS(c), nil
	case d&0xf0 == 0xf0: // 1111 always carries a class
		if c == MClassUnknown {
			return d, ErrInvalid
		}
		return d&^0x03 | DCS(c), nil
	case d&0xe0 == 0xc0, d&0xf0 == 0xe0: // 110x and 1110 never carry a class
		if c == MClassUnknown {
			return d, nil
		}
		return d, ErrInvalid
	default: // 10xx reserved coding groups
		return d, ErrInvalid
	}
}

// Compressed indicates whether the text is compressed using the algorithm
// defined in 3GPP TS 23.042, as determined from the DCS.
//
// The DCS is assumed to be defined as per 3GPP TS 23.038 Section 4.
func (d DCS) Compressed() bool {
	// only true for 0x1xxxxx (binary)
	return (d&0xa0 == 0x20)
}
