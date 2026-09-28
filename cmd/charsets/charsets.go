// SPDX-License-Identifier: MIT

package main

import (
	"fmt"

	"github.com/gomaja/go-sms/encoding/gsm7/charset"
)

var charsetName = []string{
	"Basic (default)",
	"Turkish",
	"Spanish",
	"Portuguese",
	"Bengali",
	"Gujarati",
	"Hindi",
	"Kannada",
	"Malayalam",
	"Oriya",
	"Punjabi",
	"Tamil",
	"Telugu",
	"Urdu",
}

func main() {
	for nli := charset.Default; nli <= charset.Urdu; nli++ {
		fmt.Printf("%s Locking (NLI=%d)\n", charsetName[nli], nli)
		Display(charset.NewDecoder(nli))
		fmt.Println()
		fmt.Printf("%s Shift (NLI=%d)\n", charsetName[nli], nli)
		Display(charset.NewExtDecoder(nli))
		fmt.Println()
	}
}

// esc is the escape septet, which is not a character. In a locking shift
// table it "is an escape to an extension of this table", and in a single
// shift table it "is reserved for the extension to another extension table"
// (3GPP TS 23.038 V20.0.0 Section 6.2.1 Note 1 and Annex A.3, Section 6.2.1.1
// Note 1 and Annex A.2).
const esc = 0x1b

// Display prints the character set for a given character set decoder.
//
// The escape is shown at 0x1B whatever the decoder has there, as no decoder
// has an entry for it.
func Display(m charset.Decoder) {
	specials := map[rune]string{
		'\n':   "LF",
		'\r':   "CR",
		'\f':   "FF",
		' ':    "SP",
		0x20ac: " €",
	}
	fmt.Printf("      ")
	for c := 0; c < 8; c++ {
		fmt.Printf("0x%d_ ", c)
	}
	fmt.Println("")
	for r := 0; r < 0x10; r++ {
		fmt.Printf("0x_%x: ", r)
		for c := 0; c < 8; c++ {
			k := byte(c*0x10 + r)
			if k == esc {
				fmt.Printf("%3s  ", "ESC")
				continue
			}
			if v, ok := m[k]; ok {
				if s, ok := specials[v]; ok {
					fmt.Printf("%3s  ", s)
				} else if v >= 0x400 {
					fmt.Printf("%04x ", v)
				} else {
					fmt.Printf("  %c  ", v)
				}
			} else {
				fmt.Printf("     ")
			}
		}
		fmt.Println()
	}
}
