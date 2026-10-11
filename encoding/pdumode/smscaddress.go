// SPDX-License-Identifier: MIT

package pdumode

import (
	"github.com/gomaja/go-sms/encoding/semioctet"
	"github.com/gomaja/go-sms/encoding/tpdu"
)

// SMSCAddress is the address of the SMSC.
//
// The SMSCAddress is similar to a TPDU Address, but the binary form is
// marshalled differently, hence the subtype.
//
// The Type-of-number should typically be TonNational or TonInternational, but
// that is not enforced. SetNumber, promoted from the Address, gives a number
// that starts with '+' a TOA of 145, 0x91, international, and any other a
// TOA of 129, 0x81, of unknown type, the defaults 3GPP TS 27.005 Section 3.1
// gives <tosca>.
//
// The NumberingPlan should typically be NpISDN, but that is not enforced
// either.
//
// The PDU either carries an SMSC address or has a zero length octet in its
// place, in which case the SMSC set with +CSCA is used and no TOA is present
// (3GPP TS 27.005 Section 3.3.1 and Section 4.3). The zero value is that
// absent address. An address with digits is always present, and Present
// records a received address without digits for inspection. Such an address
// cannot be marshalled: TS 24.011 Sections 8.2.5.1 and 8.2.5.2 give a
// non-zero length octet a minimum value of 2, counting the TOA and digits.
type SMSCAddress struct {
	tpdu.Address

	// Present indicates the address is present even if Addr is empty.
	//
	// UnmarshalBinary sets it for any non-zero length octet, including a
	// TOA-only or fill-only field. MarshalBinary rejects it without digits.
	Present bool
}

// maxSMSCAddressLen is the largest value of the SMSC address length octet.
//
// 3GPP TS 24.011 Sections 8.2.5.1 and 8.2.5.2 give "the maximum value is 11"
// for the RP-Originator and RP-Destination Address, so the length octet
// counts at most the TOA and 10 octets of digits, which hold 20 digits.
const maxSMSCAddressLen = 11

// MarshalBinary marshals the SMSC Address into binary.
//
// An absent address, one with no digits and Present not set, is marshalled as
// a single zero length octet, with no TOA. A present address without digits
// returns ErrInvalid. 3GPP TS 27.005 Section 3.1 defines <pdu> as "3GPP TS
// 24.011 [6] SC address followed by 3GPP TS 23.040 [3] TPDU". TS 24.011
// Sections 8.2.5.1 and 8.2.5.2 say "the minimum value of the length octet is
// 2 and the maximum value is 11" in the direction that carries an address.
// Otherwise the address is marshalled as the length octet, the TOA and the
// digits. The TOA is always marshalled with bit 7 set.
//
// An address longer than 20 digits returns an error, as its length would not
// fit the field.
func (a *SMSCAddress) MarshalBinary() (dst []byte, err error) {
	addr, err := semioctet.Encode([]byte(a.Addr))
	if err != nil {
		return nil, tpdu.NewEncodeError("addr", err)
	}
	if len(addr) == 0 {
		if a.Present {
			return nil, tpdu.NewEncodeError("addr", tpdu.ErrInvalid)
		}
		return []byte{0}, nil
	}
	l := len(addr) + 1 // in octets and includes the toa
	if l > maxSMSCAddressLen {
		return nil, tpdu.NewEncodeError("addr", tpdu.ErrOverlength)
	}
	dst = make([]byte, 2, l+1)
	dst[0] = byte(l)
	// Bit 7 of the TOA is always 1 (3GPP TS 23.040 Section 9.1.2.5,
	// Type-of-Address figure), as in the 145 and 129 defaults for <tosca> of
	// 3GPP TS 27.005 Section 3.1, which SetNumber sets, but a TOA set
	// directly may lack it.
	dst[1] = a.TOA | 0x80
	dst = append(dst, addr...)
	return dst, nil
}

// UnmarshalBinary unmarshals an SMSC Address from the start of a PDU, where
// it precedes the TPDU.
//
// It returns the number of bytes read from the source, and any error detected
// while decoding.
//
// The address is cleared first, so a zero length octet, which selects the
// default SMSC, and any error both leave it empty rather than holding a
// previously decoded address.
func (a *SMSCAddress) UnmarshalBinary(src []byte) (int, error) {
	*a = SMSCAddress{}
	if len(src) < 1 {
		return 0, tpdu.NewDecodeError("length", 0, tpdu.ErrUnderflow)
	}
	l := int(src[0]) // len is octets including toa
	if l > maxSMSCAddressLen {
		return 1, tpdu.NewDecodeError("length", 0, tpdu.ErrOverlength)
	}
	if l == 0 {
		return 1, nil
	}
	if len(src) < 2 {
		return 1, tpdu.NewDecodeError("toa", 1, tpdu.ErrUnderflow)
	}
	toa := src[1]
	ri := 2
	l-- // encoded length includes toa
	if len(src) < ri+l {
		return len(src), tpdu.NewDecodeError("addr", ri, tpdu.ErrUnderflow)
	}
	baddr, n, err := semioctet.Decode(make([]byte, l*2), src[ri:ri+l])
	ri += n
	// should never trip - semioctet.Decode can only fail if dst length is odd.
	if err != nil {
		return ri, tpdu.NewDecodeError("addr", ri-n, err)
	}
	a.Addr = string(baddr)
	a.TOA = toa
	a.Present = true
	return ri, nil
}
