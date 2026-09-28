// SPDX-License-Identifier: MIT

// smssubmit provides an example of encoding a message into a set of SMS-SUBMIT
// TPDUs.
package main

import (
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/gomaja/go-sms"
	"github.com/gomaja/go-sms/encoding/gsm7/charset"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run encodes the message given by the arguments, writes the TPDUs to stdout,
// and returns the exit status: 0 on success, 1 if the message cannot be
// encoded, and 2 if the arguments are not valid.
func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("smssubmit", flag.ContinueOnError)
	fs.SetOutput(stderr)
	number := fs.String("number", "", "Destination number, which is international if it starts with '+'")
	msg := fs.String("message", "", "The message to encode")
	nli := fs.Int("language", 0, fmt.Sprintf(
		"The NLI of a character set to use in addition to the default, from %d to %d",
		charset.Start, charset.End-1))
	fs.Usage = func() { usage(fs) }
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *number == "" || *msg == "" || fs.NArg() != 0 {
		fs.Usage()
		return 2
	}
	if *nli != 0 && (*nli < charset.Start || *nli >= charset.End) {
		warnf(stderr, "invalid -language %d: the NLI must be from %d to %d",
			*nli, charset.Start, charset.End-1)
		return 2
	}
	options := []sms.EncoderOption{sms.To(*number)}
	if *nli != 0 {
		options = append(options, sms.WithCharset(*nli))
	}
	pdus, err := sms.Encode([]byte(*msg), options...)
	if err != nil {
		warnf(stderr, "%v", err)
		return 1
	}
	// Marshal every TPDU before writing any, so that a failure writes none.
	bins := make([][]byte, len(pdus))
	for i := range pdus {
		if bins[i], err = pdus[i].MarshalBinary(); err != nil {
			warnf(stderr, "%v", err)
			return 1
		}
	}
	for i, b := range bins {
		label := "Submit TPDU:"
		if len(bins) > 1 {
			label = fmt.Sprintf("Submit TPDU %d:", i+1)
		}
		if _, err := fmt.Fprintf(stdout, "%s\n%s\n", label, hex.EncodeToString(b)); err != nil {
			warnf(stderr, "%v", err)
			return 1
		}
	}
	return 0
}

// warnf writes a diagnostic to stderr, where a failed write can be reported
// nowhere else.
func warnf(stderr io.Writer, format string, args ...interface{}) {
	_, _ = fmt.Fprintf(stderr, "smssubmit: "+format+"\n", args...)
}

func usage(fs *flag.FlagSet) {
	_, _ = fmt.Fprintf(fs.Output(), "smssubmit encodes a message into a SMS Submit TPDU.\n"+
		"The message is encoded using the GSM7 default alphabet, or if necessary\n"+
		"an optionally specified character set, or failing those as UCS-2.\n"+
		"If the message is too long for a single PDU then it is split into several.\n\n"+
		"Usage: smssubmit -number <number> -message <message> [-language <nli>]\n")
	fs.PrintDefaults()
}
