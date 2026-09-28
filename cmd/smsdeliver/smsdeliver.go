// SPDX-License-Identifier: MIT

// smsdeliver provides an example of extracting a message from a set of
// SMS-DELIVER TPDUs.
package main

import (
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/gomaja/go-sms"
	"github.com/gomaja/go-sms/encoding/pdumode"
	"github.com/gomaja/go-sms/encoding/tpdu"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run decodes the TPDUs given by the arguments, writes each message they
// carry to stdout, and returns the exit status: 0 if every TPDU was decoded
// into a complete message, 1 if not, and 2 if the arguments are not valid.
func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("smsdeliver", flag.ContinueOnError)
	fs.SetOutput(stderr)
	pm := fs.Bool("p", false, "PDU is prefixed with SCA (PDU mode)")
	fs.Usage = func() { usage(fs) }
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() == 0 {
		fs.Usage()
		return 2
	}
	status := 0
	fail := func(format string, args ...interface{}) {
		// A diagnostic that cannot be written can be reported nowhere else.
		_, _ = fmt.Fprintf(stderr, "smsdeliver: "+format+"\n", args...)
		status = 1
	}
	// Without a timeout the handler is only called by Close, from this
	// goroutine, with each message still incomplete.
	var incomplete [][]*tpdu.TPDU
	c := sms.NewCollector(
		sms.WithReassemblyTimeout(0),
		sms.WithExpiryHandler(func(segments []*tpdu.TPDU, _ error) {
			incomplete = append(incomplete, segments)
		}))
	for i, a := range fs.Args() {
		t, err := unmarshal(a, *pm)
		if err != nil {
			fail("TPDU %d: %v", i+1, err)
			continue
		}
		if st := t.SmsType(); st != tpdu.SmsDeliver {
			fail("TPDU %d: %s is not an SMS-DELIVER", i+1, st)
			continue
		}
		pdus, err := c.Collect(t)
		if err != nil {
			fail("TPDU %d: %v", i+1, err)
			continue
		}
		if pdus == nil {
			continue
		}
		msg, err := sms.Decode(pdus)
		if err != nil {
			fail("TPDU %d: %v", i+1, err)
			continue
		}
		if _, err := fmt.Fprintf(stdout, "%s: %s\n", pdus[0].OA.Number(), msg); err != nil {
			fail("%v", err)
		}
	}
	// Closing passes the incomplete messages to the expiry handler.
	c.Close()
	for _, segments := range incomplete {
		fail("%s", describeIncomplete(segments))
	}
	return status
}

// unmarshal decodes a TPDU from its hex form, which in PDU mode is prefixed
// by the SC address.
func unmarshal(s string, pm bool) (*tpdu.TPDU, error) {
	b, err := hex.DecodeString(s)
	if err != nil {
		return nil, err
	}
	if pm {
		pdu, err := pdumode.UnmarshalBinary(b)
		if err != nil {
			return nil, err
		}
		b = pdu.TPDU
	}
	return sms.Unmarshal(b)
}

// describeIncomplete describes the segments of an incomplete concatenated
// message, as given to the expiry handler.
func describeIncomplete(segments []*tpdu.TPDU) string {
	var have, missing []string
	var oa string
	ref := 0
	for i, s := range segments {
		if s == nil {
			missing = append(missing, fmt.Sprint(i+1))
			continue
		}
		have = append(have, fmt.Sprint(i+1))
		oa = s.OA.Number()
		ci, _ := s.ConcatInfo()
		ref = ci.Ref
	}
	return fmt.Sprintf("incomplete message from %s, reference %d: have segments %s of %d, missing %s",
		oa, ref, strings.Join(have, ", "), len(segments), strings.Join(missing, ", "))
}

func usage(fs *flag.FlagSet) {
	_, _ = fmt.Fprintf(fs.Output(), "smsdeliver decodes and displays the message from one or more SMS Deliver TPDUs.\n"+
		"Usage: smsdeliver [-p] <pdu> [pdu...]\n")
	fs.PrintDefaults()
}
