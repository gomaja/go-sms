// SPDX-License-Identifier: MIT

package cbs_test

import (
	"fmt"

	"github.com/gomaja/go-sms/encoding/cbs"
)

func ExampleDCS_DecodeUSSD() {
	// Section 5, figure 5-1: eng, followed by UCS2 "Hi".
	text, lang, err := cbs.DCS(0x12).DecodeUSSD([]byte{0x23, 0x4c, 0x00, 0x48, 0x00, 0x69})
	if err != nil {
		panic(err)
	}
	fmt.Printf("%s: %s\n", lang, text)
	// Output: eng: Hi
}

func ExampleDCS_EncodeUSSD() {
	b, err := cbs.GSM7.EncodeUSSD("*100#", "")
	if err != nil {
		panic(err)
	}
	fmt.Printf("%x\n", b)
	// Output: aa180c3602
}
