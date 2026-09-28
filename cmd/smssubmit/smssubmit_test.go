// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/gomaja/go-sms"
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

// tpdus returns the TPDUs written by smssubmit, unmarshalled.
func tpdus(t *testing.T, out string) []*tpdu.TPDU {
	t.Helper()
	var pdus []*tpdu.TPDU
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	require.Zero(t, len(lines)%2, out)
	for i := 0; i < len(lines); i += 2 {
		require.True(t, strings.HasPrefix(lines[i], "Submit TPDU"), lines[i])
		b, err := hex.DecodeString(lines[i+1])
		require.NoError(t, err)
		p, err := sms.Unmarshal(b, sms.AsMO)
		require.NoError(t, err)
		pdus = append(pdus, p)
	}
	return pdus
}

func TestRun(t *testing.T) {
	long := strings.Repeat("this is long 😁 ", 20)
	patterns := []struct {
		name   string
		args   []string
		status int
		msg    string // the message the TPDUs carry, if any
		nli    int
		stderr string
	}{
		{"single", []string{"-number", "12345", "-message", "Hello world"}, 0, "Hello world", 0, ""},
		{"concatenated", []string{"-number", "12345", "-message", long}, 0, long, 0, ""},
		{"language", []string{"-number", "12345", "-message", "hi ş", "-language", "1"}, 0, "hi ş", 1, ""},
		{"bad number", []string{"-number", "12x45", "-message", "hi"}, 1, "", 0, "invalid digit"},
		{"invalid utf8", []string{"-number", "12345", "-message", "hi\xff"}, 1, "", 0, "invalid UTF8"},
		{"language 99", []string{"-number", "12345", "-message", "hi", "-language", "99"}, 2, "", 0, "invalid -language 99"},
		{"language 14", []string{"-number", "12345", "-message", "hi", "-language", "14"}, 2, "", 0, "invalid -language 14"},
		{"language -1", []string{"-number", "12345", "-message", "hi", "-language", "-1"}, 2, "", 0, "invalid -language -1"},
		{"no number", []string{"-message", "hi"}, 2, "", 0, "Usage: smssubmit"},
		{"no message", []string{"-number", "12345"}, 2, "", 0, "Usage: smssubmit"},
		{"extra argument", []string{"-number", "12345", "-message", "hi", "extra"}, 2, "", 0, "Usage: smssubmit"},
		{"unknown flag", []string{"-x"}, 2, "", 0, "flag provided but not defined"},
	}
	for _, p := range patterns {
		f := func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			status := run(p.args, &stdout, &stderr)
			assert.Equal(t, p.status, status)
			assert.Contains(t, stderr.String(), p.stderr)
			if p.status != 0 {
				assert.Empty(t, stdout.String())
				return
			}
			assert.Empty(t, stderr.String())
			pdus := tpdus(t, stdout.String())
			c := sms.NewCollector()
			defer c.Close()
			var msg []*tpdu.TPDU
			for i, pdu := range pdus {
				assert.Equal(t, "+12345", pdu.DA.Number())
				if i > 0 {
					assert.Equal(t, pdus[i-1].MR+1, pdu.MR)
				}
				if p.nli != 0 {
					ie, ok := pdu.UDH.IE(tpdu.IEINationalLanguageLockingShift)
					assert.True(t, ok)
					assert.Equal(t, []byte{byte(p.nli)}, ie.Data)
				}
				out, err := c.Collect(pdu, sms.WithOriginator("test"))
				require.NoError(t, err)
				if out != nil {
					msg = out
				}
			}
			require.NotNil(t, msg)
			m, err := sms.Decode(msg)
			require.NoError(t, err)
			assert.Equal(t, p.msg, string(m))
		}
		t.Run(p.name, f)
	}
}

type errWriter struct{}

func (errWriter) Write([]byte) (int, error) {
	return 0, errors.New("write failed")
}

// A failure to write the TPDUs is reported and fails the run.
func TestRunWriteError(t *testing.T) {
	var stderr bytes.Buffer
	status := run([]string{"-number", "12345", "-message", "hi"}, errWriter{}, &stderr)
	assert.Equal(t, 1, status)
	assert.Equal(t, "smssubmit: write failed\n", stderr.String())
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
		{[]string{"-number", "12345", "-message", "Hello world"}, 0, "Submit TPDU:\n0101", ""},
		{[]string{"-number", "12x45", "-message", "hi"}, 1, "", "smssubmit: "},
		{[]string{"-number", "12345", "-message", "hi", "-language", "99"}, 2, "", "invalid -language"},
		{nil, 2, "", "Usage: smssubmit"},
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
		if p.stdout == "" {
			assert.Empty(t, stdout.String(), "%v", p.args)
		} else {
			assert.True(t, strings.HasPrefix(stdout.String(), p.stdout), "%v: %q", p.args, stdout.String())
		}
		assert.Contains(t, stderr.String(), p.stderr, "%v", p.args)
	}
}
