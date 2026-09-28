// SPDX-License-Identifier: MIT

package pdumode

import (
	"github.com/gomaja/go-sms/encoding/semioctet"
	"github.com/gomaja/go-sms/encoding/tpdu"
)

// SMSCAddress is the address of the SMSC.
//
// The SMCSAddress is similar to a TPDU Address, but the binary form is
// marshalled differently, hence the subtype.
//
// The Type-of-number should typically be TonNational or TonInternational, but
// that is not enforced.
//
// The NumberingPlan should typically be NpISDN, but that is not enforced
// either.
//
// The PDU either carries an SMSC address or has a zero length octet in its
// place, in which case the SMSC set with +CSCA is used and no TOA is present
// (3GPP TS 27.005 Section 3.3.1 and Section 4.3). The zero value is that
// absent address. An address with digits is always present, and Present
// marks an address without digits as present, so that a field holding only a
// TOA is kept distinct from the absent address.
type SMSCAddress struct {
	tpdu.Address

	// Present indicates the address is present even if Addr is empty.
	//
	// UnmarshalBinary sets it for any non-zero length octet.
	Present bool
}

// maxSMSCAddressLen is the largest value of the SMSC address length octet.
//
// The SMSC address is an LV element of 1-12 octets, counting the length
// octet itself (3GPP TS 27.005 Section 2.4.1.8, RP-Destination-Address, and
// Sections 2.5.2.5 and 2.5.2.6, "The address is of variable length, 1-12
// octets"). So the length octet counts at most 11 octets, the TOA and 10
// octets of digits, which hold 20 digits.
const maxSMSCAddressLen = 11

// MarshalBinary marshals the SMSC Address into binary.
//
// An absent address, one with no digits and Present not set, is marshalled as
// a single zero length octet, with no TOA. Any other address is marshalled as
// the length octet, the TOA and the digits, so an address with a TOA but no
// digits has a length of 1.
//
// An address longer than 20 digits returns an error, as its length would not
// fit the field.
func (a *SMSCAddress) MarshalBinary() (dst []byte, err error) {
	addr, err := semioctet.Encode([]byte(a.Addr))
	if err != nil {
		return nil, tpdu.EncodeError("addr", err)
	}
	if len(addr) == 0 && !a.Present {
		return []byte{0}, nil
	}
	l := len(addr) + 1 // in octets and includes the toa
	if l > maxSMSCAddressLen {
		return nil, tpdu.EncodeError("addr", tpdu.ErrOverlength)
	}
	dst = make([]byte, 2, l+1)
	dst[0] = byte(l)
	dst[1] = a.TOA
	dst = append(dst, addr...)
	return dst, nil
}

// UnmarshalBinary unmarshals an SMSC Address from a TPDU field.
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
