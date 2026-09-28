// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"testing"

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

// The TPDUs of the README: a single SMS-DELIVER in PDU mode, and the three
// segments of a concatenated one.
const (
	woot = "07911614220991F1040B911605935713F200008140806113912304D7F79B0E"
	seg1 = "400B911605935713F20008814080611373238C050003C00301007400680069007300200069007300200061002000760065007200790020006C006F006E00670020006D0065007300730061006700650020007400680061007400200064006F006500730020006E006F0074002000660069007400200069006E00200061002000730069006E0067006C006500200053004D00530020006D0065007300730061"
	seg2 = "400B911605935713F20008814080611373238C050003C0030200670065002C0020006100740020006C0065006100730074002000690074002000770069006C006C002000690066002000490020006B00650065007000200061006400640069006E00670020006D006F0072006500200074006F0020006900740020006100730020003100360030002000630068006100720061006300740065007200730020"
	seg3 = "440B911605935713F200088140806113832344050003C00303006900730020006D006F007200650020007400680061006E00200079006F00750020006D00690067006800740020007400680069006E006B0020D83DDE01"

	long = "+61503975312: this is a very long message that does not fit in a single SMS message, at least it will if I keep adding more to it as 160 characters is more than you might think 😁\n"

	// an SMS-STATUS-REPORT, as received by the MS
	statusReport = "060105912143f5101010000000001010100000000000"
)

func TestRun(t *testing.T) {
	patterns := []struct {
		name   string
		args   []string
		status int
		stdout string
		stderr string
	}{
		{"pdu mode", []string{"-p", woot}, 0, "+61503975312: Woot\n", ""},
		{"concatenated", []string{seg1, seg2, seg3}, 0, long, ""},
		{"shuffled", []string{seg3, seg1, seg2}, 0, long, ""},
		{"duplicate", []string{seg3, seg1, seg1, seg2}, 1, long,
			"smsdeliver: TPDU 3: duplicate segment\n"},
		{"incomplete", []string{seg1, seg3}, 1, "",
			"smsdeliver: incomplete message from +61503975312, reference 192: have segments 1, 3 of 3, missing 2\n"},
		{"two incomplete", []string{seg3, "-", seg2}, 1, "",
			"smsdeliver: TPDU 2: encoding/hex: invalid byte: U+002D '-'\n" +
				"smsdeliver: incomplete message from +61503975312, reference 192: have segments 2, 3 of 3, missing 1\n"},
		{"underflow", []string{"00"}, 1, "",
			"smsdeliver: TPDU 1: tpdu: error decoding SmsDeliver.oa.addr at octet 1: underflow\n"},
		{"bad hex then good", []string{"-p", "zz", woot}, 1, "+61503975312: Woot\n",
			"smsdeliver: TPDU 1: encoding/hex: invalid byte: U+007A 'z'\n"},
		{"bad pdu mode", []string{"-p", "07"}, 1, "", "smsdeliver: TPDU 1: "},
		{"status report", []string{statusReport}, 1, "",
			"smsdeliver: TPDU 1: SmsStatusReport is not an SMS-DELIVER\n"},
		{"no pdu", nil, 2, "", "Usage: smsdeliver"},
		{"unknown flag", []string{"-x"}, 2, "", "flag provided but not defined"},
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

type errWriter struct{}

func (errWriter) Write([]byte) (int, error) {
	return 0, errors.New("write failed")
}

// A failure to write a message is reported and fails the run.
func TestRunWriteError(t *testing.T) {
	var stderr bytes.Buffer
	status := run([]string{"-p", woot}, errWriter{}, &stderr)
	assert.Equal(t, 1, status)
	assert.Equal(t, "smsdeliver: write failed\n", stderr.String())
}

func TestDescribeIncomplete(t *testing.T) {
	seg := func(seqno byte) *tpdu.TPDU {
		p := &tpdu.TPDU{OA: tpdu.Address{Addr: "1234", TOA: 0x91}}
		p.SetUDH(tpdu.UserDataHeader{{ID: 8, Data: []byte{0x12, 0x34, 4, seqno}}})
		return p
	}
	assert.Equal(t,
		"incomplete message from +1234, reference 4660: have segments 2, 4 of 4, missing 1, 3",
		describeIncomplete([]*tpdu.TPDU{nil, seg(2), nil, seg(4)}))
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
		{[]string{"-p", woot}, 0, "+61503975312: Woot\n", ""},
		{[]string{seg1, seg2, seg3}, 0, long, ""},
		{[]string{"00"}, 1, "", "underflow"},
		{[]string{statusReport}, 1, "", "is not an SMS-DELIVER"},
		{[]string{seg1}, 1, "", "incomplete message"},
		{nil, 2, "", "Usage: smsdeliver"},
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
		assert.Equal(t, p.stdout, stdout.String(), "%v", p.args)
		assert.Contains(t, stderr.String(), p.stderr, "%v", p.args)
	}
}
