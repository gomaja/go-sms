// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/gomaja/go-sms/encoding/pdumode"
	"github.com/gomaja/go-sms/encoding/tpdu"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runMainEnv makes the test binary run main, so tests can run the tool as
// a process and check its exit status.
const runMainEnv = "GO_SMS_RUN_MAIN"

func TestMain(m *testing.M) {
	if os.Getenv(runMainEnv) == "1" {
		main()
	}
	os.Exit(m.Run())
}

type errWriter struct {
	err error
}

func (w errWriter) Write([]byte) (int, error) {
	return 0, w.err
}

func TestDecode(t *testing.T) {
	patterns := []struct {
		name string
		pdu  string
		pm   bool
		mo   bool
		out  *tpdu.TPDU
		err  error
	}{
		{
			"invalid hex",
			"bad hex",
			false,
			false,
			nil,
			hex.InvalidByteError(' '),
		},
		{
			"decode fail",
			"010005912143f500000bc8329bfd06dddf7236",
			false,
			true,
			nil,
			tpdu.DecodeError{
				Field:  "SmsSubmit.ud.sm",
				Offset: 10,
				Err:    tpdu.ErrUnderflow,
			},
		},
		{
			"pdumode decode fail",
			"07911614220991",
			true,
			true,
			nil,
			tpdu.DecodeError{
				Field:  "addr",
				Offset: 2,
				Err:    tpdu.ErrUnderflow,
			},
		},
		{
			// 3GPP TS 23.040 Section 9.2.2.1: "Any unused bits shall be set
			// to zero by the sending entity and shall be ignored by the
			// receiving entity."
			"ignore non-zero SMS 7-bit spare septet",
			"07912180958739F144038102F100001211304113338A2F0608041E9602026177F85C06E5DF7539283C1EBFEB6E3A284C0785E974D7F8DD7EB5F37079191E4E9341",
			true,
			false,
			&tpdu.TPDU{
				FirstOctet: 0x44,
				OA:         tpdu.Address{TOA: 0x81, Addr: "201"},
				SCTS: tpdu.Timestamp{
					Time: time.Date(2021, time.November, 3, 14, 31, 33, 0,
						time.FixedZone("SCTS", -7*3600)),
				},
				UDH: tpdu.UserDataHeader{
					{ID: 0x08, Data: []byte{0x1e, 0x96, 0x02, 0x02}},
				},
				UD: []byte("anage your account at att.com/myprepaid"),
			},
			nil,
		},
		{
			"submit",
			"010005912143f500000bc8329bfd06dddf723619",
			false,
			true,
			&tpdu.TPDU{
				Direction:  1,
				FirstOctet: 1,
				DA:         tpdu.Address{TOA: 0x91, Addr: "12345"},
				UD: []byte{
					0x48, 0x65, 0x6c, 0x6c, 0x6f, 0x20, 0x77, 0x6f, 0x72, 0x6c, 0x64,
				},
			},
			nil,
		},
		{
			"submit pdumode",
			"07911614220991f1010005912143f500000bc8329bfd06dddf723619",
			true,
			true,
			&tpdu.TPDU{
				Direction:  1,
				FirstOctet: 1,
				DA:         tpdu.Address{TOA: 0x91, Addr: "12345"},
				UD: []byte{
					0x48, 0x65, 0x6c, 0x6c, 0x6f, 0x20, 0x77, 0x6f, 0x72, 0x6c, 0x64,
				},
			},
			nil,
		},
	}

	for _, p := range patterns {
		f := func(t *testing.T) {
			d, err := decode(p.pdu, p.pm, p.mo, false)
			assert.Equal(t, p.err, err)
			assert.Equal(t, p.out, d.tpdu)
		}
		t.Run(p.name, f)
	}
}

func TestDecodeRPError(t *testing.T) {
	d, err := decode("00d000", false, true, true)
	require.NoError(t, err)
	assert.Equal(t, tpdu.SmsDeliverReport, d.tpdu.SmsType())
	assert.Equal(t, tpdu.RPError, d.tpdu.RPMessage)
	assert.Equal(t, byte(0xd0), d.tpdu.FCS)
	assert.Equal(t, []byte{0x00, 0xd0, 0x00}, d.raw)
}

func TestDumpSMSCReturnsWriteError(t *testing.T) {
	errWrite := errors.New("write failed")
	err := dumpSMSC(errWriter{err: errWrite}, &pdumode.SMSCAddress{})
	assert.True(t, errors.Is(err, errWrite))
}

func TestDumpTPDUReturnsWriteError(t *testing.T) {
	errWrite := errors.New("write failed")
	err := dumpTPDU(errWriter{err: errWrite}, &tpdu.TPDU{Direction: tpdu.MO, FirstOctet: tpdu.FirstOctet(tpdu.MtSubmit)}, nil)
	assert.True(t, errors.Is(err, errWrite))
}

// A failure to write the fields is reported and fails the run.
func TestRunWriteError(t *testing.T) {
	var stderr bytes.Buffer
	status := run([]string{"-p", "07911614220991F1040B911605935713F200008140806113912304D7F79B0E"},
		errWriter{err: errors.New("write failed")}, &stderr)
	assert.Equal(t, 1, status)
	assert.Equal(t, "smsdecode: write failed\n", stderr.String())
	stderr.Reset()
	status = run([]string{"44049121430000628092100000000100"}, errWriter{err: errors.New("write failed")}, &stderr)
	assert.Equal(t, 1, status)
	assert.Equal(t, "smsdecode: write failed\n", stderr.String())
	// only the SC address fails to be written
	stderr.Reset()
	var stdout bytes.Buffer
	status = run([]string{"-p", "07911614220991F1040B911605935713F200008140806113912304D7F79B0E"},
		&firstWriteFails{w: &stdout}, &stderr)
	assert.Equal(t, 1, status)
	assert.Equal(t, "smsdecode: write failed\n", stderr.String())
	assert.Empty(t, stdout.String())
}

// firstWriteFails fails its first write, and passes the others to w.
type firstWriteFails struct {
	w      io.Writer
	failed bool
}

func (f *firstWriteFails) Write(b []byte) (int, error) {
	if !f.failed {
		f.failed = true
		return 0, errors.New("write failed")
	}
	return f.w.Write(b)
}

func TestDumpTPDUUnsupportedType(t *testing.T) {
	var b bytes.Buffer
	err := dumpTPDU(&b, &tpdu.TPDU{Direction: tpdu.MO, FirstOctet: tpdu.FirstOctet(tpdu.MtReserved)}, nil)
	assert.True(t, errors.Is(err, errUnsupportedType))
	assert.Empty(t, b.String())
}

// lines joins the lines of an expected output.
func lines(l ...string) string {
	return strings.Join(l, "\n") + "\n"
}

func TestRun(t *testing.T) {
	patterns := []struct {
		name   string
		args   []string
		status int
		stdout string
		stderr string
	}{
		{
			"submit",
			[]string{"-o", "010105912143f500000bc8329bfd06dddf723619"},
			0,
			lines(
				"TPDU: SMS-SUBMIT",
				"TP-MTI: 0x01 SMS-SUBMIT",
				"TP-RD: false",
				"TP-VPF: 0x00 Not Present",
				"TP-RP: false",
				"TP-UDHI: false",
				"TP-SRR: false",
				"TP-MR: 1",
				"TP-DA: +12345",
				"TP-PID: 0x00",
				"TP-DCS: 0x00 7bit",
				"TP-VP: Not Present",
				"TP-UD: 00000000  48 65 6c 6c 6f 20 77 6f  72 6c 64                 |Hello world|",
			),
			"",
		},
		{
			// 3GPP TS 23.040 Section 9.2.2.1a: if any of bits 7 and 5 - 2 of
			// octet 1 is non-zero "the receiver shall not examine the other
			// field and shall treat the TP-Failure-Cause as "Unspecified
			// error cause"", so the fields after the TP-FCS are not shown.
			"deliver report rp-error unused bits",
			[]string{"-o", "-e", "04d007"},
			0,
			lines(
				"TPDU: SMS-DELIVER-REPORT",
				"TP-MTI: 0x00 SMS-DELIVER-REPORT",
				"TP-UDHI: false",
				"TP-FCS: 0xff Unspecified error cause (first octet has unused bits set, TP-FCS received as 0xd0)",
			),
			"",
		},
		{
			"deliver report rp-error unused bit 7",
			[]string{"-o", "-e", "80d0"},
			0,
			lines(
				"TPDU: SMS-DELIVER-REPORT",
				"TP-MTI: 0x00 SMS-DELIVER-REPORT",
				"TP-UDHI: false",
				"TP-FCS: 0xff Unspecified error cause (first octet has unused bits set, TP-FCS received as 0xd0)",
			),
			"",
		},
		{
			"submit report rp-error unused bits",
			[]string{"-e", "21d0"},
			0,
			lines(
				"TPDU: SMS-SUBMIT-REPORT",
				"TP-MTI: 0x01 SMS-SUBMIT-REPORT",
				"TP-UDHI: false",
				"TP-FCS: 0xff Unspecified error cause (first octet has unused bits set, TP-FCS received as 0xd0)",
			),
			"",
		},
		{
			// TP-MMS 1: "No more messages are waiting for the MS in this SC"
			// (3GPP TS 23.040 Section 9.2.3.2).
			"deliver pdu mode",
			[]string{"-p", "07911614220991F1040B911605935713F200008140806113912304D7F79B0E"},
			0,
			lines(
				"SMSC: +61412290191",
				"TPDU: SMS-DELIVER",
				"TP-MTI: 0x00 SMS-DELIVER",
				"TP-MMS: 0x01 No more messages are waiting",
				"TP-LP: false",
				"TP-RP: false",
				"TP-UDHI: false",
				"TP-SRI: false",
				"TP-OA: +61503975312",
				"TP-PID: 0x00",
				"TP-DCS: 0x00 7bit",
				"TP-SCTS: 2018-04-08 16:31:19 +0800",
				"TP-UD: 00000000  57 6f 6f 74                                       |Woot|",
			),
			"",
		},
		{
			"deliver more messages waiting, with udh",
			[]string{"400491214300046280921000000008050003090201ff00"},
			0,
			lines(
				"TPDU: SMS-DELIVER",
				"TP-MTI: 0x00 SMS-DELIVER",
				"TP-MMS: 0x00 More messages are waiting",
				"TP-LP: false",
				"TP-RP: false",
				"TP-UDHI: true",
				"TP-SRI: false",
				"TP-OA: +1234",
				"TP-PID: 0x00",
				"TP-DCS: 0x04 8bit",
				"TP-SCTS: 2026-08-29 01:00:00 +0000",
				"TP-UDH: ID: 0  Data: [9 2 1]",
				"TP-UD: 00000000  ff 00                                             |..|",
			),
			"",
		},
		{
			"deliver udh with two IEs",
			[]string{"40049121430004628092100000000a070003090201050001ff"},
			0,
			lines(
				"TPDU: SMS-DELIVER",
				"TP-MTI: 0x00 SMS-DELIVER",
				"TP-MMS: 0x00 More messages are waiting",
				"TP-LP: false",
				"TP-RP: false",
				"TP-UDHI: true",
				"TP-SRI: false",
				"TP-OA: +1234",
				"TP-PID: 0x00",
				"TP-DCS: 0x04 8bit",
				"TP-SCTS: 2026-08-29 01:00:00 +0000",
				"TP-UDH: ID: 0  Data: [9 2 1]",
				"        ID: 5  Data: []",
				"TP-UD: 00000000  01 ff                                             |..|",
			),
			"",
		},
		{
			// A UDHI with a UDHL of 0 is an empty UDH, which was dumped as
			// "index out of range [0] with length 0".
			"deliver empty udh",
			[]string{"44049121430000628092100000000100"},
			0,
			lines(
				"TPDU: SMS-DELIVER",
				"TP-MTI: 0x00 SMS-DELIVER",
				"TP-MMS: 0x01 No more messages are waiting",
				"TP-LP: false",
				"TP-RP: false",
				"TP-UDHI: true",
				"TP-SRI: false",
				"TP-OA: +1234",
				"TP-PID: 0x00",
				"TP-DCS: 0x00 7bit",
				"TP-SCTS: 2026-08-29 01:00:00 +0000",
				"TP-UDH: no IE",
				"TP-UD: ",
			),
			"",
		},
		{
			// 3GPP TS 23.040 Section 9.2.3.1: an MS processes a Reserved
			// TP-MTI "as if it were an SMS-DELIVER".
			"reserved mti",
			[]string{"03049121430000628092100000000100"},
			0,
			lines(
				"TPDU: SMS-DELIVER",
				"TP-MTI: 0x03 Reserved, processed as SMS-DELIVER",
				"TP-MMS: 0x00 More messages are waiting",
				"TP-LP: false",
				"TP-RP: false",
				"TP-UDHI: false",
				"TP-SRI: false",
				"TP-OA: +1234",
				"TP-PID: 0x00",
				"TP-DCS: 0x00 7bit",
				"TP-SCTS: 2026-08-29 01:00:00 +0000",
				"TP-UD: 00000000  00                                                |.|",
			),
			"",
		},
		{
			// SMS-COMMAND has no TP-SCTS (3GPP TS 23.040 Section 9.2.2.4).
			"command",
			[]string{"-o", "020100000505912143f503010203"},
			0,
			lines(
				"TPDU: SMS-COMMAND",
				"TP-MTI: 0x02 SMS-COMMAND",
				"TP-UDHI: false",
				"TP-SRR: false",
				"TP-MR: 1",
				"TP-PID: 0x00",
				"TP-CT: 0x00",
				"TP-MN: 5",
				"TP-DA: +12345",
				"TP-CDL: 3",
				"TP-CD: 00000000  01 02 03                                          |...|",
			),
			"",
		},
		{
			// The TP-CDL counts the header (3GPP TS 23.040 Section 9.2.3.20).
			"command with udh",
			[]string{"-o", "420100000505912143f5080500030702016162"},
			0,
			lines(
				"TPDU: SMS-COMMAND",
				"TP-MTI: 0x02 SMS-COMMAND",
				"TP-UDHI: true",
				"TP-SRR: false",
				"TP-MR: 1",
				"TP-PID: 0x00",
				"TP-CT: 0x00",
				"TP-MN: 5",
				"TP-DA: +12345",
				"TP-CDL: 8",
				"TP-UDH: ID: 0  Data: [7 2 1]",
				"TP-CD: 00000000  61 62                                             |ab|",
			),
			"",
		},
		{
			// A malformed header is ignored, but still counted.
			"command with ignored udh",
			[]string{"-o", "420100000505912143f506040003070261"},
			0,
			lines(
				"TPDU: SMS-COMMAND",
				"TP-MTI: 0x02 SMS-COMMAND",
				"TP-UDHI: true",
				"TP-SRR: false",
				"TP-MR: 1",
				"TP-PID: 0x00",
				"TP-CT: 0x00",
				"TP-MN: 5",
				"TP-DA: +12345",
				"TP-CDL: 6",
				"TP-UDH: no IE",
				"TP-CD: 00000000  61                                                |a|",
			),
			"",
		},
		{
			// The TP-MTI 2 of an MT TPDU is an SMS-STATUS-REPORT.
			"status report",
			[]string{"060105912143f5101010000000001010100000000000"},
			0,
			lines(
				"TPDU: SMS-STATUS-REPORT",
				"TP-MTI: 0x02 SMS-STATUS-REPORT",
				"TP-UDHI: false",
				"TP-MMS: 0x01 No more messages are waiting",
				"TP-LP: false",
				"TP-SRQ: false",
				"TP-MR: 1",
				"TP-RA: +12345",
				"TP-SCTS: 2001-01-01 00:00:00 +0000",
				"TP-DT: 2001-01-01 00:00:00 +0000",
				"TP-ST: 0x00",
				"TP-PI: 0",
				"TP-UD: ",
			),
			"",
		},
		{
			// A report carried by an RP-ACK has no TP-FCS.
			"deliver report rp-ack",
			[]string{"-o", "0000"},
			0,
			lines(
				"TPDU: SMS-DELIVER-REPORT",
				"TP-MTI: 0x00 SMS-DELIVER-REPORT",
				"TP-UDHI: false",
				"TP-PI: 0",
				"TP-UD: ",
			),
			"",
		},
		{
			"deliver report rp-error",
			[]string{"-o", "-e", "00d000"},
			0,
			lines(
				"TPDU: SMS-DELIVER-REPORT",
				"TP-MTI: 0x00 SMS-DELIVER-REPORT",
				"TP-UDHI: false",
				"TP-FCS: 0xd0",
				"TP-PI: 0",
				"TP-UD: ",
			),
			"",
		},
		{
			"submit report rp-ack",
			[]string{"010010101000000000"},
			0,
			lines(
				"TPDU: SMS-SUBMIT-REPORT",
				"TP-MTI: 0x01 SMS-SUBMIT-REPORT",
				"TP-UDHI: false",
				"TP-PI: 0",
				"TP-SCTS: 2001-01-01 00:00:00 +0000",
				"TP-UD: ",
			),
			"",
		},
		{
			"submit report rp-error",
			[]string{"-e", "01d00010101000000000"},
			0,
			lines(
				"TPDU: SMS-SUBMIT-REPORT",
				"TP-MTI: 0x01 SMS-SUBMIT-REPORT",
				"TP-UDHI: false",
				"TP-FCS: 0xd0",
				"TP-PI: 0",
				"TP-SCTS: 2001-01-01 00:00:00 +0000",
				"TP-UD: ",
			),
			"",
		},
		{"bad hex", []string{"zz"}, 1, "", "smsdecode: encoding/hex: invalid byte: U+007A 'z'\n"},
		{"underflow", []string{"00"}, 1, "", "smsdecode: tpdu: error decoding SmsDeliver.oa.addr at octet 1: underflow\n"},
		{"reserved mti mo", []string{"-o", "0300"}, 1, "", "unsupported SMS type: 0x7"},
		{"no pdu", nil, 2, "", "Usage: smsdecode"},
		{"two pdus", []string{"00", "00"}, 2, "", "Usage: smsdecode"},
		{"unknown flag", []string{"-x", "00"}, 2, "", "flag provided but not defined"},
	}
	for _, p := range patterns {
		f := func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			status := run(p.args, &stdout, &stderr)
			assert.Equal(t, p.status, status)
			assert.Equal(t, p.stdout, stdout.String())
			if p.stderr == "" {
				assert.Empty(t, stderr.String())
			} else {
				assert.Contains(t, stderr.String(), p.stderr)
			}
		}
		t.Run(p.name, f)
	}
}

// The tool as a process: its exit status and output.
func TestMainProcess(t *testing.T) {
	if testing.Short() {
		t.Skip("runs the test binary")
	}
	patterns := []struct {
		args   []string
		status int
		stdout string
		stderr string
	}{
		{[]string{"-o", "010105912143f500000bc8329bfd06dddf723619"}, 0, "TPDU: SMS-SUBMIT\n", ""},
		{[]string{"44049121430000628092100000000100"}, 0, "TPDU: SMS-DELIVER\n", ""},
		{[]string{"zz"}, 1, "", "invalid byte"},
		{nil, 2, "", "Usage: smsdecode"},
	}
	for _, p := range patterns {
		cmd := exec.Command(os.Args[0], p.args...)
		cmd.Env = append(os.Environ(), runMainEnv+"=1")
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()
		status := 0
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			status = ee.ExitCode()
		} else {
			require.NoError(t, err)
		}
		assert.Equal(t, p.status, status, "%v", p.args)
		assert.True(t, strings.HasPrefix(stdout.String(), p.stdout), "%v: %q", p.args, stdout.String())
		assert.Contains(t, stderr.String(), p.stderr, "%v", p.args)
	}
}
