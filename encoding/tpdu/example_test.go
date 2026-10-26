// SPDX-License-Identifier: MIT

package tpdu_test

import (
	"fmt"

	"github.com/gomaja/go-sms/encoding/tpdu"
)

func ExampleInformationElement_ConcatInfo() {
	// Part 5 of 3, which a receiving entity ignores, so the header has no
	// usable concatenation IE.
	udh := tpdu.UserDataHeader{{ID: tpdu.IEIConcat8Bit, Data: []byte{78, 3, 5}}}
	_, ok := udh.ConcatInfo()
	fmt.Println("concatenated:", ok)

	// Decoding the IEs themselves gives the values the header carries.
	for _, ie := range udh {
		ci, err := ie.ConcatInfo()
		if err != nil {
			continue // not a concatenation IE, or one of the wrong length
		}
		fmt.Printf("reference %d, part %d of %d, valid %t\n", ci.Ref, ci.Seqno, ci.Total, ci.Valid())
	}
	// Output:
	// concatenated: false
	// reference 78, part 5 of 3, valid false
}

func ExampleUserDataHeader_PortAddress() {
	var header tpdu.UserDataHeader
	_, err := header.UnmarshalBinary([]byte{6, 5, 4, 0x0b, 0x84, 0x23, 0xf0})
	if err != nil {
		fmt.Println(err)
		return
	}
	port, ok := header.PortAddress()
	fmt.Printf("destination %d, originator %d, 16-bit %t, valid %t\n", port.Dest, port.Orig, port.Port16Bit, ok)
	// Output:
	// destination 2948, originator 9200, 16-bit true, valid true
}

func ExampleUserDataHeader_SpecialMessageIndications() {
	var header tpdu.UserDataHeader
	_, err := header.UnmarshalBinary([]byte{8, 1, 2, 0, 4, 1, 2, 0x81, 2})
	if err != nil {
		fmt.Println(err)
		return
	}
	for _, indication := range header.SpecialMessageIndications() {
		fmt.Printf("type %d, messages %d, store %t\n", indication.Type, indication.Count, indication.Store)
	}
	// Output:
	// type 0, messages 4, store false
	// type 1, messages 2, store true
}
