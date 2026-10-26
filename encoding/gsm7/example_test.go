// SPDX-License-Identifier: MIT

package gsm7_test

import (
	"fmt"

	"github.com/gomaja/go-sms/encoding/gsm7"
)

func ExampleDecodeReport() {
	text, replacements, _ := gsm7.DecodeReport([]byte{0x41, 0x1b, 0, 0x1b, 0x1b, 0x42})
	fmt.Printf("%s\n", text)
	for _, r := range replacements {
		fmt.Printf("%d %02x %s %q\n", r.Offset, r.Septet, r.Rule, r.Rune)
	}
	// Output:
	// A@ B
	// 2 00 ExtensionFallback '@'
	// 4 1b ReservedEscape ' '
}
