// SPDX-License-Identifier: MIT

// smsdecode provides an example of unmarshalling and displaying a SMS TPDU.
package main

import (
	"encoding/hex"
	"errors"
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

// run decodes the TPDU given by the arguments, writes its fields to stdout,
// and returns the exit status: 0 on success, 1 if the TPDU cannot be decoded,
// and 2 if the arguments are not valid.
func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("smsdecode", flag.ContinueOnError)
	fs.SetOutput(stderr)
	pm := fs.Bool("p", false, "PDU is prefixed with SCA (PDU mode)")
	mo := fs.Bool("o", false, "PDU is mobile originated")
	rpError := fs.Bool("e", false, "PDU is a report carried by an RP-ERROR, so has a TP-FCS")
	fs.Usage = func() { usage(fs) }
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return 2
	}
	d, err := decode(fs.Arg(0), *pm, *mo, *rpError)
	if err != nil {
		warnf(stderr, "%v", err)
		return 1
	}
	if d.smsc != nil {
		if err := dumpSMSC(stdout, d.smsc); err != nil {
			warnf(stderr, "%v", err)
			return 1
		}
	}
	if err := dumpTPDU(stdout, d.tpdu, d.raw); err != nil {
		warnf(stderr, "%v", err)
		return 1
	}
	return 0
}

// warnf writes a diagnostic to stderr, where a failed write can be reported
// nowhere else.
func warnf(stderr io.Writer, format string, args ...interface{}) {
	_, _ = fmt.Fprintf(stderr, "smsdecode: "+format+"\n", args...)
}

// decoded is a decoded TPDU, with its SC address in PDU mode, and its octets.
type decoded struct {
	tpdu *tpdu.TPDU
	smsc *pdumode.SMSCAddress
	raw  []byte
}

func decode(s string, pm, mo, rpError bool) (decoded, error) {
	var d decoded
	b, err := hex.DecodeString(s)
	if err != nil {
		return d, err
	}
	if pm {
		pdu, err := pdumode.UnmarshalBinary(b)
		if err != nil {
			return d, err
		}
		d.smsc = &pdu.SMSC
		b = pdu.TPDU
	}
	options := []sms.UnmarshalOption{}
	if mo {
		options = append(options, sms.AsMO)
	}
	if rpError {
		options = append(options, sms.AsRPError)
	}
	d.tpdu, err = sms.Unmarshal(b, options...)
	if err != nil {
		return decoded{}, err
	}
	d.raw = b
	return d, nil
}

type dumper struct {
	w   io.Writer
	err error
}

func (d *dumper) printf(format string, args ...interface{}) {
	if d.err != nil {
		return
	}
	_, d.err = fmt.Fprintf(d.w, format, args...)
}

func dumpSMSC(w io.Writer, smsc *pdumode.SMSCAddress) error {
	d := dumper{w: w}
	n := smsc.Number()
	d.printf("SMSC: %s\n", n)
	return d.err
}

// typeNames are the names of the TPDU types in 3GPP TS 23.040 Section 9.2.2.
var typeNames = map[tpdu.SmsType]string{
	tpdu.SmsDeliver:       "SMS-DELIVER",
	tpdu.SmsDeliverReport: "SMS-DELIVER-REPORT",
	tpdu.SmsSubmit:        "SMS-SUBMIT",
	tpdu.SmsSubmitReport:  "SMS-SUBMIT-REPORT",
	tpdu.SmsStatusReport:  "SMS-STATUS-REPORT",
	tpdu.SmsCommand:       "SMS-COMMAND",
}

// errUnsupportedType indicates a TPDU of no type smsdecode can display.
var errUnsupportedType = errors.New("unsupported TPDU type")

// dumpTPDU writes the fields of the TPDU, whose octets are raw, to w.
func dumpTPDU(w io.Writer, t *tpdu.TPDU, raw []byte) error {
	d := dumper{w: w}
	st := t.SmsType()
	name, ok := typeNames[st]
	if !ok {
		return fmt.Errorf("%w: %s", errUnsupportedType, st)
	}
	d.printf("TPDU: %s\n", name)
	// The TP-MTI means a type in each direction, as 3GPP TS 23.040 Section
	// 9.2.3.1 defines, and an MS processes a Reserved TP-MTI "as if it were
	// an SMS-DELIVER".
	mti := t.FirstOctet.MTI()
	if mti == tpdu.MtReserved {
		name = "Reserved, processed as " + name
	}
	d.printf("TP-MTI: 0x%02x %s\n", int(mti), name)
	switch st {
	case tpdu.SmsCommand:
		dumpCommand(&d, t, raw)
	case tpdu.SmsDeliver:
		dumpDeliver(&d, t)
	case tpdu.SmsDeliverReport:
		dumpDeliverReport(&d, t)
	case tpdu.SmsStatusReport:
		dumpStatusReport(&d, t)
	case tpdu.SmsSubmit:
		dumpSubmit(&d, t)
	case tpdu.SmsSubmitReport:
		dumpSubmitReport(&d, t)
	}
	return d.err
}

func dumpCommand(d *dumper, t *tpdu.TPDU, raw []byte) {
	d.printf("TP-UDHI: %t\n", t.FirstOctet.UDHI())
	d.printf("TP-SRR: %t\n", t.FirstOctet.SRR())
	d.printf("TP-MR: %d\n", t.MR)
	d.printf("TP-PID: 0x%02x\n", t.PID)
	d.printf("TP-CT: 0x%02x\n", t.CT)
	d.printf("TP-MN: %d\n", t.MN)
	d.printf("TP-DA: %s\n", t.DA.Number())
	d.printf("TP-CDL: %d\n", commandDataLength(t, raw))
	if t.UDH != nil {
		dumpUDH(d, t.UDH)
	}
	dumpHex(d, "TP-CD", t.UD)
}

// commandDataLength returns the TP-CDL of an SMS-COMMAND, which counts the
// octets of the TP-CD, including any header, as 3GPP TS 23.040 Section
// 9.2.3.20 defines.
//
// It is read from the octets, which follow the TP-DA, whose length octet
// counts its semi-octets (Section 9.1.2.5), as the UDH does not hold the
// length of a header that was ignored when decoded.
func commandDataLength(t *tpdu.TPDU, raw []byte) int {
	const daOffset = 5
	if len(raw) > daOffset {
		if cdl := daOffset + 2 + (int(raw[daOffset])+1)/2; cdl < len(raw) {
			return int(raw[cdl])
		}
	}
	n := len(t.UD)
	if t.UDH != nil {
		n += 1 + t.UDH.UDHL()
	}
	return n
}

// dumpMMS writes the TP-MMS, whose bit set means no more messages are
// waiting, as 3GPP TS 23.040 Section 9.2.3.2 defines.
func dumpMMS(d *dumper, t *tpdu.TPDU) {
	if t.FirstOctet.MMS() {
		d.printf("TP-MMS: 0x01 No more messages are waiting\n")
	} else {
		d.printf("TP-MMS: 0x00 More messages are waiting\n")
	}
}

// reportUnused masks the bits of the first octet of an SMS-DELIVER-REPORT
// or SMS-SUBMIT-REPORT that are "presently unused": 7 and 5 to 2 (3GPP TS
// 23.040 Sections 9.2.2.1a and 9.2.2.2a).
const reportUnused tpdu.FirstOctet = 0xbc

// dumpFCS writes the TP-FCS of a report, which only one carried by an
// RP-ERROR has, and reports whether the fields after it are to be shown.
//
// If any of the unused bits of the first octet is set then "the receiver
// shall not examine the other field and shall treat the TP-Failure-Cause as
// "Unspecified error cause"" (3GPP TS 23.040 Sections 9.2.2.1a and 9.2.2.2a),
// so the failure cause is shown as the receiver takes it, and the fields after
// it are not shown.
func dumpFCS(d *dumper, t *tpdu.TPDU) bool {
	if t.RPMessage != tpdu.RPError {
		return true
	}
	if t.FirstOctet&reportUnused != 0 {
		d.printf("TP-FCS: 0x%02x Unspecified error cause (first octet has unused bits set, TP-FCS received as 0x%02x)\n",
			t.FailureCause(), t.FCS)
		return false
	}
	d.printf("TP-FCS: 0x%02x\n", t.FCS)
	return true
}

func dumpDeliver(d *dumper, t *tpdu.TPDU) {
	dumpMMS(d, t)
	d.printf("TP-LP: %t\n", t.FirstOctet.LP())
	d.printf("TP-RP: %t\n", t.FirstOctet.RP())
	d.printf("TP-UDHI: %t\n", t.FirstOctet.UDHI())
	d.printf("TP-SRI: %t\n", t.FirstOctet.SRI())
	d.printf("TP-OA: %s\n", t.OA.Number())
	d.printf("TP-PID: 0x%02x\n", t.PID)
	d.printf("TP-DCS: %s\n", t.DCS)
	d.printf("TP-SCTS: %s\n", t.SCTS)
	if t.UDH != nil {
		dumpUDH(d, t.UDH)
	}
	dumpHex(d, "TP-UD", t.UD)
}

func dumpDeliverReport(d *dumper, t *tpdu.TPDU) {
	d.printf("TP-UDHI: %t\n", t.FirstOctet.UDHI())
	if !dumpFCS(d, t) {
		return
	}
	d.printf("TP-PI: %s\n", t.PI)
	if t.PI.PID() {
		d.printf("TP-PID: 0x%02x\n", t.PID)
	}
	if t.PI.DCS() {
		d.printf("TP-DCS: %s\n", t.DCS)
	}
	if t.UDH != nil {
		dumpUDH(d, t.UDH)
	}
	dumpHex(d, "TP-UD", t.UD)
}

func dumpStatusReport(d *dumper, t *tpdu.TPDU) {
	d.printf("TP-UDHI: %t\n", t.FirstOctet.UDHI())
	dumpMMS(d, t)
	d.printf("TP-LP: %t\n", t.FirstOctet.LP())
	d.printf("TP-SRQ: %t\n", t.FirstOctet.SRQ())
	d.printf("TP-MR: %d\n", t.MR)
	d.printf("TP-RA: %s\n", t.RA.Number())
	d.printf("TP-SCTS: %s\n", t.SCTS)
	d.printf("TP-DT: %s\n", t.DT)
	d.printf("TP-ST: 0x%02x\n", t.ST)
	d.printf("TP-PI: %s\n", t.PI)
	if t.PI.PID() {
		d.printf("TP-PID: 0x%02x\n", t.PID)
	}
	if t.PI.DCS() {
		d.printf("TP-DCS: %s\n", t.DCS)
	}
	if t.UDH != nil {
		dumpUDH(d, t.UDH)
	}
	dumpHex(d, "TP-UD", t.UD)
}

func dumpSubmit(d *dumper, t *tpdu.TPDU) {
	d.printf("TP-RD: %t\n", t.FirstOctet.RD())
	d.printf("TP-VPF: 0x%02x %s\n", int(t.FirstOctet.VPF()), t.FirstOctet.VPF())
	d.printf("TP-RP: %t\n", t.FirstOctet.RP())
	d.printf("TP-UDHI: %t\n", t.FirstOctet.UDHI())
	d.printf("TP-SRR: %t\n", t.FirstOctet.SRR())
	d.printf("TP-MR: %d\n", t.MR)
	d.printf("TP-DA: %s\n", t.DA.Number())
	d.printf("TP-PID: 0x%02x\n", t.PID)
	d.printf("TP-DCS: %s\n", t.DCS)
	dumpVP(d, t.VP)
	if t.UDH != nil {
		dumpUDH(d, t.UDH)
	}
	dumpHex(d, "TP-UD", t.UD)
}

func dumpSubmitReport(d *dumper, t *tpdu.TPDU) {
	d.printf("TP-UDHI: %t\n", t.FirstOctet.UDHI())
	if !dumpFCS(d, t) {
		return
	}
	d.printf("TP-PI: %s\n", t.PI)
	d.printf("TP-SCTS: %s\n", t.SCTS)
	if t.PI.PID() {
		d.printf("TP-PID: 0x%02x\n", t.PID)
	}
	if t.PI.DCS() {
		d.printf("TP-DCS: %s\n", t.DCS)
	}
	if t.UDH != nil {
		dumpUDH(d, t.UDH)
	}
	dumpHex(d, "TP-UD", t.UD)
}

func dumpVP(d *dumper, vp tpdu.ValidityPeriod) {
	switch vp.Format {
	case tpdu.VpfNotPresent:
		d.printf("TP-VP: Not Present\n")
	case tpdu.VpfAbsolute:
		d.printf("TP-VP: Absolute - %s\n", vp.Time)
	case tpdu.VpfEnhanced:
		d.printf("TP-VP: Enhanced %s - %s\n",
			tpdu.EnhancedFormat(vp.EFI), vp.Duration)
	case tpdu.VpfRelative:
		d.printf("TP-VP: Relative - %s\n", vp.Duration)
	}
}

// dumpUDH writes the IEs of the UDH, which may have none, when its UDHL is 0
// or it was ignored as malformed.
func dumpUDH(d *dumper, udh tpdu.UserDataHeader) {
	if len(udh) == 0 {
		d.printf("TP-UDH: no IE\n")
		return
	}
	for i, ie := range udh {
		label := "TP-UDH:"
		if i > 0 {
			label = "       "
		}
		d.printf("%s ID: %d  Data: %v\n", label, ie.ID, ie.Data)
	}
}

// dumpHex writes the octets as a hex dump, labelled on its first line.
func dumpHex(d *dumper, label string, b []byte) {
	lines := strings.Split(strings.TrimSuffix(hex.Dump(b), "\n"), "\n")
	d.printf("%s: %s\n", label, lines[0])
	indent := strings.Repeat(" ", len(label)+2)
	for _, l := range lines[1:] {
		d.printf("%s%s\n", indent, l)
	}
}

func usage(fs *flag.FlagSet) {
	_, _ = fmt.Fprintf(fs.Output(), "Usage: smsdecode [-p] [-o] [-e] <sms>\n")
	fs.PrintDefaults()
}
