// SPDX-License-Identifier: MIT

package tpdu

import (
	"fmt"
	"strings"
)

// PI is the first octet of the TP-PI parameter indicator, as defined in 3GPP
// TS 23.040 Section 9.2.3.27.
type PI byte

// PID returns true if a PID field is present in the TPDU.
func (p PI) PID() bool {
	return p&PiPID != 0
}

// DCS returns true if a DCS field is present in the TPDU.
func (p PI) DCS() bool {
	return p&PiDCS != 0
}

// UDL returns true if a UDL, and hence a UD, field is present in the TPDU.
func (p PI) UDL() bool {
	return p&PiUDL != 0
}

func (p PI) String() string {
	if p == 0 {
		return "0"
	}
	elems := []string{}
	if p.PID() {
		elems = append(elems, "PID")
	}
	if p.DCS() {
		elems = append(elems, "DCS")
	}
	if p.UDL() {
		elems = append(elems, "UDL")
	}
	if r := p & PiReserved; r != 0 {
		elems = append(elems, fmt.Sprintf("0x%02x", byte(r)))
	}
	if p&PiExt != 0 {
		elems = append(elems, "EXT")
	}
	return strings.Join(elems, "|")
}

const (
	// PI bit fields

	// PiPID indicates a TP-PID field is present in the TPDU
	PiPID = 1 << iota

	// PiDCS indicates a TP-DCS field is present in the TPDU
	PiDCS

	// PiUDL indicates a TP-UDL field is present in the TPDU
	PiUDL
)

const (
	// PiReserved masks the reserved bits, 3 to 6, of the first TP-PI octet.
	//
	// 3GPP TS 23.040 Section 9.2.3.27: "If a Reserved bit is set to "1" then
	// the receiving entity shall ignore the setting. The setting of this bit
	// shall mean that additional information will follow the TP-User-Data,
	// so a receiving entity shall discard any octets following the
	// TP-User-Data."
	PiReserved = 0x78

	// PiExt is the extension bit of each TP-PI octet, which indicates that
	// another TP-PI octet follows.
	PiExt = 0x80
)
