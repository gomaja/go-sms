// SPDX-License-Identifier: MIT

package main

import (
	"bufio"
	"fmt"
	"io"
	"os"

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
	os.Exit(run(os.Stdout, os.Stderr))
}

// run writes the character tables to stdout and returns 1 on a write error,
// reporting the error on stderr, or 0 on success.
func run(stdout, stderr io.Writer) int {
	w := bufio.NewWriter(stdout)
	for nli := charset.Default; nli <= charset.Urdu; nli++ {
		_, _ = fmt.Fprintf(w, "%s Locking (NLI=%d)\n", charsetName[nli], nli)
		if err := Display(w, charset.NewDecoder(nli)); err != nil {
			_, _ = fmt.Fprintf(stderr, "charsets: %v\n", err)
			return 1
		}
		_, _ = fmt.Fprintln(w)
		_, _ = fmt.Fprintf(w, "%s Shift (NLI=%d)\n", charsetName[nli], nli)
		if err := Display(w, charset.NewExtDecoder(nli)); err != nil {
			_, _ = fmt.Fprintf(stderr, "charsets: %v\n", err)
			return 1
		}
		_, _ = fmt.Fprintln(w)
	}
	if err := w.Flush(); err != nil {
		_, _ = fmt.Fprintf(stderr, "charsets: %v\n", err)
		return 1
	}
	return 0
}

// esc is the escape septet, which is not a character. In a locking shift
// table it "is an escape to an extension of this table", and in a single
// shift table it "is reserved for the extension to another extension table"
// (3GPP TS 23.038 V20.0.0 Section 6.2.1 Note 1 and Annex A.3, Section 6.2.1.1
// Note 1 and Annex A.2).
const esc = 0x1b

// Display writes the character set for a given decoder, returning the first
// write error.
//
// The escape is shown at 0x1B whatever the decoder has there, as no decoder
// has an entry for it.
func Display(stdout io.Writer, m charset.Decoder) error {
	w := bufio.NewWriter(stdout)
	specials := map[rune]string{
		'\n':   "LF",
		'\r':   "CR",
		'\f':   "FF",
		' ':    "SP",
		0x20ac: " €",
	}
	_, _ = fmt.Fprintf(w, "      ")
	for c := 0; c < 8; c++ {
		_, _ = fmt.Fprintf(w, "0x%d_ ", c)
	}
	_, _ = fmt.Fprintln(w)
	for r := 0; r < 0x10; r++ {
		_, _ = fmt.Fprintf(w, "0x_%x: ", r)
		for c := 0; c < 8; c++ {
			k := byte(c*0x10 + r)
			if k == esc {
				_, _ = fmt.Fprintf(w, "%3s  ", "ESC")
				continue
			}
			if v, ok := m[k]; ok {
				if s, ok := specials[v]; ok {
					_, _ = fmt.Fprintf(w, "%3s  ", s)
				} else if v >= 0x400 {
					_, _ = fmt.Fprintf(w, "%04x ", v)
				} else {
					_, _ = fmt.Fprintf(w, "  %c  ", v)
				}
			} else {
				_, _ = fmt.Fprintf(w, "     ")
			}
		}
		_, _ = fmt.Fprintln(w)
	}
	return w.Flush()
}
