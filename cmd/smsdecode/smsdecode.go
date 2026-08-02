// SPDX-License-Identifier: MIT

// smsdecode provides an example of unmarshalling and displaying a SMS TPDU.
package main

import (
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/gomaja/go-sms"
	"github.com/gomaja/go-sms/encoding/pdumode"
	"github.com/gomaja/go-sms/encoding/tpdu"
)

func main() {
	pm := flag.Bool("p", false, "PDU is prefixed with SCA (PDU mode)")
	orig := flag.Bool("o", false, "PDU is mobile originated")
	flag.Usage = usage
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(1)
	}
	tp, smsc, err := decode(flag.Arg(0), *pm, *orig)
	if err != nil {
		log.Fatal(err)
	}
	if smsc != nil {
		if err := dumpSMSC(os.Stdout, smsc); err != nil {
			log.Fatal(err)
		}
	}
	if err := dumpTPDU(os.Stdout, tp); err != nil {
		log.Fatal(err)
	}
}

func decode(s string, pm, mo bool) (p *tpdu.TPDU, a *pdumode.SMSCAddress, err error) {
	b, err := hex.DecodeString(s)
	if err != nil {
		return
	}
	if pm {
		var pdu *pdumode.PDU
		pdu, err = pdumode.UnmarshalHexString(s)
		if err != nil {
			return
		}
		a = &pdu.SMSC
		b = pdu.TPDU
	}
	if mo {
		p, err = sms.Unmarshal(b, sms.AsMO)
		return
	}
	p, err = sms.Unmarshal(b)
	return
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

func dumpTPDU(w io.Writer, t *tpdu.TPDU) error {
	d := dumper{w: w}
	var st string
	var dump func(d *dumper, t *tpdu.TPDU)
	switch t.SmsType() {
	case tpdu.SmsCommand:
		st = "SMS-COMMAND"
		dump = dumpCommand
	case tpdu.SmsDeliver:
		st = "SMS-DELIVER"
		dump = dumpDeliver
	case tpdu.SmsDeliverReport:
		st = "SMS-DELIVER-REPORT"
		dump = dumpDeliverReport
	case tpdu.SmsStatusReport:
		st = "SMS-STATUS-REPORT"
		dump = dumpStatusReport
	case tpdu.SmsSubmit:
		st = "SMS-SUBMIT"
		dump = dumpSubmit
	case tpdu.SmsSubmitReport:
		st = "SMS-SUBMIT-REPORT"
		dump = dumpSubmitReport
	}
	d.printf("TPDU: %s\n", st)
	dump(&d, t)
	return d.err
}

func dumpCommand(d *dumper, t *tpdu.TPDU) {
	d.printf("TP-MTI: 0x%02x %s\n", int(t.SmsType().MTI()), t.SmsType().MTI())
	d.printf("TP-UDHI: %t\n", t.FirstOctet.UDHI())
	d.printf("TP-SRR: %t\n", t.FirstOctet.SRR())
	d.printf("TP-MR: %d\n", t.MR)
	d.printf("TP-PID: 0x%02x\n", t.PID)
	d.printf("TP-CT: 0x%02x\n", t.CT)
	d.printf("TP-MN: %d\n", t.MN)
	d.printf("TP-DA: %s\n", t.DA.Number())
	d.printf("TP-SCTS: %s\n", t.SCTS)
	d.printf("TP-CDL: %d\n", len(t.UD))
	dumpCD(d, t.UD)
}

func dumpDeliver(d *dumper, t *tpdu.TPDU) {
	d.printf("TP-MTI: 0x%02x %s\n", int(t.SmsType().MTI()), t.SmsType().MTI())
	d.printf("TP-MMS: %t\n", t.FirstOctet.MMS())
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
	dumpUD(d, t.UD)
}

func dumpDeliverReport(d *dumper, t *tpdu.TPDU) {
	d.printf("TP-MTI: 0x%02x %s\n", int(t.SmsType().MTI()), t.SmsType().MTI())
	d.printf("TP-UDHI: %t\n", t.FirstOctet.UDHI())
	d.printf("TP-FCS: 0x%02x\n", t.FCS)
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
	dumpUD(d, t.UD)
}

func dumpStatusReport(d *dumper, t *tpdu.TPDU) {
	d.printf("TP-MTI: 0x%02x %s\n", int(t.SmsType().MTI()), t.SmsType().MTI())
	d.printf("TP-UDHI: %t\n", t.FirstOctet.UDHI())
	d.printf("TP-MMS: %t\n", t.FirstOctet.MMS())
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
	dumpUD(d, t.UD)
}

func dumpSubmit(d *dumper, t *tpdu.TPDU) {
	d.printf("TP-MTI: 0x%02x %s\n", int(t.SmsType().MTI()), t.SmsType().MTI())
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
	dumpUD(d, t.UD)
}

func dumpSubmitReport(d *dumper, t *tpdu.TPDU) {
	d.printf("TP-MTI: 0x%02x %s\n", int(t.SmsType().MTI()), t.SmsType().MTI())
	d.printf("TP-UDHI: %t\n", t.FirstOctet.UDHI())
	d.printf("TP-FCS: 0x%02x\n", t.FCS)
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
	dumpUD(d, t.UD)
}

func dumpCD(d *dumper, ud []byte) {
	lines := strings.Split(hex.Dump(ud), "\n")
	d.printf("TP-CD: %s\n", lines[0])
	for _, l := range lines[1:] {
		d.printf("       %s\n", l)
	}
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

func dumpUDH(d *dumper, udh tpdu.UserDataHeader) {
	ie := udh[0]
	d.printf("TP-UDH: ID: %d  Data: %v\n", ie.ID, ie.Data)
	for _, ie = range udh[1:] {
		d.printf("       ID: %d  Data: %v\n", ie.ID, ie.Data)
	}
}

func dumpUD(d *dumper, ud []byte) {
	lines := strings.Split(strings.TrimSpace(hex.Dump(ud)), "\n")
	d.printf("TP-UD: %s\n", lines[0])
	for _, l := range lines[1:] {
		d.printf("       %s\n", l)
	}
}

func usage() {
	_, _ = fmt.Fprintf(os.Stderr, "Usage: smsdecode [-p] [-o] <sms>\n")
	flag.PrintDefaults()
}
