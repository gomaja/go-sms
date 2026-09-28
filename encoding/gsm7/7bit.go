// SPDX-License-Identifier: MIT

package gsm7

import "fmt"

const cr byte = 0x0d

// Pack7Bit packs an array of septets into an 8bit array as per the packing
// rules defined in 3GPP TS 23.038 Section 6.1.2.1.1.
//
// The fillBits is the number of zero bits to place at the beginning of the
// packed array, as the packed septets may not start on an octet boundary.
// These are the fill bits that pad a User Data Header to a septet boundary
// (3GPP TS 23.040 Section 9.2.3.24), so fillBits must be 0 to 6. Pack7Bit
// panics on any other value, which is a programming error.
//
// A septet has 7 bits, so Pack7Bit returns an ErrInvalidSeptet for the first
// byte of u above 0x7F rather than let its 8th bit corrupt the next septet.
//
// The last octet is completed with zeros, so 8n-1 septets pack to the same
// octets as the same septets followed by a 0x00 septet. See Unpack7Bit.
func Pack7Bit(u []byte, fillBits int) ([]byte, error) {
	checkFillBits(fillBits)
	for i, s := range u {
		if s > 0x7f {
			return nil, ErrInvalidSeptet{Offset: i, Septet: s}
		}
	}
	if len(u) == 0 {
		return append(u[:0:0], u...), nil
	}
	p := make([]byte, 0, (len(u)*7+7+fillBits)/8)
	var r, s byte
	rbits := uint(fillBits)
	for _, s = range u {
		if rbits == 0 {
			// no residual bits so not enough for a full octet
			r = s
			rbits = 7
			continue
		}
		r = (r | s<<rbits) & 0xff
		p = append(p, r)
		r = s >> (8 - rbits)
		rbits--
	}
	if rbits != 0 {
		p = append(p, r)
	}
	return p, nil
}

// Unpack7Bit unpacks septets, packed into an 8bit array as per the packing
// rules defined in 3GPP TS 23.038 Section 6.1.2.1.1, into an array of septets.
//
// The fillBits is the number of bits of pad at the beginning of p, as the
// packed septets may not start on an octet boundary. As for Pack7Bit, it must
// be 0 to 6, and Unpack7Bit panics on any other value.
//
// Unpack7Bit returns a septet for every 7 bits of p after the fill bits,
// which is (len(p)*8-fillBits)/7 septets. The octets do not say how many
// septets were packed into them: where the packed septets leave 7 bits spare
// in the last octet, as 8n-1 septets with no fill bits do, those bits are
// returned as a final 0x00 septet, which is '@', and the caller must drop it.
// The number of septets is carried separately, as in the TP-User-Data-Length
// of 3GPP TS 23.040 Section 9.2.3.16.
func Unpack7Bit(p []byte, fillBits int) []byte {
	checkFillBits(fillBits)
	if len(p) == 0 {
		return append(p[:0:0], p...)
	}
	u := make([]byte, 0, (len(p)*8+6+fillBits)/7)
	var r byte
	var rbits uint
	if fillBits != 0 {
		rbits = uint(7 - fillBits)
	}
	for _, o := range p {
		r = (r | o<<rbits) & 0x7f
		u = append(u, r)
		if rbits == 6 {
			// only needed 1 bit from p, so there is a complete septet left...
			u = append(u, o>>1)
			rbits = 0
			r = 0
		} else {
			// each octet provides one extra residual bit
			rbits++
			r = o >> (8 - rbits)
		}
	}
	if fillBits > 0 {
		u = u[1:]
	}
	return u
}

// checkFillBits panics if fillBits is not a number of fill bits that can pad
// a User Data Header to a septet boundary.
func checkFillBits(fillBits int) {
	if fillBits < 0 || fillBits > 6 {
		panic(fmt.Sprintf("gsm7: fillBits %d not in range 0..6", fillBits))
	}
}

// Pack7BitUSSD packs an array of septets into an 8bit array as per the USSD
// packing rules defined in 3GPP TS 23.038 Section 6.1.2.3.1.
//
// The septets are packed as for SMS, with no fill bits. A message of 8n-1
// septets leaves 7 spare bits in the final octet, and these are filled with
// CR rather than zeroes so the receiver does not decode them as '@'. A message
// of 8n septets that ends with CR ends on an octet boundary, where the
// receiver would discard that CR as filler, so a second CR is appended.
//
// As for Pack7Bit, it returns an ErrInvalidSeptet for a byte above 0x7F.
func Pack7BitUSSD(u []byte) ([]byte, error) {
	p, err := Pack7Bit(u, 0)
	if err != nil {
		return nil, err
	}
	switch {
	case len(u)%8 == 7:
		p[len(p)-1] |= cr << 1
	case len(u)%8 == 0 && len(u) > 0 && u[len(u)-1] == cr:
		p = append(p, cr)
	}
	return p, nil
}

// Unpack7BitUSSD unpacks septets, packed into an 8bit array as per the USSD
// packing rules defined in 3GPP TS 23.038 Section 6.1.2.3.1, into an array of
// septets.
//
// When the septets end on an octet boundary, which is when the number of
// octets is a multiple of 7, a final CR is filler and is removed. A message
// that the sender ended with a doubled CR keeps both, as CR CR is defined to
// be identical to CR (3GPP TS 23.038 Section 6.1.1).
func Unpack7BitUSSD(p []byte) []byte {
	u := Unpack7Bit(p, 0)
	if len(p)%7 == 0 && len(u) > 0 && u[len(u)-1] == cr {
		u = u[:len(u)-1]
	}
	return u
}
