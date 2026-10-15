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

// Pack7BitCR packs septets as Pack7Bit does, with the CR padding used for
// USSD in 3GPP TS 23.038 V20.0.0 Section 6.1.2.3.1. It is for data whose
// length is given in octets rather than septets.
//
// The fillBits is the number of zero bits before the septets. As for Pack7Bit,
// it must be 0 to 6, and Pack7BitCR panics on any other value. It returns an
// ErrInvalidSeptet for the first byte above 0x7F. An empty u packs to no octets.
//
// If the fill bits and septets leave 7 spare bits in the last octet, those
// bits are filled with CR so the receiver does not decode them as '@'. If a
// final CR ends on an octet boundary, another CR and a zero padding bit are
// appended. All other spare bits are zero.
func Pack7BitCR(u []byte, fillBits int) ([]byte, error) {
	p, err := Pack7Bit(u, fillBits)
	if err != nil {
		return nil, err
	}
	if len(u) == 0 {
		return p, nil
	}
	spare := (8 - (fillBits+7*len(u))%8) % 8
	switch {
	case spare == 7:
		p[len(p)-1] |= cr << 1
	case spare == 0 && u[len(u)-1] == cr:
		p = append(p, cr)
	}
	return p, nil
}

// Unpack7BitCR unpacks septets as Unpack7Bit does, and removes a final CR
// where the septets end on an octet boundary, following 3GPP TS 23.038
// V20.0.0 Section 6.1.2.3.1. The fillBits must be 0 to 6, as for Unpack7Bit;
// Unpack7BitCR panics on any other value. No octets unpack to no septets.
//
// A final CR doubled by Pack7BitCR keeps both CRs: Section 6.1.2.3.1 defines
// CR and CR CR to have the same effect. A caller that knows the number of
// septets can instead take that many from Unpack7Bit.
func Unpack7BitCR(p []byte, fillBits int) []byte {
	u := Unpack7Bit(p, fillBits)
	if (8*len(p)-fillBits)%7 == 0 && len(u) > 0 && u[len(u)-1] == cr {
		u = u[:len(u)-1]
	}
	return u
}

// Pack7BitUSSD packs septets using the USSD packing rules defined in 3GPP
// TS 23.038 Section 6.1.2.3.1. It is Pack7BitCR with no fill bits.
//
// As for Pack7Bit, it returns an ErrInvalidSeptet for a byte above 0x7F.
func Pack7BitUSSD(u []byte) ([]byte, error) {
	return Pack7BitCR(u, 0)
}

// Unpack7BitUSSD unpacks septets using the USSD packing rules defined in
// 3GPP TS 23.038 Section 6.1.2.3.1. It is Unpack7BitCR with no fill bits.
func Unpack7BitUSSD(p []byte) []byte {
	return Unpack7BitCR(p, 0)
}
