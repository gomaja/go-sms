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
